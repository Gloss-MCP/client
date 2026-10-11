package plugins

import (
	"bytes"
	"fmt"
	"html/template"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

// Markdown renders .md files in two views: "Rendered" (goldmark HTML) and
// "Source" (highlighted, via the code plugin). Both are line-annotatable:
// in the Rendered view every top-level block -- and every list item --
// is wrapped in an element carrying data-line (first source line) and
// data-line-end (last source line), so a selection anchors to the source
// lines of the block it covers.
//
// Raw HTML in the markdown source is omitted and unsafe URLs are
// neutralised (goldmark's defaults) because the content is AI-generated.
type Markdown struct {
	md   goldmark.Markdown
	code Code
}

// NewMarkdown constructs the markdown plugin.
func NewMarkdown() Markdown {
	return Markdown{
		md:   goldmark.New(goldmark.WithExtensions(extension.GFM)),
		code: NewCode(),
	}
}

// Name implements Plugin.
func (Markdown) Name() string { return "markdown" }

// Accepts implements Plugin.
func (Markdown) Accepts(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".md", ".markdown":
		return true
	}
	return false
}

// Render implements Plugin.
func (m Markdown) Render(f File) ([]View, error) {
	if isBinary(f.Content) {
		return NewPlaintext().Render(f)
	}
	source, err := m.code.Render(f)
	if err != nil {
		return nil, err
	}
	rendered, err := m.renderBlocks(f.Content)
	if err != nil {
		return nil, fmt.Errorf("plugins: render markdown %s: %w", f.Path, err)
	}
	return []View{
		{Name: "Rendered", HTML: rendered},
		{Name: "Source", HTML: source[0].HTML},
	}, nil
}

func (m Markdown) renderBlocks(src []byte) (template.HTML, error) {
	doc := m.md.Parser().Parse(text.NewReader(src))
	lm := newLineMap(src)

	var buf bytes.Buffer
	buf.WriteString(`<div class="gloss-md">`)
	prevEnd := 0
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		start, end := blockSpan(n, src, lm, prevEnd)
		prevEnd = end
		if list, ok := n.(*ast.List); ok {
			if err := m.renderList(&buf, list, src, lm, start); err != nil {
				return "", err
			}
			continue
		}
		fmt.Fprintf(&buf, `<div class="gloss-md-block" data-line="%d" data-line-end="%d">`, start, end)
		if err := m.md.Renderer().Render(&buf, src, n); err != nil {
			return "", err
		}
		buf.WriteString("</div>")
	}
	buf.WriteString("</div>")
	return template.HTML(buf.String()), nil
}

// renderList renders a list item by item so each item is its own
// annotatable block. The list wrapper is rendered by hand; the item
// bodies by goldmark.
func (m Markdown) renderList(buf *bytes.Buffer, list *ast.List, src []byte, lm lineMap, listStart int) error {
	tag := "ul"
	if list.IsOrdered() {
		tag = "ol"
	}
	fmt.Fprintf(buf, "<%s", tag)
	if list.IsOrdered() && list.Start != 1 {
		fmt.Fprintf(buf, ` start="%d"`, list.Start)
	}
	buf.WriteString(">\n")
	prevEnd := listStart - 1
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		start, end := blockSpan(item, src, lm, prevEnd)
		prevEnd = end
		var li bytes.Buffer
		if err := m.md.Renderer().Render(&li, src, item); err != nil {
			return err
		}
		s := li.String()
		if !strings.HasPrefix(s, "<li") {
			return fmt.Errorf("unexpected list item markup %q", s)
		}
		fmt.Fprintf(buf, `<li class="gloss-md-block" data-line="%d" data-line-end="%d"%s`, start, end, s[len("<li"):])
	}
	fmt.Fprintf(buf, "</%s>\n", tag)
	return nil
}

// lineMap converts byte offsets in the source to 1-indexed line numbers.
type lineMap struct {
	src    []byte
	starts []int // byte offset of the start of each line
}

