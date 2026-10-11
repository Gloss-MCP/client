package plugins

import (
	"bytes"
	"fmt"
	"html/template"
)

// Audio renders audio files (mp3, wav, ogg, m4a, flac) as a native
// <audio controls> player served from /raw, wrapped in a
// [data-time-surface] element. The UI marks a start and end on the
// player's timeline; a selection anchors with a TimeAnchor (seconds).
type Audio struct{}

// NewAudio constructs the audio plugin.
func NewAudio() Audio { return Audio{} }

// Name implements Plugin.
func (Audio) Name() string { return "audio" }

// Accepts implements Plugin.
func (Audio) Accepts(p string) bool { return MediaKindOf(p) == MediaAudio }

// Streamed implements Streamed: the bytes are served by /raw, not rendered.
func (Audio) Streamed() {}

var audioTemplate = template.Must(template.New("audio").Parse(
	`<div class="gloss-media gloss-time-surface" data-time-surface>` +
		`<audio class="gloss-player" controls preload="metadata" src="{{.Src}}"></audio>` +
		`</div>`,
))

// Render implements Plugin.
func (Audio) Render(f File) ([]View, error) {
	if f.size() > MaxMediaBytes {
		return []View{{HTML: tooLargeNote(f.size())}}, nil
	}
	var buf bytes.Buffer
	if err := audioTemplate.Execute(&buf, struct{ Src string }{rawURL(f.Path)}); err != nil {
		return nil, fmt.Errorf("plugins: render audio %s: %w", f.Path, err)
	}
	return []View{{HTML: template.HTML(buf.String()), Surface: SurfaceTime}}, nil
}
