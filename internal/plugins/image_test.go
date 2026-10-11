package plugins

import (
	"strings"
	"testing"
)

func TestImageAccepts(t *testing.T) {
	img := NewImage()
	for p, want := range map[string]bool{
		"a.png": true, "dir/b.JPEG": true, "c.gif": true, "d.webp": true, "e.SVG": true,
		"f.mp3": false, "g.txt": false, "png": false,
	} {
		if got := img.Accepts(p); got != want {
			t.Errorf("Accepts(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestImageRenderBuildsSurfaceAndEscapesPath(t *testing.T) {
	views, err := NewImage().Render(File{Path: `weird dir/a"b<c>?#.png`, Size: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].Surface != SurfaceRegion {
		t.Fatalf("views = %+v, want one region-surface view", views)
	}
	html := string(views[0].HTML)
	for _, want := range []string{
		`data-region-surface`,
		`src="/raw/weird%20dir/a%22b%3Cc%3E%3F%23.png"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in:\n%s", want, html)
		}
	}
	// The alt text carries the path; it must be attribute-escaped.
	if strings.Contains(html, `a"b<c>`) {
		t.Errorf("unescaped path in markup:\n%s", html)
	}
}

func TestImageOversizedHasNoSurface(t *testing.T) {
	views, err := NewImage().Render(File{Path: "huge.png", Size: MaxMediaBytes + 1})
	if err != nil {
		t.Fatal(err)
	}
	html := string(views[0].HTML)
	if views[0].Surface != "" || strings.Contains(html, "data-region-surface") || strings.Contains(html, "<img") {
		t.Errorf("oversized image must not be annotatable:\n%s", html)
	}
	if !strings.Contains(html, "too large") {
		t.Errorf("missing too-large note:\n%s", html)
	}
}

func TestImageIsStreamed(t *testing.T) {
	var _ Streamed = NewImage()
}
