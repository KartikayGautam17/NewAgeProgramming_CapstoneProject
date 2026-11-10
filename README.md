# Concurrent Web Scraper in Go

ENSP465 Capstone, Phase II (implementation). A command-line tool that downloads
many web pages **concurrently** using a pool of goroutines and extracts, for each
page: HTTP status, title, meta description, number of links, number of headings
and response time. Results go to JSON or CSV, with a summary in the terminal.

## Setup

Requires Go 1.23 or newer.

```bash
go mod tidy        # downloads golang.org/x/net and creates go.sum
go build -o scraper ./cmd/scraper
```

## Usage

```bash
./scraper -input testdata/urls.txt                        # JSON -> results.json
./scraper -input testdata/urls.txt -workers 20 -format csv
./scraper -workers 5 -retries 1 -timeout 5s https://example.com https://go.dev
./scraper -input urls.txt -out -                          # JSON to stdout
```

| Flag | Default | Meaning |
|---|---|---|
| `-input` | none | File with one URL per line (`-` = stdin) |
| `-workers` | 10 | Concurrent workers, 1-100 |
| `-timeout` | 10s | Timeout per HTTP request |
| `-retries` | 2 | Extra attempts for temporary failures (timeouts, network errors, 5xx, 429), with doubling backoff |
| `-format` | json | `json` or `csv` |
| `-out` | results.<format> | Output file, `-` for stdout |
| `-user-agent` | GoWebScraper/0.2 | User-Agent header |

Positional arguments are extra URLs. Blank lines, `#` comments, invalid URLs
(not http/https) and duplicates are skipped. Ctrl+C stops the run cleanly and
saves the results gathered so far.

## Design (matches the Phase I report)

```
URLs -> [M1 config/loader] -> [M2 dispatcher] --jobs chan--> [M3 N workers: fetch + parse]
                                                                    |
                                          [M5 report writer] <- [M4 collector] <--results chan--
```

| Module | Where | What it does |
|---|---|---|
| M1 Config & Input Loader | `internal/config` | Flags, validation, URL file loading |
| M2 Dispatcher / M3 Worker pool / M4 Collector | `internal/scraper` | Goroutines, channels, `sync.WaitGroup`, `context`, retries |
| Fetcher | `internal/fetcher` | `net/http` client, timeouts, error classification (`Fetcher` interface) |
| Parser | `internal/parser` | `golang.org/x/net/html` tokenizer |
| M5 Report Writer | `internal/report` | JSON/CSV output and run summary |

Go features used: goroutines, buffered channels, `sync.WaitGroup`, `context`
cancellation (`signal.NotifyContext`), interfaces for testing, struct tags,
error classification with `errors.As`/`errors.Is`.

## Tests and benchmark

```bash
go test -race -cover ./...        # unit tests + race detector
go run ./cmd/benchmark            # sequential vs concurrent speed-up (local server, 200 ms per page)
go test -bench . ./internal/scraper
```
