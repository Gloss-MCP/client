package plugins

import (
	"bytes"
	"fmt"
	"html/template"
	"net/url"
)

// Image renders png, jpg, gif, webp and svg files as an <img> served from
// the /raw route, wrapped in a [data-region-surface] element. A selection
// on the surface anchors with a RegionAnchor: x, y, width and height as
// fractions (0-1) of the displayed image. The UI draws and selects
// regions; the plugin only provides the surface.
//
// SVG is shown as an image only -- never inlined -- so its markup cannot
// run script in gloss's origin, and it is not line-annotatable.
type Image struct{}

// NewImage constructs the image plugin.
func NewImage() Image { return Image{} }

// Name implements Plugin.
func (Image) Name() string { return "image" }

// Accepts implements Plugin.
func (Image) Accepts(p string) bool { return MediaKindOf(p) == MediaImage }

// Streamed implements Streamed: the bytes are served by /raw, not rendered.
func (Image) Streamed() {}

var imageTemplate = template.Must(template.New("image").Parse(
	`<div class="gloss-media gloss-region-surface" data-region-surface>` +
		`<img class="gloss-media-img" src="{{.Src}}" alt="{{.Alt}}" draggable="false">` +
		`</div>`,
))

// Render implements Plugin.
func (Image) Render(f File) ([]View, error) {
	if f.size() > MaxMediaBytes {
		return []View{{HTML: tooLargeNote(f.size())}}, nil
	}
	var buf bytes.Buffer
	err := imageTemplate.Execute(&buf, struct{ Src, Alt string }{rawURL(f.Path), f.Path})
	if err != nil {
		return nil, fmt.Errorf("plugins: render image %s: %w", f.Path, err)
	}
	return []View{{HTML: template.HTML(buf.String()), Surface: SurfaceRegion}}, nil
}

// rawURL is the /raw URL for a repo-relative, slash-separated path.
func rawURL(p string) string {
	return (&url.URL{Path: "/raw/" + p}).EscapedPath()
}

// tooLargeNote is the placeholder for media above MaxMediaBytes: no
// surface, so nothing on it can be annotated.
func tooLargeNote(size int64) template.HTML {
	return template.HTML(fmt.Sprintf(
		`<p class="gloss-note text-xs text-amber-700">File is too large to preview (%.1f MB; limit %d MB).</p>`,
		float64(size)/(1<<20), MaxMediaBytes>>20))
}
