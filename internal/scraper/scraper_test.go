package scraper

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"webscraper/internal/fetcher"
	"webscraper/internal/model"
)

const okPage = `<html><head><title>%s</title><meta name="description" content="d"></head><body><h1>h</h1><a href="/a">a</a></body></html>`

func newServer(t *testing.T, delay time.Duration) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/ok/", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		fmt.Fprintf(w, okPage, r.URL.Path)
	})
	mux.HandleFunc("/missing", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	mux.HandleFunc("/boom", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "x", 500) })
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) { time.Sleep(500 * time.Millisecond) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRunKeepsInputOrderAndExtractsData(t *testing.T) {
	srv := newServer(t, 5*time.Millisecond)
	var urls []string
	for i := 0; i < 30; i++ {
		urls = append(urls, fmt.Sprintf("%s/ok/%d", srv.URL, i))
	}
	s := New(fetcher.NewHTTPFetcher(5*time.Second, "test"), Options{Workers: 8})
	res := s.Run(context.Background(), urls)

	if len(res) != len(urls) {
		t.Fatalf("got %d results, want %d", len(res), len(urls))
	}
	for i, r := range res {
		if r.URL != urls[i] {
			t.Fatalf("result %d out of order: %s", i, r.URL)
		}
		if !r.OK() || r.StatusCode != 200 || r.Title != fmt.Sprintf("/ok/%d", i) ||
			r.MetaDesc != "d" || r.HeadingCount != 1 || r.LinkCount != 1 || r.Attempts != 1 {
			t.Fatalf("unexpected result: %+v", r)
		}
	}
}

func TestRunMixedFailuresDoNotStopTheRun(t *testing.T) {
	srv := newServer(t, 0)
	urls := []string{srv.URL + "/ok/1", srv.URL + "/missing", srv.URL + "/boom", srv.URL + "/ok/2", "http://127.0.0.1:1/unreachable"}
	s := New(fetcher.NewHTTPFetcher(2*time.Second, "test"), Options{Workers: 3, Retries: 1, Backoff: time.Millisecond})
	res := s.Run(context.Background(), urls)
	if len(res) != 5 {
		t.Fatalf("got %d results, want 5", len(res))
	}
	want := []struct {
		ok       bool
		kind     string
		attempts int
	}{
		{true, "", 1},
		{false, fetcher.KindHTTP4xx, 1}, // 404 is permanent: no retry
		{false, fetcher.KindHTTP5xx, 2}, // 500 is temporary: retried once
		{true, "", 1},
		{false, fetcher.KindNetwork, 2}, // connection refused: retried once
	}
	for i, w := range want {
		r := res[i]
		if r.OK() != w.ok || r.ErrorKind != w.kind || r.Attempts != w.attempts {
			t.Errorf("result %d (%s): got ok=%v kind=%q attempts=%d, want ok=%v kind=%q attempts=%d",
				i, r.URL, r.OK(), r.ErrorKind, r.Attempts, w.ok, w.kind, w.attempts)
		}
	}
}

func TestTimeoutIsClassified(t *testing.T) {
	srv := newServer(t, 0)
	s := New(fetcher.NewHTTPFetcher(50*time.Millisecond, "test"), Options{Workers: 1, Retries: 0})
	res := s.Run(context.Background(), []string{srv.URL + "/slow"})
	if len(res) != 1 || res[0].ErrorKind != fetcher.KindTimeout {
		t.Fatalf("want one timeout result, got %+v", res)
	}
}

// flakyFetcher fails the first n calls for each URL, then succeeds.
type flakyFetcher struct {
	mu    sync.Mutex
	calls map[string]int
	n     int
}

func (f *flakyFetcher) Fetch(ctx context.Context, url string) (fetcher.Response, error) {
	f.mu.Lock()
	f.calls[url]++
	c := f.calls[url]
	f.mu.Unlock()
	if c <= f.n {
		return fetcher.Response{StatusCode: 503}, nil
	}
	return fetcher.Response{StatusCode: 200, Body: []byte("<title>ok</title>")}, nil
}

func TestRetrySucceedsAfterTemporaryFailures(t *testing.T) {
	f := &flakyFetcher{calls: map[string]int{}, n: 2}
	s := New(f, Options{Workers: 2, Retries: 2, Backoff: time.Millisecond})
	res := s.Run(context.Background(), []string{"http://a", "http://b"})
	for _, r := range res {
		if !r.OK() || r.Attempts != 3 || r.Title != "ok" {
			t.Errorf("want success on 3rd attempt, got %+v", r)
		}
	}
	// Not enough retries: must report failure.
	f2 := &flakyFetcher{calls: map[string]int{}, n: 5}
	res = New(f2, Options{Workers: 1, Retries: 1, Backoff: time.Millisecond}).Run(context.Background(), []string{"http://c"})
	if res[0].OK() || res[0].Attempts != 2 || res[0].ErrorKind != fetcher.KindHTTP5xx {
		t.Errorf("want failure after 2 attempts, got %+v", res[0])
	}
}

func TestCancellationStopsTheRun(t *testing.T) {
	srv := newServer(t, 50*time.Millisecond)
	var urls []string
	for i := 0; i < 200; i++ {
		urls = append(urls, fmt.Sprintf("%s/ok/%d", srv.URL, i))
	}
	ctx, cancel := context.WithCancel(context.Background())
	var done atomic.Int32
	s := New(fetcher.NewHTTPFetcher(5*time.Second, "t"), Options{
		Workers: 4,
		Progress: func(d, _ int, _ model.PageResult) {
			if done.Add(1) == 10 {
				cancel()
			}
		},
	})
	start := time.Now()
	res := s.Run(ctx, urls)
	if time.Since(start) > 3*time.Second {
		t.Error("run did not stop promptly after cancellation")
	}
	if len(res) >= len(urls) || len(res) < 10 {
		t.Errorf("expected a partial result set, got %d of %d", len(res), len(urls))
	}
}

func TestConcurrencyIsFasterThanSequential(t *testing.T) {
	srv := newServer(t, 50*time.Millisecond)
	var urls []string
	for i := 0; i < 40; i++ {
		urls = append(urls, fmt.Sprintf("%s/ok/%d", srv.URL, i))
	}
	f := fetcher.NewHTTPFetcher(5*time.Second, "t")
	t0 := time.Now()
	New(f, Options{}).RunSequential(context.Background(), urls)
	seq := time.Since(t0)
	t0 = time.Now()
	New(f, Options{Workers: 10}).Run(context.Background(), urls)
	conc := time.Since(t0)
	if conc*4 > seq {
		t.Errorf("expected at least 4x speed-up: sequential=%s concurrent=%s", seq, conc)
	}
	t.Logf("sequential=%s concurrent(10)=%s speed-up=%.1fx", seq, conc, float64(seq)/float64(conc))
}

func TestEmptyInput(t *testing.T) {
	res := New(&flakyFetcher{calls: map[string]int{}}, Options{Workers: 4}).Run(context.Background(), nil)
	if len(res) != 0 {
		t.Errorf("want no results, got %d", len(res))
	}
}

func BenchmarkSequential(b *testing.B) { benchRun(b, 1, true) }
func BenchmarkWorkers10(b *testing.B)  { benchRun(b, 10, false) }

func benchRun(b *testing.B, workers int, seq bool) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(20 * time.Millisecond)
		fmt.Fprintf(w, okPage, "b")
	}))
	defer srv.Close()
	urls := make([]string, 50)
	for i := range urls {
		urls[i] = fmt.Sprintf("%s/p/%d", srv.URL, i)
	}
	s := New(fetcher.NewHTTPFetcher(5*time.Second, "b"), Options{Workers: workers})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if seq {
			s.RunSequential(context.Background(), urls)
		} else {
			s.Run(context.Background(), urls)
		}
	}
}
