// Package fetcher downloads pages over HTTP and classifies failures.
package fetcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Error kinds recorded in results and counted in the summary.
const (
	KindTimeout   = "timeout"
	KindNetwork   = "network"
	KindDNS       = "dns"
	KindCancelled = "cancelled"
	KindHTTP4xx   = "http_4xx"
	KindHTTP5xx   = "http_5xx"
	KindParse     = "parse"
)

// maxBody caps how much of a response we read, so one huge page cannot
// exhaust memory.
const maxBody = 5 << 20 // 5 MiB

// Response is a downloaded page.
type Response struct {
	StatusCode int
	Body       []byte
}

// Fetcher downloads a URL. It is an interface so tests can substitute a fake.
type Fetcher interface {
	Fetch(ctx context.Context, url string) (Response, error)
}

// HTTPFetcher is the real implementation backed by net/http.
type HTTPFetcher struct {
	Client    *http.Client
	UserAgent string
}

// NewHTTPFetcher builds a fetcher whose requests are limited to timeout.
func NewHTTPFetcher(timeout time.Duration, userAgent string) *HTTPFetcher {
	return &HTTPFetcher{
		Client:    &http.Client{Timeout: timeout},
		UserAgent: userAgent,
	}
}

// Fetch performs an HTTP GET. A non-2xx status is NOT an error here; the
// caller decides what to do with it. Transport failures are returned as errors.
func (f *HTTPFetcher) Fetch(ctx context.Context, url string) (Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Response{}, fmt.Errorf("build request: %w", err)
	}
	if f.UserAgent != "" {
		req.Header.Set("User-Agent", f.UserAgent)
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := f.Client.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return Response{StatusCode: resp.StatusCode}, fmt.Errorf("read body: %w", err)
	}
	return Response{StatusCode: resp.StatusCode, Body: body}, nil
}

// Classify maps a transport error to one of the Kind* constants.
func Classify(err error) string {
	var dnsErr *net.DNSError
	var netErr net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return KindCancelled
	case errors.Is(err, context.DeadlineExceeded):
		return KindTimeout
	case errors.As(err, &netErr) && netErr.Timeout():
		return KindTimeout
	case errors.As(err, &dnsErr):
		return KindDNS
	default:
		return KindNetwork
	}
}

// StatusKind maps an HTTP status code of 400 or above to a Kind* constant.
func StatusKind(code int) string {
	if code >= 500 {
		return KindHTTP5xx
	}
	return KindHTTP4xx
}
