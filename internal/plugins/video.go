package plugins

import (
	"bytes"
	"fmt"
	"html/template"
)

// Video renders video files (mp4, webm, mov, ogv) as a native <video
// controls> player served from /raw, wrapped in a [data-region-surface]
// element. A selection anchors with a RegionTimeAnchor: a region of the
// frame (fractions 0-1) at a time range in seconds. The UI only draws in
// an explicit "draw" mode so it never competes with the player controls.
type Video struct{}

// NewVideo constructs the video plugin.
func NewVideo() Video { return Video{} }

// Name implements Plugin.
func (Video) Name() string { return "video" }

// Accepts implements Plugin.
func (Video) Accepts(p string) bool { return MediaKindOf(p) == MediaVideo }

// Streamed implements Streamed: the bytes are served by /raw, not rendered.
func (Video) Streamed() {}

var videoTemplate = template.Must(template.New("video").Parse(
	`<div class="gloss-media gloss-video-surface" data-region-surface data-time-surface>` +
		`<video class="gloss-media-video" controls preload="metadata" src="{{.Src}}"></video>` +
		`</div>`,
))

// Render implements Plugin.
func (Video) Render(f File) ([]View, error) {
	if f.size() > MaxMediaBytes {
		return []View{{HTML: tooLargeNote(f.size())}}, nil
	}
	var buf bytes.Buffer
	if err := videoTemplate.Execute(&buf, struct{ Src string }{rawURL(f.Path)}); err != nil {
		return nil, fmt.Errorf("plugins: render video %s: %w", f.Path, err)
	}
	return []View{{HTML: template.HTML(buf.String()), Surface: SurfaceRegionTime}}, nil
}
