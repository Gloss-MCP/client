package server

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/gloss-mcp/client/internal/connector"
	"github.com/gloss-mcp/client/internal/plugins"
)

// handleRaw streams a tracked media file's bytes for the image, audio and
// video plugins ("<img src=/raw/...>"). It serves only files in the
// tracked set (the same check, and so the same path-traversal and ignore
// defence, as the browse routes) with an allowlisted media extension and
// at most plugins.MaxMediaBytes.
//
// http.ServeContent provides Range support, which audio/video seeking
// needs. The response is locked down so a hostile file can't act as a
// page: nosniff pins the declared type, and a sandbox CSP stops an SVG
// opened directly from running script in gloss's origin.
func (s *Server) handleRaw(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")

	contentType := plugins.MediaContentType(path)
	if contentType == "" {
		http.NotFound(w, r)
		return
	}
	paths, err := connector.ListFiles(s.cfg.Root, s.cfg.ConnectorType)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !containsPath(paths, path) {
		http.NotFound(w, r)
		return
	}

	f, err := os.Open(filepath.Join(s.cfg.Root, filepath.FromSlash(path)))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	if info.Size() > plugins.MaxMediaBytes {
		http.Error(w, "file too large to preview", http.StatusRequestEntityTooLarge)
		return
	}

	h := w.Header()
	h.Set("Content-Type", contentType)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'unsafe-inline'")
	h.Set("Cache-Control", "no-cache") // files change on disk; revalidate via Last-Modified
	http.ServeContent(w, r, "", info.ModTime(), f)
}
