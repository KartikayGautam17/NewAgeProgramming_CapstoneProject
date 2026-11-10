// Package model defines the data types passed between the scraper's stages.
package model

// Job is one unit of work sent from the dispatcher to a worker.
type Job struct {
	Index int    // original position in the input list, used to keep output ordered
	URL   string // absolute http(s) URL to fetch
}

// PageResult is the outcome of scraping one URL. It is sent from a worker to
// the collector through the results channel. A failed page still produces a
// PageResult, with Error and ErrorKind set, so that no URL is silently lost.
type PageResult struct {
	Index        int    `json:"-"`
	URL          string `json:"url"`
	StatusCode   int    `json:"status"`
	Title        string `json:"title"`
	MetaDesc     string `json:"meta_description"`
	LinkCount    int    `json:"links"`
	HeadingCount int    `json:"headings"`
	ResponseMs   int64  `json:"response_ms"`
	Attempts     int    `json:"attempts"`
	Error        string `json:"error,omitempty"`
	ErrorKind    string `json:"error_kind,omitempty"`
}

// OK reports whether the page was fetched and parsed successfully.
func (r PageResult) OK() bool { return r.Error == "" }
