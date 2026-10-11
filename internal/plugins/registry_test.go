package plugins

import (
	"strings"
	"testing"
)

type stubPlugin struct {
	name    string
	matches func(string) bool
}

func (s stubPlugin) Name() string             { return s.name }
func (s stubPlugin) Accepts(path string) bool { return s.matches(path) }
func (s stubPlugin) Render(File) ([]View, error) {
	return []View{{HTML: "stub"}}, nil
}

func TestRegistryFirstMatchWins(t *testing.T) {
	md := stubPlugin{name: "markdown", matches: func(p string) bool { return p == "a.md" }}
	catchAll := stubPlugin{name: "plaintext", matches: func(string) bool { return true }}

	r := NewRegistry(md, catchAll)

	if got := r.For("a.md"); got == nil || got.Name() != "markdown" {
		t.Errorf("For(a.md) = %v, want markdown", got)
	}
	if got := r.For("a.txt"); got == nil || got.Name() != "plaintext" {
		t.Errorf("For(a.txt) = %v, want plaintext (catch-all)", got)
	}
}

func TestRegistryNoMatch(t *testing.T) {
	md := stubPlugin{name: "markdown", matches: func(p string) bool { return p == "a.md" }}
	r := NewRegistry(md)

	if got := r.For("a.txt"); got != nil {
		t.Errorf("For(a.txt) = %v, want nil (no plugin matches)", got)
	}
}

func TestDefaultRegistryPrecedence(t *testing.T) {
	r := NewDefaultRegistry()
	for path, want := range map[string]string{
		"README.md":       "markdown",
		"docs/a.MARKDOWN": "markdown",
		"data.json":       "json",
		"table.csv":       "csv",
		"main.go":         "code",
		"app.py":          "code",
		"notes.txt":       "code", // chroma has a plain-text lexer; still line-annotatable
		"photo.png":       "image",
		"Logo.SVG":        "image",
		"talk.mp3":        "audio",
		"Talk.WAV":        "audio",
		"Makefile":        "code",
		"mystery.xyz123":  "plaintext",
		"noextension":     "plaintext",
	} {
		got := r.For(path)
		if got == nil {
			t.Errorf("For(%q) = nil", path)
			continue
		}
		if got.Name() != want {
			t.Errorf("For(%q) = %s, want %s", path, got.Name(), want)
		}
	}
}

func TestDefaultRegistryFallsBackToPlaintextWhenOversized(t *testing.T) {
	r := NewDefaultRegistry()
	big := make([]byte, MaxRenderBytes+1)
	for i := range big {
		big[i] = 'a'
		if i%80 == 79 {
			big[i] = '\n'
		}
	}
	for _, path := range []string{"big.md", "big.json", "big.csv", "big.go"} {
		views, err := r.For(path).Render(File{Path: path, Content: big})
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if len(views) != 1 || views[0].Name != "" {
			t.Errorf("%s: oversized file rendered %d views (first %q), want single plaintext view", path, len(views), views[0].Name)
		}
		if strings.Contains(string(views[0].HTML), "gloss-md") || strings.Contains(string(views[0].HTML), `class="k`) {
			t.Errorf("%s: oversized file was not rendered as plaintext", path)
		}
	}

	// Just under the cap still uses the specific plugin.
	views, err := r.For("ok.json").Render(File{Path: "ok.json", Content: []byte(`{"a":1}`)})
	if err != nil || len(views) != 2 {
		t.Errorf("small json: err=%v views=%d, want 2", err, len(views))
	}
}
