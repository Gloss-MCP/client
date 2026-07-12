package plugins

import (
	"fmt"
	"strings"
	"testing"
)

func TestPlaintextAcceptsEverything(t *testing.T) {
	p := NewPlaintext()
	for _, path := range []string{"a.txt", "a.go", "a.png", "no-extension", ""} {
		if !p.Accepts(path) {
			t.Errorf("Accepts(%q) = false, want true (plaintext is the catch-all)", path)
		}
	}
}

func TestPlaintextRendersEscapedText(t *testing.T) {
	p := NewPlaintext()
	views, err := p.Render(File{Path: "a.txt", Content: []byte("<script>alert(1)</script>")})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("len(views) = %d, want 1", len(views))
	}
	html := string(views[0].HTML)
	if wantContains := "&lt;script&gt;"; !strings.Contains(html, wantContains) {
		t.Errorf("HTML = %q, want it to contain escaped %q", html, wantContains)
	}
	if strings.Contains(html, "<script>") {
		t.Errorf("HTML = %q, must not contain an unescaped <script> tag", html)
	}
}

func TestPlaintextDetectsBinaryContent(t *testing.T) {
	p := NewPlaintext()
	content := append([]byte("PK\x03\x04"), 0x00, 0x01, 0x02)
	views, err := p.Render(File{Path: "a.bin", Content: content})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	html := string(views[0].HTML)
	if !strings.Contains(html, "Binary file not shown") {
		t.Errorf("HTML = %q, want the binary placeholder", html)
	}
}

func TestPlaintextRendersEmptyFile(t *testing.T) {
	p := NewPlaintext()
	views, err := p.Render(File{Path: "empty.txt", Content: nil})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(string(views[0].HTML), "Binary file not shown") {
		t.Errorf("empty file must not be treated as binary")
	}
}

func TestPlaintextWrapsEachLineWithDataLine(t *testing.T) {
	p := NewPlaintext()
	views, err := p.Render(File{Path: "a.txt", Content: []byte("one\ntwo\nthree\n")})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	html := string(views[0].HTML)
	for n, want := range map[int]string{1: "one", 2: "two", 3: "three"} {
		wantAttr := fmt.Sprintf(`data-line="%d"`, n)
		if !strings.Contains(html, wantAttr) {
			t.Errorf("HTML missing %q: %s", wantAttr, html)
		}
		if !strings.Contains(html, want) {
			t.Errorf("HTML missing line content %q: %s", want, html)
		}
	}
	if strings.Contains(html, `data-line="4"`) {
		t.Errorf("trailing newline must not produce a phantom fourth line: %s", html)
	}
}

func TestPlaintextNoTrailingNewlineStillCountsLastLine(t *testing.T) {
	p := NewPlaintext()
	views, err := p.Render(File{Path: "a.txt", Content: []byte("one\ntwo")})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	html := string(views[0].HTML)
	if !strings.Contains(html, `data-line="2"`) {
		t.Errorf("HTML missing data-line=\"2\" for content with no trailing newline: %s", html)
	}
}
