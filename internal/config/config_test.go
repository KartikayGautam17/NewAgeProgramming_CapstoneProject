package config

import (
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestLoadURLs(t *testing.T) {
	in := `
# a comment
https://example.com
http://example.org/page
https://example.com
   https://example.net/trim   
ftp://example.com/file
not a url
https://

`
	valid, rejected, err := LoadURLs(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	wantValid := []string{"https://example.com", "http://example.org/page", "https://example.net/trim"}
	wantRej := []string{"ftp://example.com/file", "not a url", "https://"}
	if !reflect.DeepEqual(valid, wantValid) {
		t.Errorf("valid = %v, want %v", valid, wantValid)
	}
	if !reflect.DeepEqual(rejected, wantRej) {
		t.Errorf("rejected = %v, want %v", rejected, wantRej)
	}
}

func TestParseFlags(t *testing.T) {
	ok, err := Parse([]string{"-input", "u.txt", "-workers", "5", "-format", "CSV", "https://a.com"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if ok.Workers != 5 || ok.Format != "csv" || ok.Output != "results.csv" || len(ok.Extra) != 1 {
		t.Errorf("unexpected config: %+v", ok)
	}

	bad := [][]string{
		{},                                   // no URLs
		{"-input", "u", "-workers", "0"},     // too few workers
		{"-input", "u", "-workers", "101"},   // too many workers
		{"-input", "u", "-retries", "9"},     // too many retries
		{"-input", "u", "-timeout", "0s"},    // bad timeout
		{"-input", "u", "-format", "xml"},    // bad format
		{"-input", "u", "-nonexistent-flag"}, // unknown flag
	}
	for _, args := range bad {
		if _, err := Parse(args, io.Discard); err == nil {
			t.Errorf("Parse(%v) expected an error", args)
		}
	}
}
