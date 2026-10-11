package plugins

import "testing"

func TestMediaKindAndContentType(t *testing.T) {
	for _, tt := range []struct {
		path string
		kind MediaKind
		ct   string
	}{
		{"a.png", MediaImage, "image/png"},
		{"dir/Photo.JPG", MediaImage, "image/jpeg"},
		{"logo.svg", MediaImage, "image/svg+xml"},
		{"talk.mp3", MediaAudio, "audio/mpeg"},
		{"talk.OGG", MediaAudio, "audio/ogg"},
		{"demo.webm", MediaVideo, "video/webm"},
		{"README.md", MediaNone, ""},
		{"png", MediaNone, ""},
		{"archive.png.zip", MediaNone, ""},
	} {
		if got := MediaKindOf(tt.path); got != tt.kind {
			t.Errorf("MediaKindOf(%q) = %v, want %v", tt.path, got, tt.kind)
		}
		if got := MediaContentType(tt.path); got != tt.ct {
			t.Errorf("MediaContentType(%q) = %q, want %q", tt.path, got, tt.ct)
		}
	}
}
