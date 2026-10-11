package plugins

import (
	"bytes"
	"fmt"
	"html/template"
)

// binarySniffLen is how many leading bytes Plaintext inspects to decide
// whether content is binary -- the same heuristic git and file(1) use (a
// NUL byte in the leading chunk).
const binarySniffLen = 8000

var binaryTemplate = template.Must(template.New("binary").Parse(
	`<p class="gloss-binary">Binary file not shown.</p>`,
))

// Plaintext is the catch-all Plugin: it renders any file as escaped text,
// one line-addressable element per line, or a placeholder for binary
// content.
type Plaintext struct{}

// NewPlaintext constructs the plaintext plugin.
func NewPlaintext() Plaintext { return Plaintext{} }

// Name implements Plugin.
func (Plaintext) Name() string { return "plaintext" }

// Accepts implements Plugin. Plaintext is the catch-all: it accepts
// every path.
func (Plaintext) Accepts(string) bool { return true }

// Render implements Plugin.
func (Plaintext) Render(f File) ([]View, error) {
	if isBinary(f.Content) {
		var buf bytes.Buffer
		if err := binaryTemplate.Execute(&buf, nil); err != nil {
			return nil, fmt.Errorf("plugins: render plaintext %s: %w", f.Path, err)
		}
		return []View{{HTML: template.HTML(buf.String())}}, nil
	}

	texts := splitLines(f.Content)
	lines := make([]renderedLine, len(texts))
	for i, t := range texts {
		lines[i] = renderedLine{N: i + 1, HTML: template.HTML(template.HTMLEscapeString(t))}
	}
	html, err := renderLines(lines)
	if err != nil {
		return nil, fmt.Errorf("plugins: render plaintext %s: %w", f.Path, err)
	}
	return []View{{HTML: html}}, nil
}
