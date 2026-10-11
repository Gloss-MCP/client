package plugins

import (
	"strings"
	"testing"
)

func TestAudioAcceptsAndRenders(t *testing.T) {
	a := NewAudio()
	for p, want := range map[string]bool{"a.mp3": true, "b.WAV": true, "c.ogg": true, "d.png": false, "e.mp4": false} {
		if got := a.Accepts(p); got != want {
			t.Errorf("Accepts(%q) = %v, want %v", p, got, want)
		}
	}
	views, err := a.Render(File{Path: "talks/a b.mp3", Size: 10})
	if err != nil {
		t.Fatal(err)
	}
	html := string(views[0].HTML)
	if views[0].Surface != SurfaceTime || !strings.Contains(html, "data-time-surface") ||
		!strings.Contains(html, `src="/raw/talks/a%20b.mp3"`) || !strings.Contains(html, "<audio") {
		t.Errorf("view = %+v", views[0])
	}
	var _ Streamed = a
}

func TestAudioOversizedHasNoSurface(t *testing.T) {
	views, err := NewAudio().Render(File{Path: "long.mp3", Size: MaxMediaBytes + 1})
	if err != nil {
		t.Fatal(err)
	}
	if views[0].Surface != "" || strings.Contains(string(views[0].HTML), "<audio") {
		t.Errorf("oversized audio must not be annotatable: %+v", views[0])
	}
}
