// Package report writes results to JSON or CSV and prints the run summary.
package report

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"

	"webscraper/internal/model"
)

// WriteJSON writes the results as an indented JSON array.
func WriteJSON(w io.Writer, rs []model.PageResult) error {
	if rs == nil {
		rs = []model.PageResult{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rs)
}

// WriteCSV writes the results as CSV with a header row.
func WriteCSV(w io.Writer, rs []model.PageResult) error {
	cw := csv.NewWriter(w)
	header := []string{"url", "status", "title", "meta_description", "links", "headings", "response_ms", "attempts", "error_kind", "error"}
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, r := range rs {
		row := []string{
			r.URL, strconv.Itoa(r.StatusCode), r.Title, r.MetaDesc,
			strconv.Itoa(r.LinkCount), strconv.Itoa(r.HeadingCount),
			strconv.FormatInt(r.ResponseMs, 10), strconv.Itoa(r.Attempts),
			r.ErrorKind, r.Error,
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// Summary is the aggregate view of one run.
type Summary struct {
	Total     int // URLs supplied
	Succeeded int
	Failed    int
	Skipped   int            // never started because the run was cancelled
	Elapsed   time.Duration  // wall-clock time of the whole run
	AvgMs     float64        // mean response time of successful pages
	Failures  map[string]int // failure count by error kind
}

// Summarize computes run statistics. total is the number of URLs supplied.
func Summarize(rs []model.PageResult, total int, elapsed time.Duration) Summary {
	s := Summary{Total: total, Elapsed: elapsed, Failures: map[string]int{}}
	var sum int64
	for _, r := range rs {
		if r.OK() {
			s.Succeeded++
			sum += r.ResponseMs
		} else {
			s.Failed++
			s.Failures[r.ErrorKind]++
		}
	}
	if s.Succeeded > 0 {
		s.AvgMs = float64(sum) / float64(s.Succeeded)
	}
	s.Skipped = total - len(rs)
	return s
}

// Print writes a human-readable summary.
func (s Summary) Print(w io.Writer) {
	fmt.Fprintln(w, "---------------- Summary ----------------")
	fmt.Fprintf(w, "URLs supplied : %d\n", s.Total)
	fmt.Fprintf(w, "Succeeded     : %d\n", s.Succeeded)
	fmt.Fprintf(w, "Failed        : %d\n", s.Failed)
	if s.Skipped > 0 {
		fmt.Fprintf(w, "Skipped       : %d (run cancelled)\n", s.Skipped)
	}
	kinds := make([]string, 0, len(s.Failures))
	for k := range s.Failures {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		fmt.Fprintf(w, "  - %-10s: %d\n", k, s.Failures[k])
	}
	fmt.Fprintf(w, "Avg response  : %.0f ms (successful pages)\n", s.AvgMs)
	fmt.Fprintf(w, "Total time    : %s\n", s.Elapsed.Round(time.Millisecond))
}
