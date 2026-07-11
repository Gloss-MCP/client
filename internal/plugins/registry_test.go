package plugins

import "testing"

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
