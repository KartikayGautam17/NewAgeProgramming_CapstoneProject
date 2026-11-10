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
}
