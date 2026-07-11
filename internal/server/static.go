package server

import (
	"embed"
	"io/fs"
)

//go:embed static
var staticFS embed.FS

// staticContentFS roots staticFS at "static" so URLs map 1:1 to the
// vendored/generated asset paths (e.g. /static/vendor/htmx.min.js ->
// static/vendor/htmx.min.js).
var staticContentFS = func() fs.FS {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic("server: static assets not embedded: " + err.Error())
	}
	return sub
}()
