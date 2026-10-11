package plugins

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
)

// linesTemplate renders one element per line, each carrying a data-line
// attribute. This is the line-addressing convention any plugin that
// supports line anchors (docs/data-model.md#anchor) is expected to
// follow: the UI layer selects and highlights ranges generically over
// [data-line] elements, independent of which plugin produced them.
var linesTemplate = template.Must(template.New("lines").Parse(`<div class="gloss-code font-mono text-xs leading-5">
{{- range .}}
<div class="gloss-line flex" data-line="{{.N}}"><span class="gloss-line-no w-10 shrink-0 select-none text-right pr-3 text-gray-400">{{.N}}</span><span class="gloss-line-content whitespace-pre-wrap break-all">{{.HTML}}</span></div>
{{- end}}
</div>`))

// renderedLine is one 1-indexed line of a file whose content is already
// safe HTML (plaintext escapes it; code highlighting emits token spans).
type renderedLine struct {
	N    int
	HTML template.HTML
}

// renderLines renders lines in the shared line-addressable markup.
func renderLines(lines []renderedLine) (template.HTML, error) {
	var buf bytes.Buffer
	if err := linesTemplate.Execute(&buf, lines); err != nil {
		return "", fmt.Errorf("plugins: render lines: %w", err)
	}
	return template.HTML(buf.String()), nil
}

// splitLines splits content into lines. A trailing newline does not
// produce a phantom empty final line, matching how line numbers are
// counted in editors and in LineAnchor.
func splitLines(content []byte) []string {
	s := strings.TrimSuffix(string(content), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
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
