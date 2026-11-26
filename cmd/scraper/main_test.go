package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"webscraper/internal/model"
)

// testSite serves a few small pages: /a and /b are fine, anything else is 404.
func testSite(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<title>Page A</title><meta name="description" content="about A"><h1>A</h1><a href="/b">b</a>`)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<title>Page B</title><h1>B</h1><h2>sub</h2>`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func runCmd(args ...string) (code int, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestRunWritesJSONFile(t *testing.T) {
	srv := testSite(t)
	list := writeFile(t, "urls.txt", srv.URL+"/a\n"+srv.URL+"/missing\nnot-a-url\n"+srv.URL+"/b\n"+srv.URL+"/a\n")
	out := filepath.Join(t.TempDir(), "r.json")

	code, _, stderr := runCmd("-input", list, "-out", out, "-workers", "3", "-retries", "0", "-rate", "0")
	if code != 0 {
		t.Fatalf("exit code %d, stderr:\n%s", code, stderr)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var got []model.PageResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	// the duplicate /a is removed and the invalid line is skipped => 3 results, in input order
	if len(got) != 3 {
		t.Fatalf("got %d results, want 3: %s", len(got), data)
	}
	if got[0].Title != "Page A" || got[0].MetaDesc != "about A" || got[0].LinkCount != 1 || got[0].HeadingCount != 1 {
		t.Errorf("first result wrong: %+v", got[0])
	}
	if got[1].Error != "HTTP 404" || got[1].ErrorKind != "http_4xx" {
		t.Errorf("second result should be the 404: %+v", got[1])
	}
	if got[2].Title != "Page B" || got[2].HeadingCount != 2 {
		t.Errorf("third result wrong: %+v", got[2])
	}
	for _, want := range []string{"warning: skipping invalid URL \"not-a-url\"", "Summary", "Succeeded     : 2", "Failed        : 1", "rate limit: unlimited", "Results written to"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr is missing %q:\n%s", want, stderr)
		}
	}
}

func TestRunWritesCSVFile(t *testing.T) {
	srv := testSite(t)
	out := filepath.Join(t.TempDir(), "r.csv")
	code, _, stderr := runCmd("-format", "csv", "-out", out, "-rate", "0", srv.URL+"/a", srv.URL+"/b")
	if code != 0 {
		t.Fatalf("exit code %d: %s", code, stderr)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil || len(rows) != 3 || rows[0][0] != "url" || rows[1][2] != "Page A" {
		t.Fatalf("unexpected CSV (err=%v): %v", err, rows)
	}
}

func TestRunStdoutKeepsDataAndSummarySeparate(t *testing.T) {
	srv := testSite(t)
	code, stdout, stderr := runCmd("-out", "-", "-rate", "0", srv.URL+"/a")
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	var got []model.PageResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil || len(got) != 1 {
		t.Fatalf("stdout should hold only the JSON (err=%v): %q", err, stdout)
	}
	if !strings.Contains(stderr, "Summary") || strings.Contains(stderr, "Results written to") {
		t.Errorf("summary belongs on stderr, and no 'written to' line when output is stdout:\n%s", stderr)
	}
}

func TestRunCombinesFileAndArgumentURLsWithoutDuplicates(t *testing.T) {
	srv := testSite(t)
	list := writeFile(t, "u.txt", srv.URL+"/a\n")
	code, stdout, _ := runCmd("-input", list, "-out", "-", "-rate", "0", srv.URL+"/a", srv.URL+"/b")
	var got []model.PageResult
	if code != 0 || json.Unmarshal([]byte(stdout), &got) != nil || len(got) != 2 {
		t.Fatalf("want 2 unique results (a from file, b from args), code=%d out=%s", code, stdout)
	}
}

func TestRunReportsFailuresButStillSucceeds(t *testing.T) {
	// A run where every page fails is still a completed run (exit 0); failures are in the report.
	code, stdout, stderr := runCmd("-out", "-", "-rate", "0", "-retries", "0", "-timeout", "1s", "http://127.0.0.1:1/x")
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if !strings.Contains(stdout, `"error_kind": "network"`) || !strings.Contains(stderr, "Failed        : 1") {
		t.Errorf("failure not reported:\nstdout=%s\nstderr=%s", stdout, stderr)
	}
}

func TestRunShowsConfiguredRate(t *testing.T) {
	srv := testSite(t)
	_, _, stderr := runCmd("-out", "-", "-rate", "50", srv.URL+"/a")
	if !strings.Contains(stderr, "rate limit: 50/s per host") {
		t.Errorf("rate not shown:\n%s", stderr)
	}
}

func TestRunUsageAndInputErrorsExitWithCode2(t *testing.T) {
	onlyBad := writeFile(t, "bad.txt", "nope\nftp://x.com\n")
	cases := map[string][]string{
		"no urls":               {},
		"bad workers":           {"-workers", "0", "http://x.com"},
		"bad rate":              {"-rate", "-1", "http://x.com"},
		"unknown flag":          {"-bogus", "http://x.com"},
		"help":                  {"-h"},
		"missing input file":    {"-input", filepath.Join(t.TempDir(), "nope.txt")},
		"no valid urls in file": {"-input", onlyBad},
	}
	for name, args := range cases {
		if code, _, _ := runCmd(args...); code != 2 {
			t.Errorf("%s: exit code %d, want 2", name, code)
		}
	}
}

func TestRunUnwritableOutputExitsWithCode1(t *testing.T) {
	srv := testSite(t)
	bad := filepath.Join(t.TempDir(), "no-such-dir", "r.json")
	code, _, stderr := runCmd("-out", bad, "-rate", "0", srv.URL+"/a")
	if code != 1 || !strings.Contains(stderr, "error writing output") {
		t.Errorf("code=%d stderr=%s", code, stderr)
	}
}