func newLineMap(src []byte) lineMap {
	starts := []int{0}
	for i, b := range src {
		if b == '\n' && i+1 < len(src) {
			starts = append(starts, i+1)
		}
	}
	return lineMap{src: src, starts: starts}
}

func (lm lineMap) count() int { return len(lm.starts) }

// lineOf returns the 1-indexed line containing byte offset off.
func (lm lineMap) lineOf(off int) int {
	return sort.Search(len(lm.starts), func(i int) bool { return lm.starts[i] > off })
}

// text returns line n (1-indexed) without its terminator, or "" if out
// of range.
func (lm lineMap) text(n int) string {
	if n < 1 || n > len(lm.starts) {
		return ""
	}
	s := lm.starts[n-1]
	e := len(lm.src)
	if n < len(lm.starts) {
		e = lm.starts[n] - 1
	}
	return strings.TrimRight(string(lm.src[s:e]), "\r")
}

var (
	fenceRe  = regexp.MustCompile("^\\s{0,3}(```|~~~)")
	setextRe = regexp.MustCompile(`^\s{0,3}(=+|-+)\s*$`)
)

// blockSpan returns the 1-indexed first and last source line of block n.
// prevEnd is the last line of the preceding sibling, used to place blocks
// that carry no source segment of their own (thematic breaks).
func blockSpan(n ast.Node, src []byte, lm lineMap, prevEnd int) (int, int) {
	start, end := rawSpan(n, lm)
	if start == 0 {
		// No segment: first non-blank line after the previous block.
		start = prevEnd + 1
		for start < lm.count() && strings.TrimSpace(lm.text(start)) == "" {
			start++
		}
		end = start
	}
	if start > lm.count() {
		start = lm.count()
	}
	if end < start {
		end = start
	}
	if end > lm.count() {
		end = lm.count()
	}
	return start, end
}

// rawSpan derives a node's line span from its segments and its
// descendants. It returns 0,0 when the node has none.
func rawSpan(n ast.Node, lm lineMap) (int, int) {
	start, end := 0, 0
	extend := func(s, e int) {
		if s == 0 {
			return
		}
		if start == 0 || s < start {
			start = s
		}
		if e > end {
			end = e
		}
	}

	switch b := n.(type) {
	case *ast.FencedCodeBlock:
		// Lines() excludes the fence lines; the span includes them.
		if b.Info != nil {
			s := lm.lineOf(b.Info.Segment.Start)
			extend(s, s)
		}
		if l := b.Lines(); l.Len() > 0 {
			first := lm.lineOf(l.At(0).Start)
			last := lm.lineOf(max(l.At(l.Len()-1).Stop-1, 0))
			if b.Info == nil {
				extend(first-1, first-1)
			}
			extend(first, last)
			if fenceRe.MatchString(lm.text(last + 1)) {
				extend(last+1, last+1)
			}
		} else if start != 0 && fenceRe.MatchString(lm.text(start+1)) {
			extend(start+1, start+1)
		}
		return start, end
	case *ast.HTMLBlock:
		if l := b.Lines(); l.Len() > 0 {
			extend(lm.lineOf(l.At(0).Start), lm.lineOf(max(l.At(l.Len()-1).Stop-1, 0)))
		}
		if b.HasClosure() {
			extend(lm.lineOf(b.ClosureLine.Start), lm.lineOf(b.ClosureLine.Start))
		}
		return start, end
	}

	if l := n.Lines(); l != nil && l.Len() > 0 {
		extend(lm.lineOf(l.At(0).Start), lm.lineOf(max(l.At(l.Len()-1).Stop-1, 0)))
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if c.Type() == ast.TypeBlock {
			extend(rawSpan(c, lm))
		}
	}

	// A setext heading's underline is not part of its segment.
	if _, ok := n.(*ast.Heading); ok && start != 0 {
		if !strings.HasPrefix(strings.TrimSpace(lm.text(start)), "#") && setextRe.MatchString(lm.text(end+1)) {
			extend(end+1, end+1)
		}
	}
	return start, end
}
