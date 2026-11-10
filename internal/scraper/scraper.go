// Package scraper contains the concurrent core of the tool: a dispatcher that
// feeds a jobs channel, a pool of worker goroutines, and a collector that
// gathers results from a results channel.
//
//	URLs --> dispatcher --jobs chan--> N workers --results chan--> collector
package scraper

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"webscraper/internal/fetcher"
	"webscraper/internal/model"
	"webscraper/internal/parser"
)

// Options controls how the scraper runs.
type Options struct {
	Workers int           // number of worker goroutines (>= 1)
	Retries int           // extra attempts after the first for temporary failures
	Backoff time.Duration // delay before first retry; doubles each time

	// Progress, if set, is called by the collector after each result.
	// It runs on a single goroutine, so it needs no locking.
	Progress func(done, total int, r model.PageResult)
}

// Scraper downloads and parses pages using a Fetcher.
type Scraper struct {
	fetch fetcher.Fetcher
	opts  Options
}

// New creates a Scraper. Workers below 1 is treated as 1.
func New(f fetcher.Fetcher, opts Options) *Scraper {
	if opts.Workers < 1 {
		opts.Workers = 1
	}
	if opts.Backoff <= 0 {
		opts.Backoff = 200 * time.Millisecond
	}
	return &Scraper{fetch: f, opts: opts}
}

// Run scrapes every URL concurrently and returns the results in the same order
// as urls. If ctx is cancelled, URLs not yet started are skipped and the
// returned slice contains only the pages that were processed.
func (s *Scraper) Run(ctx context.Context, urls []string) []model.PageResult {
	jobs := make(chan model.Job, s.opts.Workers)
	results := make(chan model.PageResult, s.opts.Workers)

	// M2: dispatcher. The only goroutine that writes to jobs, so it closes it.
	go func() {
		defer close(jobs)
		for i, u := range urls {
			select {
			case jobs <- model.Job{Index: i, URL: u}:
			case <-ctx.Done():
				return
			}
		}
	}()

	// M3: worker pool.
	var wg sync.WaitGroup
	for w := 0; w < s.opts.Workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				results <- s.process(ctx, job)
			}
		}()
	}

	// Closer: results is closed only after every worker has returned.
	go func() {
		wg.Wait()
		close(results)
	}()

	// M4: collector. Only this goroutine touches `out`, so no mutex is needed.
	out := make([]model.PageResult, 0, len(urls))
	for r := range results {
		out = append(out, r)
		if s.opts.Progress != nil {
			s.opts.Progress(len(out), len(urls), r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out
}

// RunSequential processes the URLs one at a time on the calling goroutine.
// It exists as the baseline for measuring the speed-up from concurrency.
func (s *Scraper) RunSequential(ctx context.Context, urls []string) []model.PageResult {
	out := make([]model.PageResult, 0, len(urls))
	for i, u := range urls {
		if ctx.Err() != nil {
			break
		}
		r := s.process(ctx, model.Job{Index: i, URL: u})
		out = append(out, r)
		if s.opts.Progress != nil {
			s.opts.Progress(len(out), len(urls), r)
		}
	}
	return out
}

// process fetches and parses one URL, retrying temporary failures.
func (s *Scraper) process(ctx context.Context, job model.Job) model.PageResult {
	res := model.PageResult{Index: job.Index, URL: job.URL}
	backoff := s.opts.Backoff

	for attempt := 0; attempt <= s.opts.Retries; attempt++ {
		res.Attempts = attempt + 1
		start := time.Now()
		resp, err := s.fetch.Fetch(ctx, job.URL)
		res.ResponseMs = time.Since(start).Milliseconds()

		retryable := false
		switch {
		case err != nil:
			res.StatusCode = resp.StatusCode
			res.ErrorKind = fetcher.Classify(err)
			res.Error = err.Error()
			retryable = res.ErrorKind == fetcher.KindTimeout || res.ErrorKind == fetcher.KindNetwork

		case resp.StatusCode >= 400:
			res.StatusCode = resp.StatusCode
			res.ErrorKind = fetcher.StatusKind(resp.StatusCode)
			res.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
			retryable = resp.StatusCode >= 500 || resp.StatusCode == 429

		default:
			page, perr := parser.Parse(bytes.NewReader(resp.Body))
			res.StatusCode = resp.StatusCode
			if perr != nil {
				res.ErrorKind = fetcher.KindParse
				res.Error = perr.Error()
				return res
			}
			res.Title, res.MetaDesc = page.Title, page.MetaDesc
			res.LinkCount, res.HeadingCount = page.LinkCount, page.HeadingCount
			res.Error, res.ErrorKind = "", ""
			return res
		}

		if !retryable || attempt == s.opts.Retries {
			return res
		}
		select { // wait before retrying, but stop at once if cancelled
		case <-time.After(backoff):
			backoff *= 2
		case <-ctx.Done():
			res.ErrorKind, res.Error = fetcher.KindCancelled, ctx.Err().Error()
			return res
		}
	}
	return res
}
