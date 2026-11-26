// Command benchmark measures the speed-up from concurrency. It starts a local
// HTTP server whose every page takes a fixed delay to respond (simulating
// network latency), then scrapes it sequentially and with several pool sizes.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"webscraper/internal/fetcher"
	"webscraper/internal/scraper"
)

func main() {
	pages := flag.Int("pages", 100, "number of pages to scrape")
	delay := flag.Duration("delay", 200*time.Millisecond, "simulated server latency per page")
	flag.Parse()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(*delay)
		fmt.Fprintf(w, `<html><head><title>Page %s</title><meta name="description" content="demo"></head>
<body><h1>Hello</h1><a href="/x">x</a><a href="/y">y</a></body></html>`, r.URL.Path)
	}))
	defer srv.Close()

	urls := make([]string, *pages)
	for i := range urls {
		urls[i] = fmt.Sprintf("%s/page/%d", srv.URL, i)
	}
	f := fetcher.NewHTTPFetcher(10*time.Second, "benchmark")
	ctx := context.Background()

	fmt.Printf("Scraping %d pages, server delay %s per page\n\n", *pages, *delay)
	fmt.Printf("%-22s %12s %10s\n", "Mode", "Time", "Speed-up")

	start := time.Now()
	scraper.New(f, scraper.Options{}).RunSequential(ctx, urls)
	base := time.Since(start)
	fmt.Printf("%-22s %12s %9.1fx\n", "sequential (baseline)", base.Round(time.Millisecond), 1.0)

	for _, w := range []int{2, 5, 10, 20, 50, 100} {
		start = time.Now()
		res := scraper.New(f, scraper.Options{Workers: w}).Run(ctx, urls)
		d := time.Since(start)
		ok := 0
		for _, r := range res {
			if r.OK() {
				ok++
			}
		}
		fmt.Printf("%-22s %12s %9.1fx   (%d/%d ok)\n", fmt.Sprintf("%d workers", w), d.Round(time.Millisecond), float64(base)/float64(d), ok, len(urls))
	}
	rateTable(f, urls, 10)
}

// rateTable shows the effect of the per-host rate limiter: with every page on
// one host (as here), a lower rate spaces requests out even with many workers.
func rateTable(f fetcher.Fetcher, urls []string, workers int) {
	if len(urls) > 20 {
		urls = urls[:20]
	}
	fmt.Printf("\nEffect of the per-host rate limit (%d pages, %d workers, one host)\n\n", len(urls), workers)
	fmt.Printf("%-22s %12s\n", "Rate limit", "Time")
	for _, r := range []float64{0, 50, 20, 10} {
		name := "unlimited"
		if r > 0 {
			name = fmt.Sprintf("%g requests/s", r)
		}
		start := time.Now()
		scraper.New(f, scraper.Options{Workers: workers, Rate: r}).Run(context.Background(), urls)
		fmt.Printf("%-22s %12s\n", name, time.Since(start).Round(time.Millisecond))
	}
}
