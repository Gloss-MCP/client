package plugins

import (
	"path"
	"strings"
)

// MaxMediaBytes is the largest media file (image, audio, video) gloss
// previews or serves from /raw. Larger files render a "too large" note
// with no annotation surface.
const MaxMediaBytes = 25 << 20

// MediaKind classifies a media file for the plugin that renders it.
type MediaKind int

// Media kinds. MediaNone means path is not a media file.
const (
	MediaNone MediaKind = iota
	MediaImage
	MediaAudio
	MediaVideo
)

// mediaTypes maps a lower-case extension to its kind and the exact
// Content-Type /raw serves it with. Serving by an explicit allowlist (not
// sniffing or mime.TypeByExtension, which reads the host's mime database)
// keeps the raw route to formats the plugins can actually render.
var mediaTypes = map[string]struct {
	Kind        MediaKind
	ContentType string
}{
	".png":  {MediaImage, "image/png"},
	".jpg":  {MediaImage, "image/jpeg"},
	".jpeg": {MediaImage, "image/jpeg"},
	".gif":  {MediaImage, "image/gif"},
	".webp": {MediaImage, "image/webp"},
	".svg":  {MediaImage, "image/svg+xml"},
	".mp3":  {MediaAudio, "audio/mpeg"},
	".wav":  {MediaAudio, "audio/wav"},
	".ogg":  {MediaAudio, "audio/ogg"},
	".m4a":  {MediaAudio, "audio/mp4"},
	".flac": {MediaAudio, "audio/flac"},
	".mp4":  {MediaVideo, "video/mp4"},
	".webm": {MediaVideo, "video/webm"},
	".mov":  {MediaVideo, "video/quicktime"},
	".ogv":  {MediaVideo, "video/ogg"},
}

// MediaKindOf reports which kind of media p is, by extension
// (case-insensitive), or MediaNone.
func MediaKindOf(p string) MediaKind {
	return mediaTypes[strings.ToLower(path.Ext(p))].Kind
}

// MediaContentType returns the Content-Type /raw serves p with, or ""
// when p is not a media file.
func MediaContentType(p string) string {
	return mediaTypes[strings.ToLower(path.Ext(p))].ContentType
}
