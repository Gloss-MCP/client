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

var plaintextTemplate = template.Must(template.New("plaintext").Parse(
	`{{if .Binary}}<p class="gloss-binary">Binary file not shown.</p>{{else}}<pre class="gloss-plaintext"><code>{{.Content}}</code></pre>{{end}}`,
))

// Plaintext is the catch-all Plugin: it renders any file as escaped text,
// or a placeholder for binary content.
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
	data := struct {
		Binary  bool
		Content string
	}{
		Binary:  isBinary(f.Content),
		Content: string(f.Content),
	}

	var buf bytes.Buffer
	if err := plaintextTemplate.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("plugins: render plaintext %s: %w", f.Path, err)
	}
	return []View{{HTML: template.HTML(buf.String())}}, nil
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
