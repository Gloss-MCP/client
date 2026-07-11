package server

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/gloss-mcp/client/internal/connector"
	"github.com/gloss-mcp/client/internal/plugins"
)

// layoutData is the top-level template data for the full page.
type layoutData struct {
	RepoName string
	Tree     *treeNode
	File     *fileViewData // nil when no file is selected
}

// fileViewData is the template data for the content-pane partial.
type fileViewData struct {
	Path  string
	Views []plugins.View
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /{$}", s.handleBrowse)
	s.mux.HandleFunc("GET /files/{path...}", s.handleBrowse)
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticContentFS)))
}

// handleBrowse serves both the empty index ("/") and a selected file
// ("/files/{path...}"). A plain request renders the full page; an HTMX
// request (HX-Request header) renders just the content-pane partial, so
// clicking a tree entry swaps content without a full page reload.
func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")

	paths, err := connector.ListFiles(s.cfg.Root, s.cfg.ConnectorType)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	data := layoutData{RepoName: s.cfg.RepoName, Tree: buildTree(paths)}

	if path != "" {
		if !containsPath(paths, path) {
			http.NotFound(w, r)
			return
		}

		content, err := os.ReadFile(filepath.Join(s.cfg.Root, filepath.FromSlash(path)))
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		plugin := s.cfg.Registry.For(path)
		views, err := plugin.Render(plugins.File{Path: path, Content: content})
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		data.File = &fileViewData{Path: path, Views: views}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Header.Get("HX-Request") == "true" {
		_ = tmpl.ExecuteTemplate(w, "fileContent", data.File)
		return
	}
	_ = tmpl.ExecuteTemplate(w, "layout", data)
}

// containsPath reports whether path is present in the sorted slice
// paths. A file is only ever read from disk if it passed this check --
// the same check that determines what's shown in the tree -- which
// doubles as the path-traversal defense (no relative path outside the
// tracked file set is ever accepted).
func containsPath(paths []string, path string) bool {
	i := sort.SearchStrings(paths, path)
	return i < len(paths) && paths[i] == path
}
