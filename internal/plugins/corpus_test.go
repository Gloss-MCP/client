package plugins

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const corpusDir = "../../testdata/plugins"

func corpusFile(t *testing.T, name string) File {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(corpusDir, name))
	if err != nil {
		t.Fatalf("read corpus file: %v", err)
	}
	return File{Path: name, Content: b, Size: int64(len(b))}
}

// TestCorpusRendersThroughDefaultRegistry is the milestone-8 corpus check:
// every fixture picks the intended plugin, renders without error, never
// leaks unescaped markup, and exposes the annotation hooks its format
// promises.
func TestCorpusRendersThroughDefaultRegistry(t *testing.T) {
	reg := NewDefaultRegistry()
	tests := []struct {
		file       string
		plugin     string
		views      []string
		mustHave   []string // in the first (default) view
		mustNotHas []string // in every view
	}{
		{"code.go", "code", []string{""}, []string{`data-line="1"`, `data-line="12"`, `<span class="`}, []string{`data-line="13"`}},
		{"crlf.py", "code", []string{""}, []string{`data-line="4"`}, []string{`data-line="5"`}},
		{"doc.md", "markdown", []string{"Rendered", "Source"}, []string{`class="gloss-md"`, `<h1>`, `<table>`, `<li class="gloss-md-block"`}, []string{"<script>"}},
		{"config.json", "json", []string{"Raw", "Formatted"}, []string{`data-line="1"`}, nil},
		{"broken.json", "json", []string{""}, []string{"Not valid JSON", `data-line="4"`}, nil},
		{"prices.csv", "csv", []string{"Table", "Source"}, []string{`data-col="4"`, `data-line="3" data-line-end="4"`}, []string{"<b>bold</b>"}},
		{"ragged.csv", "csv", []string{"Table", "Source"}, []string{`data-col="4"`}, nil},
		{"empty.py", "code", []string{""}, nil, []string{`data-line=`}},
		{"pixel.png", "image", []string{""}, []string{`data-region-surface`, `src="/raw/pixel.png"`}, []string{`data-line=`}},
		{"tone.wav", "audio", []string{""}, []string{`data-time-surface`, `src="/raw/tone.wav"`}, []string{`data-line=`, `data-region-surface`}},
		{"logo.svg", "image", []string{""}, []string{`data-region-surface`, `src="/raw/logo.svg"`}, []string{"<script", "<svg", `data-line=`}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			f := corpusFile(t, tt.file)
			p := reg.For(f.Path)
			if p == nil || p.Name() != tt.plugin {
				t.Fatalf("plugin = %v, want %s", p, tt.plugin)
			}
			views, err := p.Render(f)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if len(views) != len(tt.views) {
				t.Fatalf("views = %d, want %d", len(views), len(tt.views))
			}
			for i, v := range views {
				if v.Name != tt.views[i] {
					t.Errorf("view %d name = %q, want %q", i, v.Name, tt.views[i])
				}
				for _, bad := range tt.mustNotHas {
					if strings.Contains(string(v.HTML), bad) {
						t.Errorf("view %q contains forbidden %q", v.Name, bad)
					}
				}
			}
			first := string(views[0].HTML)
			for _, want := range tt.mustHave {
				if !strings.Contains(first, want) {
					t.Errorf("first view missing %q:\n%s", want, first)
				}
			}
		})
	}
}

// TestCorpusSourceViewsAgreeOnLineCount guards the shared line-numbering
// contract: every plugin's line-addressable view of a file numbers the
// same lines plaintext does, so anchors survive switching views/plugins.
func TestCorpusSourceViewsAgreeOnLineCount(t *testing.T) {
	reg := NewDefaultRegistry()
	attr := regexp.MustCompile(`data-line="\d+"`)
	for _, name := range []string{"code.go", "crlf.py", "empty.py", "config.json", "broken.json", "prices.csv", "ragged.csv", "doc.md"} {
		t.Run(name, func(t *testing.T) {
			f := corpusFile(t, name)
			want := len(attr.FindAllString(string(mustRender(t, NewPlaintext(), f)[0].HTML), -1))
			views, err := reg.For(name).Render(f)
			if err != nil {
				t.Fatal(err)
			}
			src := views[len(views)-1] // Source / Raw view, or the only view
			if name == "config.json" {
				src = views[0] // Raw
			}
			if got := len(attr.FindAllString(string(src.HTML), -1)); got != want {
				t.Errorf("line-addressable elements = %d, plaintext has %d", got, want)
			}
		})
	}
}

func mustRender(t *testing.T, p Plugin, f File) []View {
	t.Helper()
	v, err := p.Render(f)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
