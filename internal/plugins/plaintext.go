package plugins

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
)

// binarySniffLen is how many leading bytes Plaintext inspects to decide
// whether content is binary -- the same heuristic git and file(1) use (a
// NUL byte in the leading chunk).
const binarySniffLen = 8000

// linesTemplate renders one element per line, each carrying a data-line
// attribute. This is the line-addressing convention any plugin that
// supports line anchors (docs/data-model.md#anchor) is expected to
// follow: the UI layer selects and highlights ranges generically over
// [data-line] elements, independent of which plugin produced them.
var linesTemplate = template.Must(template.New("lines").Parse(`<div class="gloss-code font-mono text-xs leading-5">
{{- range .Lines}}
<div class="gloss-line flex" data-line="{{.N}}"><span class="gloss-line-no w-10 shrink-0 select-none text-right pr-3 text-gray-400">{{.N}}</span><span class="gloss-line-content whitespace-pre-wrap break-all">{{.Text}}</span></div>
{{- end}}
</div>`))

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
	var buf bytes.Buffer
	if isBinary(f.Content) {
		if err := binaryTemplate.Execute(&buf, nil); err != nil {
			return nil, fmt.Errorf("plugins: render plaintext %s: %w", f.Path, err)
		}
		return []View{{HTML: template.HTML(buf.String())}}, nil
	}

	if err := linesTemplate.Execute(&buf, struct{ Lines []line }{splitLines(f.Content)}); err != nil {
		return nil, fmt.Errorf("plugins: render plaintext %s: %w", f.Path, err)
	}
	return []View{{HTML: template.HTML(buf.String())}}, nil
}

// line is one 1-indexed line of a rendered file.
type line struct {
	N    int
	Text string
}

// splitLines splits content into 1-indexed lines. A trailing newline does
// not produce a phantom empty final line, matching how line numbers are
// counted in editors and in LineAnchor.
func splitLines(content []byte) []line {
	s := strings.TrimSuffix(string(content), "\n")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "\n")
	lines := make([]line, len(parts))
	for i, p := range parts {
		lines[i] = line{N: i + 1, Text: p}
	}
	return lines
}

// isBinary reports whether content looks like binary data: a NUL byte
// within the first binarySniffLen bytes.
func isBinary(content []byte) bool {
	n := len(content)
	if n > binarySniffLen {
		n = binarySniffLen
	}
	return bytes.IndexByte(content[:n], 0) != -1
}
