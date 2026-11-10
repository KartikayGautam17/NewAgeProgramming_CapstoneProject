// Package parser extracts the fields we care about from an HTML document
// using the streaming tokenizer from golang.org/x/net/html.
package parser

import (
	"io"
	"strings"

	"golang.org/x/net/html"
)

// Page holds the data extracted from one HTML document.
type Page struct {
	Title        string
	MetaDesc     string
	LinkCount    int // <a> elements that have an href attribute
	HeadingCount int // <h1> .. <h6> elements
}

// Parse reads HTML from r and returns the extracted fields. Malformed HTML is
// tolerated (the tokenizer is forgiving); only genuine read errors are returned.
func Parse(r io.Reader) (Page, error) {
	var p Page
	z := html.NewTokenizer(r)
	inTitle, haveTitle := false, false
	var title strings.Builder

	for {
		switch z.Next() {
		case html.ErrorToken:
			if err := z.Err(); err != io.EOF {
				return p, err
			}
			p.Title = collapse(title.String())
			return p, nil

		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			tag := string(name)
			switch tag {
			case "title":
				if !haveTitle {
					inTitle = true
				}
			case "h1", "h2", "h3", "h4", "h5", "h6":
				p.HeadingCount++
			case "a":
				if hasAttr && hasHref(z) {
					p.LinkCount++
				}
			case "meta":
				if hasAttr && p.MetaDesc == "" {
					if d, ok := metaDescription(z); ok {
						p.MetaDesc = d
					}
				}
			}

		case html.TextToken:
			if inTitle {
				title.Write(z.Text())
			}

		case html.EndTagToken:
			name, _ := z.TagName()
			if string(name) == "title" && inTitle {
				inTitle, haveTitle = false, true
			}
		}
	}
}

func hasHref(z *html.Tokenizer) bool {
	for {
		key, val, more := z.TagAttr()
		if string(key) == "href" && strings.TrimSpace(string(val)) != "" {
			return true
		}
		if !more {
			return false
		}
	}
}

func metaDescription(z *html.Tokenizer) (string, bool) {
	var name, content string
	for {
		key, val, more := z.TagAttr()
		switch strings.ToLower(string(key)) {
		case "name":
			name = strings.ToLower(strings.TrimSpace(string(val)))
		case "content":
			content = string(val)
		}
		if !more {
			break
		}
	}
	if name == "description" {
		return collapse(content), true
	}
	return "", false
}

// collapse trims s and squeezes runs of whitespace into single spaces.
func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }
