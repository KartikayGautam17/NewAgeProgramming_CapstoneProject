package parser

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		html string
		want Page
	}{
		{
			name: "full page",
			html: `<html><head><title>  My   Page </title>
				<meta name="description" content="A  short   description"></head>
				<body><h1>A</h1><h2>B</h2><a href="/x">x</a><a href="https://y.com">y</a></body></html>`,
			want: Page{Title: "My Page", MetaDesc: "A short description", LinkCount: 2, HeadingCount: 2},
		},
		{name: "empty", html: "", want: Page{}},
		{name: "no title or meta", html: "<p>hello</p>", want: Page{}},
		{
			name: "anchor without href is not a link",
			html: `<a name="top">anchor</a><a href="">empty</a><a href="/ok">ok</a>`,
			want: Page{LinkCount: 1},
		},
		{
			name: "only first title counts",
			html: `<title>First</title><title>Second</title>`,
			want: Page{Title: "First"},
		},
		{
			name: "meta description is case-insensitive and other metas ignored",
			html: `<meta name="keywords" content="a,b"><meta NAME="Description" content="desc">`,
			want: Page{MetaDesc: "desc"},
		},
		{
			name: "all heading levels",
			html: `<h1>1</h1><h2>2</h2><h3>3</h3><h4>4</h4><h5>5</h5><h6>6</h6><h7>no</h7>`,
			want: Page{HeadingCount: 6},
		},
		{
			name: "links inside script text are ignored",
			html: `<script>var s = "<a href='/fake'>";</script><a href="/real">r</a>`,
			want: Page{LinkCount: 1},
		},
		{
			name: "malformed html does not fail",
			html: `<html><head><title>T</title><body><h1>Unclosed<a href="/a">`,
			want: Page{Title: "T", HeadingCount: 1, LinkCount: 1},
		},
		{
			name: "html entities decoded in title",
			html: `<title>Tom &amp; Jerry</title>`,
			want: Page{Title: "Tom & Jerry"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tc.html))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
