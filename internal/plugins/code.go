package plugins

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// Code renders source files with server-side syntax highlighting (chroma).
// Highlighting is tokenised for the whole file and then split per line,
// so tokens that span lines (block comments, raw strings) still yield one
// correctly-numbered [data-line] element per source line.
type Code struct {
	formatter *chromahtml.Formatter
}

// NewCode constructs the code plugin.
func NewCode() Code {
	return Code{formatter: chromahtml.New(
		chromahtml.WithClasses(true),
		chromahtml.PreventSurroundingPre(true),
	)}
}

// Name implements Plugin.
func (Code) Name() string { return "code" }

// Accepts implements Plugin: any path chroma has a lexer for.
func (Code) Accepts(path string) bool { return lexers.Match(path) != nil }

// Render implements Plugin. Binary content gets the plaintext
// placeholder; a missing lexer falls back to plaintext rendering.
func (c Code) Render(f File) ([]View, error) {
	lexer := lexers.Match(f.Path)
	if lexer == nil || isBinary(f.Content) {
		return NewPlaintext().Render(f)
	}
	lexer = chroma.Coalesce(lexer)

	it, err := lexer.Tokenise(nil, string(f.Content))
	if err != nil {
		return nil, fmt.Errorf("plugins: tokenise %s: %w", f.Path, err)
	}
	perLine := chroma.SplitTokensIntoLines(it.Tokens())

	// Drop the empty remainder after a trailing newline, mirroring
	// splitLines so line numbers agree across plugins.
	if n := len(perLine); n > 0 && lineIsEmpty(perLine[n-1]) {
		perLine = perLine[:n-1]
	}

	lines := make([]renderedLine, len(perLine))
	for i, toks := range perLine {
		html, err := c.formatLine(toks)
		if err != nil {
			return nil, fmt.Errorf("plugins: highlight %s: %w", f.Path, err)
		}
		lines[i] = renderedLine{N: i + 1, HTML: html}
	}
	html, err := renderLines(lines)
	if err != nil {
		return nil, fmt.Errorf("plugins: render code %s: %w", f.Path, err)
	}
	return []View{{HTML: html}}, nil
}

// formatLine renders one line's tokens as class-annotated spans,
// stripping the line terminator chroma leaves on the final token.
func (c Code) formatLine(toks []chroma.Token) (template.HTML, error) {
	if n := len(toks); n > 0 {
		last := toks[n-1]
		last.Value = strings.TrimSuffix(last.Value, "\n")
		toks = append(append([]chroma.Token(nil), toks[:n-1]...), last)
	}
	var buf bytes.Buffer
	if err := c.formatter.Format(&buf, styles.Fallback, chroma.Literator(toks...)); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

func lineIsEmpty(toks []chroma.Token) bool {
	for _, t := range toks {
		if strings.TrimSuffix(t.Value, "\n") != "" {
			return false
		}
	}
	return true
}
