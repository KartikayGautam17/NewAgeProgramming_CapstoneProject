# Concurrent Web Scraper in Go

ENSP465 Capstone, final version (Phase III). A command-line tool that downloads
many web pages **concurrently** using a pool of goroutines and extracts, for each
page: HTTP status, title, meta description, number of links, number of headings
and response time. It limits its own request rate per host, retries temporary
failures, writes results to JSON or CSV, and prints a summary.

## Setup

Requires Go 1.23 or newer.

```bash
go mod tidy        # downloads golang.org/x/net and creates go.sum
go build -o scraper ./cmd/scraper
```

## Usage

```bash
./scraper -input testdata/urls.txt                         # JSON -> results.json
./scraper -input testdata/urls.txt -workers 20 -format csv
./scraper -workers 5 -retries 1 -timeout 5s https://example.com https://go.dev
./scraper -input urls.txt -rate 2                          # at most 2 requests/s per host
./scraper -input urls.txt -out -                           # JSON to stdout, summary to stderr
```

| Flag | Default | Meaning |
|---|---|---|
| `-input` | none | File with one URL per line (`-` = stdin). URLs can also be given as arguments |
| `-workers` | 10 | Concurrent workers, 1-100 |
| `-timeout` | 10s | Timeout per HTTP request |
| `-retries` | 2 | Extra attempts for temporary failures (timeouts, network errors, 5xx, 429), with doubling backoff |
| `-rate` | 5 | Max requests per second to any single host, 0-1000 (0 = unlimited) |
| `-format` | json | `json` or `csv` |
| `-out` | results.<format> | Output file, `-` for stdout |
| `-user-agent` | GoWebScraper/0.2 | User-Agent header |

Blank lines, `#` comments, invalid URLs (not http/https) and duplicates are
skipped. Ctrl+C stops the run cleanly and saves the results gathered so far.
Exit codes: 0 finished, 1 output could not be written, 2 bad usage or input,
130 interrupted.

### Output fields

`url`, `status`, `title`, `meta_description`, `links`, `headings`, `response_ms`,
`attempts`, and, for failed pages only, `error_kind` (`timeout`, `network`, `dns`,
`http_4xx`, `http_5xx`, `parse`, `cancelled`) and `error`.

## Design

```
URLs -> [M1 config/loader] -> [M2 dispatcher] --jobs chan--> [M3 N workers: rate wait, fetch, retry, parse]
                                                                    |
                                          [M5 report writer] <- [M4 collector] <--results chan--
```

| Module | Where | What it does |
|---|---|---|
| M1 Config & Input Loader | `internal/config` | Flags, validation, URL file loading |
| M2 Dispatcher / M3 Worker pool / M4 Collector | `internal/scraper` | Goroutines, channels, `sync.WaitGroup`, `context`, retries |
| Per-host rate limiter | `internal/scraper/limiter.go` | Spaces requests to the same host; standard library only |
| Fetcher | `internal/fetcher` | `net/http` client, timeouts, error classification (`Fetcher` interface) |
| Parser | `internal/parser` | `golang.org/x/net/html` tokenizer |
| M5 Report Writer | `internal/report` | JSON/CSV output and run summary |

## Tests and benchmark

```bash
go test -race -cover ./...        # 34 test functions + subtests, race detector, coverage
go run ./cmd/benchmark            # speed-up table and rate-limit table (local server)
go test -bench . ./internal/scraper
```

Results on the development machine: all tests pass under `-race` (also with
`-count=3`); overall statement coverage 83.5% (cmd/scraper 94%, parser 98%,
config 96%, report 95%, scraper 91%, fetcher 89%).

Concurrency benchmark (100 pages, 200 ms simulated server latency):

| Mode | Time | Speed-up |
|---|---|---|
| sequential | 20.1 s | 1.0x |
| 5 workers | 4.0 s | 5.0x |
| 10 workers | 2.0 s | 10.0x |
| 50 workers | 0.42 s | 47.5x |
| 100 workers | 0.22 s | 91.0x |

Effect of the per-host rate limit (20 pages, 10 workers, one host):

| Rate limit | Time |
|---|---|
| unlimited | 0.40 s |
| 50 requests/s | 0.58 s |
| 20 requests/s | 1.15 s |
| 10 requests/s | 2.10 s |

The benchmark uses a local server so the numbers are repeatable. Times on real
websites depend on the network and the sites.

## Limitations

No `robots.txt` handling, no JavaScript execution, no crawling (only the URLs
given are fetched), and no login support. Use it only on pages you are allowed to
scrape.
