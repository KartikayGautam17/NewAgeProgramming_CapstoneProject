// Command scraper downloads many web pages concurrently and extracts the
// title, meta description and heading/link counts from each.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"webscraper/internal/config"
	"webscraper/internal/fetcher"
	"webscraper/internal/model"
	"webscraper/internal/report"
	"webscraper/internal/scraper"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	// M1: configuration and input loading.
	cfg, err := config.Parse(args, stderr)
	if err != nil {
		if err.Error() != "flag: help requested" {
			fmt.Fprintln(stderr, "error:", err)
		}
		return 2
	}
	urls, rejected, err := loadAll(cfg)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	for _, r := range rejected {
		fmt.Fprintf(stderr, "warning: skipping invalid URL %q\n", r)
	}
	if len(urls) == 0 {
		fmt.Fprintln(stderr, "error: no valid URLs to scrape")
		return 2
	}

	// Ctrl+C cancels the context; workers stop picking up new jobs.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	s := scraper.New(fetcher.NewHTTPFetcher(cfg.Timeout, cfg.UserAgent), scraper.Options{
		Workers: cfg.Workers,
		Retries: cfg.Retries,
		Progress: func(done, total int, r model.PageResult) {
			status := "ok"
			if !r.OK() {
				status = "FAIL " + r.ErrorKind
			}
			fmt.Fprintf(stderr, "[%d/%d] %-14s %s\n", done, total, status, r.URL)
		},
	})

	fmt.Fprintf(stderr, "Scraping %d URL(s) with %d worker(s)...\n", len(urls), cfg.Workers)
	start := time.Now()
	results := s.Run(ctx, urls)
	elapsed := time.Since(start)

	// M5: write the report, then print the summary.
	if err := writeOutput(cfg, stdout, results); err != nil {
		fmt.Fprintln(stderr, "error writing output:", err)
		return 1
	}
	sum := report.Summarize(results, len(urls), elapsed)
	sum.Print(stderr)
	if cfg.Output != "-" {
		fmt.Fprintf(stderr, "Results written to %s\n", cfg.Output)
	}
	if ctx.Err() != nil {
		fmt.Fprintln(stderr, "Interrupted: partial results were saved.")
		return 130
	}
	return 0
}

func loadAll(cfg config.Config) (urls, rejected []string, err error) {
	if cfg.InputFile != "" {
		var r io.Reader = os.Stdin
		if cfg.InputFile != "-" {
			f, ferr := os.Open(cfg.InputFile)
			if ferr != nil {
				return nil, nil, ferr
			}
			defer f.Close()
			r = f
		}
		urls, rejected, err = config.LoadURLs(r)
		if err != nil {
			return nil, nil, err
		}
	}
	extra, bad := config.Normalize(cfg.Extra)
	seen := make(map[string]bool, len(urls))
	for _, u := range urls {
		seen[u] = true
	}
	for _, u := range extra {
		if !seen[u] {
			urls = append(urls, u)
		}
	}
	return urls, append(rejected, bad...), nil
}

func writeOutput(cfg config.Config, stdout io.Writer, rs []model.PageResult) error {
	w := stdout
	if cfg.Output != "-" {
		f, err := os.Create(cfg.Output)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	if cfg.Format == "csv" {
		return report.WriteCSV(w, rs)
	}
	return report.WriteJSON(w, rs)
}
