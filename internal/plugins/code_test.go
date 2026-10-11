package plugins

import (
	"fmt"
	"strings"
	"testing"
)

func TestCodeAccepts(t *testing.T) {
	c := NewCode()
	for path, want := range map[string]bool{
		"main.go": true, "a/b/app.py": true, "x.rs": true, "script.js": true,
		"data.unknownext": false, "no-extension-file": false,
	} {
		if got := c.Accepts(path); got != want {
			t.Errorf("Accepts(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestCodeHighlightsTokens(t *testing.T) {
	views, err := NewCode().Render(File{Path: "main.go", Content: []byte("package main\n")})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	html := string(views[0].HTML)
	if !strings.Contains(html, `<span class="`) {
		t.Errorf("expected highlighted token spans: %s", html)
	}
	if !strings.Contains(html, `data-line="1"`) {
		t.Errorf("missing data-line=1: %s", html)
	}
}

func TestCodeMultiLineTokenKeepsLineNumbers(t *testing.T) {
	src := "/* one\n two\n three */\nvar x = 1\n"
	views, err := NewCode().Render(File{Path: "a.go", Content: []byte(src)})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	html := string(views[0].HTML)
	for n := 1; n <= 4; n++ {
		if !strings.Contains(html, fmt.Sprintf(`data-line="%d"`, n)) {
			t.Errorf("missing data-line=%d: %s", n, html)
		}
	}
	if strings.Contains(html, `data-line="5"`) {
		t.Errorf("trailing newline produced a phantom line: %s", html)
	}
}

func TestCodeLineCountMatchesPlaintext(t *testing.T) {
	for name, src := range map[string]string{
		"trailing newline": "a := 1\nb := 2\n",
		"no trailing":      "a := 1\nb := 2",
		"blank lines":      "a := 1\n\n\nb := 2\n",
		"crlf":             "a := 1\r\nb := 2\r\n",
		"unicode":          "s := \"héllo ✓\"\n",
		"single":           "x",
	} {
		t.Run(name, func(t *testing.T) {
			f := File{Path: "a.go", Content: []byte(src)}
			cv, err := NewCode().Render(f)
			if err != nil {
				t.Fatal(err)
			}
			pv, _ := NewPlaintext().Render(f)
			cn := strings.Count(string(cv[0].HTML), "data-line=")
			pn := strings.Count(string(pv[0].HTML), "data-line=")
			if cn != pn {
				t.Errorf("code lines = %d, plaintext lines = %d", cn, pn)
			}
		})
	}
}

func TestCodeEscapesHostileContent(t *testing.T) {
	src := "x := \"</span><script>alert(1)</script>\"\n"
	views, err := NewCode().Render(File{Path: "a.go", Content: []byte(src)})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(views[0].HTML), "<script>") {
		t.Errorf("unescaped script tag: %s", views[0].HTML)
	}
}

func TestCodeEmptyAndBinary(t *testing.T) {
	if _, err := NewCode().Render(File{Path: "a.go"}); err != nil {
		t.Errorf("empty: %v", err)
	}
	views, err := NewCode().Render(File{Path: "a.go", Content: []byte("a\x00b")})
	if err != nil || !strings.Contains(string(views[0].HTML), "Binary file not shown") {
		t.Errorf("binary: err=%v views=%v", err, views)
	}
}
