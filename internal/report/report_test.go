package report

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"webscraper/internal/model"
)

var sample = []model.PageResult{
	{URL: "https://a.com", StatusCode: 200, Title: "A, with comma", MetaDesc: `say "hi"`, LinkCount: 3, HeadingCount: 1, ResponseMs: 50, Attempts: 1},
	{URL: "https://b.com", StatusCode: 404, Attempts: 1, Error: "HTTP 404", ErrorKind: "http_4xx"},
	{URL: "https://c.com", Attempts: 3, Error: "timeout", ErrorKind: "timeout"},
}

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, sample); err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(got) != 3 || got[0]["title"] != "A, with comma" {
		t.Errorf("unexpected JSON: %s", buf.String())
	}
	if _, has := got[0]["error"]; has {
		t.Error("successful result should omit the error field")
	}
	buf.Reset()
	_ = WriteJSON(&buf, nil)
	if strings.TrimSpace(buf.String()) != "[]" {
		t.Errorf("nil results should encode as [], got %q", buf.String())
	}
}

func TestWriteCSV(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteCSV(&buf, sample); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("output is not valid CSV: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("want header + 3 rows, got %d rows", len(rows))
	}
	if rows[1][2] != "A, with comma" || rows[1][3] != `say "hi"` {
		t.Errorf("commas/quotes were not preserved: %q", rows[1])
	}
}

func TestSummarize(t *testing.T) {
	s := Summarize(sample, 5, 2*time.Second) // 5 supplied, 3 processed => 2 skipped
	if s.Succeeded != 1 || s.Failed != 2 || s.Skipped != 2 {
		t.Errorf("unexpected counts: %+v", s)
	}
	if s.Failures["http_4xx"] != 1 || s.Failures["timeout"] != 1 {
		t.Errorf("unexpected failure breakdown: %v", s.Failures)
	}
	if s.AvgMs != 50 {
		t.Errorf("AvgMs = %v, want 50", s.AvgMs)
	}
	var buf bytes.Buffer
	s.Print(&buf)
	if !strings.Contains(buf.String(), "Skipped") || !strings.Contains(buf.String(), "timeout") {
		t.Errorf("summary text missing details:\n%s", buf.String())
	}
}
