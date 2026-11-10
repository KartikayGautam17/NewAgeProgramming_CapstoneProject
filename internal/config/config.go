// Package config parses command-line flags and loads the list of URLs.
package config

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

// Config holds every user-controllable setting.
type Config struct {
	InputFile string // "-" means read URLs from standard input
	Extra     []string
	Workers   int
	Timeout   time.Duration
	Retries   int
	Format    string // "json" or "csv"
	Output    string // file path, or "-" for standard output
	UserAgent string
}

// Parse reads flags from args (normally os.Args[1:]) and validates them.
// Any positional arguments are treated as additional URLs.
func Parse(args []string, errOut io.Writer) (Config, error) {
	var c Config
	fs := flag.NewFlagSet("scraper", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.StringVar(&c.InputFile, "input", "", "file with one URL per line ('-' for stdin)")
	fs.IntVar(&c.Workers, "workers", 10, "number of concurrent workers (1-100)")
	fs.DurationVar(&c.Timeout, "timeout", 10*time.Second, "timeout for each HTTP request")
	fs.IntVar(&c.Retries, "retries", 2, "retries for temporary failures (0-5)")
	fs.StringVar(&c.Format, "format", "json", "output format: json or csv")
	fs.StringVar(&c.Output, "out", "", "output file ('-' for stdout; default results.<format>)")
	fs.StringVar(&c.UserAgent, "user-agent", "GoWebScraper/0.2 (student capstone project)", "User-Agent header")
	fs.Usage = func() {
		fmt.Fprintf(errOut, "Usage: scraper [flags] [url ...]\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	c.Extra = fs.Args()
	c.Format = strings.ToLower(c.Format)
	if c.Output == "" {
		c.Output = "results." + c.Format
	}
	return c, c.validate()
}

func (c Config) validate() error {
	switch {
	case c.InputFile == "" && len(c.Extra) == 0:
		return fmt.Errorf("no URLs given: use -input FILE or pass URLs as arguments")
	case c.Workers < 1 || c.Workers > 100:
		return fmt.Errorf("-workers must be between 1 and 100 (got %d)", c.Workers)
	case c.Timeout <= 0:
		return fmt.Errorf("-timeout must be positive")
	case c.Retries < 0 || c.Retries > 5:
		return fmt.Errorf("-retries must be between 0 and 5 (got %d)", c.Retries)
	case c.Format != "json" && c.Format != "csv":
		return fmt.Errorf("-format must be json or csv (got %q)", c.Format)
	}
	return nil
}

// LoadURLs reads one URL per line. Blank lines and lines starting with '#'
// are ignored. It returns the valid, de-duplicated URLs in input order and
// the lines that were rejected as invalid.
func LoadURLs(r io.Reader) (valid, rejected []string, err error) {
	seen := make(map[string]bool)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !validURL(line) {
			rejected = append(rejected, line)
			continue
		}
		if !seen[line] {
			seen[line] = true
			valid = append(valid, line)
		}
	}
	return valid, rejected, sc.Err()
}

// Normalize validates and de-duplicates URLs that were given as arguments.
func Normalize(urls []string) (valid, rejected []string) {
	valid, rejected, _ = LoadURLs(strings.NewReader(strings.Join(urls, "\n")))
	return
}

func validURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}
