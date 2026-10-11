// Package plugins renders file content for the web UI. A Plugin decides
// whether it applies to a given path and renders that file's content as
// one or more named HTML views; a Registry picks the first applicable
// plugin, in registration order, with plaintext as the catch-all that
// always applies.
package plugins

import "html/template"

// File is the content a Plugin renders.
type File struct {
	Path    string // relative to the repository root, slash-separated
	Content []byte // empty for Streamed plugins, which don't need the bytes
	Size    int64  // file size in bytes; set by the server, defaults to len(Content)
}

// size returns the file's size in bytes.
func (f File) size() int64 {
	if f.Size > 0 {
		return f.Size
	}
	return int64(len(f.Content))
}

// Streamed marks a plugin that renders a reference to the file (an
// <img>/<audio>/<video> pointing at the server's /raw route) instead of
// its bytes. The server then skips reading the file for Render and passes
// only Path and Size, so previewing a 25 MB video doesn't load it into
// memory.
type Streamed interface {
	Plugin
	Streamed()
}

// View is one named rendering of a File. Name is empty for a
// single-view plugin; multi-view plugins (e.g. markdown's "Rendered" and
// "Source") give each view a distinct, UI-facing name.
type View struct {
	Name string
	HTML template.HTML
	// Surface names the non-line selection model this view uses, when it
	// isn't the shared [data-line] one: SurfaceRegion for views wrapped
	// in a [data-region-surface] element. Empty for line-addressable
	// and read-only views.
	Surface string
}

// Surface values for View.Surface.
const SurfaceRegion = "region"

// Plugin renders a file's content as HTML for the file-browser view.
type Plugin interface {
	// Name identifies the plugin, e.g. "plaintext".
	Name() string
	// Accepts reports whether this plugin should render path.
	Accepts(path string) bool
	// Render produces the views to display for f. Called only after
	// Accepts(f.Path) has returned true.
	Render(f File) ([]View, error)
}

// Registry holds plugins in registration order and resolves which one
// renders a given path.
type Registry struct {
	plugins []Plugin
}

// NewRegistry builds a Registry that tries plugins in the given order.
func NewRegistry(plugins ...Plugin) *Registry {
	return &Registry{plugins: plugins}
}

// For returns the first registered plugin that accepts path, or nil if
// none does. Registries built with a catch-all plugin (plaintext) never
// return nil.
func (r *Registry) For(path string) Plugin {
	for _, p := range r.plugins {
		if p.Accepts(path) {
			return p
		}
	}
	return nil
}
