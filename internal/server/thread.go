package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gloss-mcp/client/internal/connector"
	"github.com/gloss-mcp/client/internal/plugins"
	"github.com/gloss-mcp/client/internal/store"
)

// sessionPageData is the template data for the full session-scoped page.
type sessionPageData struct {
	RepoName        string
	Tree            *treeNode
	Sessions        []*store.Session
	ActiveSessionID string
	Session         *store.Session
	Main            *sessionMainData
}

// sessionMainData is the template data for the swappable session content
// pane: either a selected file's code and its threads, or -- when no
// file is selected -- the session's whole thread list.
type sessionMainData struct {
	Session      *store.Session
	SessionID    string
	Path         string // empty when no file is selected
	Views        []plugins.View
	ThreadLines  []int // line numbers with at least one thread, for gutter highlighting
	Threads      []*threadViewData
	StatusFilter string
	ReturnTo     string
}

// threadViewData is one thread rendered in the thread panel, with its
// comments already assembled into a reply tree.
type threadViewData struct {
	Thread   *store.Thread
	Comments []*commentNode
	FilePath string // populated for the session-wide (no file selected) view
}

// handleSessionBrowse serves both the session home ("/s/{id}") and a
// selected file within a session ("/s/{id}/files/{path...}"). Mirrors
// handleBrowse's full-page-vs-HTMX-partial split.
func (s *Server) handleSessionBrowse(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("sessionID")
	path := r.PathValue("path")

	sess, err := s.cfg.Store.GetSession(ctx, sessionID)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}

	paths, err := connector.ListFiles(s.cfg.Root, s.cfg.ConnectorType)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	statusFilter := store.AnchorStatus(r.URL.Query().Get("status"))
	main := &sessionMainData{
		Session:      sess,
		SessionID:    sessionID,
		StatusFilter: string(statusFilter),
	}

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
		main.Path = path
		main.Views = views
		main.ReturnTo = fileReturnTo(sessionID, path, statusFilter)

		threads, err := s.cfg.Store.ListThreads(ctx, sessionID, store.ListThreadsFilter{FilePath: path, AnchorStatus: statusFilter})
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		main.Threads, main.ThreadLines, err = s.loadThreadViews(ctx, sessionID, threads, main.ReturnTo, false)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	} else {
		main.ReturnTo = fileReturnTo(sessionID, "", statusFilter)
		threads, err := s.cfg.Store.ListThreads(ctx, sessionID, store.ListThreadsFilter{AnchorStatus: statusFilter})
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		main.Threads, _, err = s.loadThreadViews(ctx, sessionID, threads, main.ReturnTo, true)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Header.Get("HX-Request") == "true" {
		_ = tmpl.ExecuteTemplate(w, "sessionMain", main)
		return
	}
	data := sessionPageData{
		RepoName:        s.cfg.RepoName,
		Tree:            buildTree(paths),
		Sessions:        s.listSessions(ctx),
		ActiveSessionID: sessionID,
		Session:         sess,
		Main:            main,
	}
	_ = tmpl.ExecuteTemplate(w, "sessionLayout", data)
}

// fileReturnTo builds the URL a mutation on this page should redirect
// back to, preserving the current status filter.
func fileReturnTo(sessionID, path string, status store.AnchorStatus) string {
	url := "/s/" + sessionID
	if path != "" {
		url += "/files/" + path
	}
	if status != "" {
		url += "?status=" + string(status)
	}
	return url
}

// loadThreadViews fetches each thread's comments and assembles them into
// reply trees. When includeFilePath is true, each thread's file path is
// resolved too (the session-wide view, where threads span many files).
// It also returns the set of line numbers with at least one thread, for
// gutter highlighting in the single-file view.
func (s *Server) loadThreadViews(ctx context.Context, sessionID string, threads []*store.Thread, returnTo string, includeFilePath bool) ([]*threadViewData, []int, error) {
	var views []*threadViewData
	lineSet := map[int]bool{}
	for _, t := range threads {
		comments, err := s.cfg.Store.ListComments(ctx, t.ID)
		if err != nil {
			return nil, nil, err
		}
		replyAction := "/s/" + sessionID + "/threads/" + t.ID + "/comments"
		tv := &threadViewData{Thread: t, Comments: buildCommentTree(comments, replyAction, returnTo)}
		if includeFilePath {
			if snap, err := s.cfg.Store.GetFileSnapshot(ctx, t.FileSnapshotID); err == nil {
				tv.FilePath = snap.Path
			}
		}
		views = append(views, tv)
		if la, ok := t.Anchor.(store.LineAnchor); ok {
			for n := la.StartLine; n <= la.EndLine; n++ {
				lineSet[n] = true
			}
		}
	}
	lines := make([]int, 0, len(lineSet))
	for n := range lineSet {
		lines = append(lines, n)
	}
	sort.Ints(lines)
	return views, lines, nil
}

// handleCreateThread creates a thread anchored to a line range in the
// currently viewed file, from the composer's line selection.
func (s *Server) handleCreateThread(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("sessionID")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	path := r.FormValue("file_path")
	body := strings.TrimSpace(r.FormValue("body"))
	startLine, errS := strconv.Atoi(r.FormValue("start_line"))
	endLine, errE := strconv.Atoi(r.FormValue("end_line"))
	if path == "" || body == "" || errS != nil || errE != nil || startLine < 1 || endLine < startLine {
		http.Error(w, "invalid thread parameters", http.StatusBadRequest)
		return
	}

	content, err := os.ReadFile(filepath.Join(s.cfg.Root, filepath.FromSlash(path)))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	snap, err := s.resolveFileSnapshot(ctx, path, content)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if _, _, err := s.cfg.Store.CreateThread(ctx, store.CreateThreadParams{
		SessionID:      sessionID,
		FileSnapshotID: snap.ID,
		Anchor:         store.LineAnchor{StartLine: startLine, EndLine: endLine},
		CreatedBy:      s.cfg.Author,
		Body:           body,
		AuthorType:     store.AuthorHuman,
		AuthorAgent:    s.cfg.Author,
	}); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	redirectBack(w, r, returnTo(r))
}

// resolveFileSnapshot finds the current snapshot of path, creating one on
// the fly if the file was added to the tree after gloss's startup
// snapshot pass (the tree is a live disk walk, so this can legitimately
// happen).
func (s *Server) resolveFileSnapshot(ctx context.Context, path string, content []byte) (*store.FileSnapshot, error) {
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])
	snap, err := s.cfg.Store.FindFileSnapshot(ctx, s.cfg.RepoID, path, hash)
	if errors.Is(err, store.ErrNotFound) {
		return s.cfg.Store.CreateFileSnapshot(ctx, s.cfg.RepoID, path, hash, "")
	}
	return snap, err
}

// handleResolveThread marks a thread resolved.
func (s *Server) handleResolveThread(w http.ResponseWriter, r *http.Request) {
	s.setThreadStatus(w, r, store.AnchorResolved)
}

// handleReopenThread reactivates a resolved (or orphaned) thread.
func (s *Server) handleReopenThread(w http.ResponseWriter, r *http.Request) {
	s.setThreadStatus(w, r, store.AnchorActive)
}

func (s *Server) setThreadStatus(w http.ResponseWriter, r *http.Request, status store.AnchorStatus) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	id := r.PathValue("threadID")
	if _, err := s.cfg.Store.SetAnchorStatus(r.Context(), id, status); err != nil {
		writeStoreError(w, r, err)
		return
	}
	redirectBack(w, r, returnTo(r))
}
