package plugins

import (
	"strings"
	"testing"
)

func TestVideoAcceptsAndRenders(t *testing.T) {
	v := NewVideo()
	for p, want := range map[string]bool{"a.mp4": true, "b.WEBM": true, "c.mov": true, "d.mp3": false, "e.png": false} {
		if got := v.Accepts(p); got != want {
			t.Errorf("Accepts(%q) = %v, want %v", p, got, want)
		}
	}
	views, err := v.Render(File{Path: "demos/a b.webm", Size: 10})
	if err != nil {
		t.Fatal(err)
	}
	html := string(views[0].HTML)
	if views[0].Surface != SurfaceRegionTime || !strings.Contains(html, "data-region-surface") ||
		!strings.Contains(html, `src="/raw/demos/a%20b.webm"`) || !strings.Contains(html, "<video") {
		t.Errorf("view = %+v", views[0])
	}
	var _ Streamed = v
}

func TestVideoOversizedHasNoSurface(t *testing.T) {
	views, err := NewVideo().Render(File{Path: "big.mp4", Size: MaxMediaBytes + 1})
	if err != nil {
		t.Fatal(err)
	}
	if views[0].Surface != "" || strings.Contains(string(views[0].HTML), "<video") {
		t.Errorf("oversized video must not be annotatable: %+v", views[0])
	}
}
