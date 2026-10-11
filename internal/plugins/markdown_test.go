package plugins

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var blockRe = regexp.MustCompile(`data-line="(\d+)" data-line-end="(\d+)"`)

// spans extracts the (start,end) pairs of the Rendered view's blocks.
func spans(t *testing.T, src string) [][2]int {
	t.Helper()
	views, err := NewMarkdown().Render(File{Path: "a.md", Content: []byte(src)})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(views) != 2 || views[0].Name != "Rendered" || views[1].Name != "Source" {
		t.Fatalf("views = %+v, want Rendered then Source", views)
	}
	var out [][2]int
	for _, m := range blockRe.FindAllStringSubmatch(string(views[0].HTML), -1) {
		s, _ := strconv.Atoi(m[1])
		e, _ := strconv.Atoi(m[2])
		out = append(out, [2]int{s, e})
	}
	return out
}

func TestMarkdownAccepts(t *testing.T) {
	m := NewMarkdown()
	for path, want := range map[string]bool{"a.md": true, "B.MARKDOWN": true, "a.txt": false, "a.mdx": false} {
		if got := m.Accepts(path); got != want {
			t.Errorf("Accepts(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestMarkdownBlockSpans(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want [][2]int
	}{
		{"heading and paragraphs", "# Title\n\npara one\nstill one\n\npara two\n", [][2]int{{1, 1}, {3, 4}, {6, 6}}},
		{"no trailing newline", "# Title\n\nbody", [][2]int{{1, 1}, {3, 3}}},
		{"fenced code includes fences", "intro\n\n```go\nx := 1\ny := 2\n```\n\nafter\n", [][2]int{{1, 1}, {3, 6}, {8, 8}}},
		{"tilde fence no info", "~~~\ncode\n~~~\n", [][2]int{{1, 3}}},
		{"setext heading includes underline", "Title\n=====\n\ntext\n", [][2]int{{1, 2}, {4, 4}}},
		{"thematic break", "above\n\n---\n\nbelow\n", [][2]int{{1, 1}, {3, 3}, {5, 5}}},
		{"blockquote", "> a\n> b\n\nz\n", [][2]int{{1, 2}, {4, 4}}},
		{"list items are separate blocks", "- one\n- two\n- three\n", [][2]int{{1, 1}, {2, 2}, {3, 3}}},
		{"nested list stays in parent item", "- a\n  - b\n- c\n", [][2]int{{1, 2}, {3, 3}}},
		{"multi-line list item", "1. first\n   cont\n2. second\n", [][2]int{{1, 2}, {3, 3}}},
		{"crlf", "# T\r\n\r\ntext\r\n", [][2]int{{1, 1}, {3, 3}}},
		{"table", "| a | b |\n|---|---|\n| 1 | 2 |\n", [][2]int{{1, 3}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := spans(t, tt.src)
			if len(got) != len(tt.want) {
				t.Fatalf("spans = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("spans = %v, want %v", got, tt.want)
					break
				}
			}
		})
	}
}

func TestMarkdownSourceViewIsLineAnnotatable(t *testing.T) {
	views, _ := NewMarkdown().Render(File{Path: "a.md", Content: []byte("# a\n\nb\n")})
	if !strings.Contains(string(views[1].HTML), `data-line="3"`) {
		t.Errorf("Source view missing data-line: %s", views[1].HTML)
	}
}

func TestMarkdownOmitsRawHTMLAndUnsafeLinks(t *testing.T) {
	src := "<script>alert(1)</script>\n\n[x](javascript:alert(1))\n\n<img src=x onerror=alert(1)>\n"
	views, err := NewMarkdown().Render(File{Path: "a.md", Content: []byte(src)})
	if err != nil {
		t.Fatal(err)
	}
	html := string(views[0].HTML)
	for _, bad := range []string{"<script>", "javascript:", "<img src=x"} {
		if strings.Contains(html, bad) {
			t.Errorf("Rendered view contains %q: %s", bad, html)
		}
	}
}

func TestMarkdownEmptyAndBinary(t *testing.T) {
	if _, err := NewMarkdown().Render(File{Path: "a.md"}); err != nil {
		t.Errorf("empty: %v", err)
	}
	views, err := NewMarkdown().Render(File{Path: "a.md", Content: []byte("a\x00b")})
	if err != nil || len(views) != 1 || !strings.Contains(string(views[0].HTML), "Binary") {
		t.Errorf("binary: err=%v views=%v", err, views)
	}
}
