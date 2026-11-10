package fetcher

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchSetsUserAgentAndReturnsBody(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(404)
		w.Write([]byte("nope"))
	}))
	defer srv.Close()

	resp, err := NewHTTPFetcher(time.Second, "UA-test").Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err) // a 404 is not a transport error
	}
	if gotUA != "UA-test" || resp.StatusCode != 404 || string(resp.Body) != "nope" {
		t.Errorf("got ua=%q resp=%d %q", gotUA, resp.StatusCode, resp.Body)
	}
}

func TestFetchLimitsBodySize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("x", maxBody+1000)))
	}))
	defer srv.Close()
	resp, err := NewHTTPFetcher(5*time.Second, "").Fetch(context.Background(), srv.URL)
	if err != nil || len(resp.Body) != maxBody {
		t.Errorf("want body capped at %d, got %d (err=%v)", maxBody, len(resp.Body), err)
	}
}

func TestFetchBadURL(t *testing.T) {
	if _, err := NewHTTPFetcher(time.Second, "").Fetch(context.Background(), "://bad"); err == nil {
		t.Error("expected error for malformed URL")
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{context.Canceled, KindCancelled},
		{context.DeadlineExceeded, KindTimeout},
		{&net.DNSError{Err: "no such host", Name: "x", IsNotFound: true}, KindDNS},
		{errors.New("connection refused"), KindNetwork},
	}
	for _, tc := range tests {
		if got := Classify(tc.err); got != tc.want {
			t.Errorf("Classify(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
	if StatusKind(404) != KindHTTP4xx || StatusKind(503) != KindHTTP5xx {
		t.Error("StatusKind mapping wrong")
	}
}
