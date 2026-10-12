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
	"github.com/gloss-mcp/client/internal/delta"
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
	Session     *store.Session
	SessionID   string
	Path        string // empty when no file is selected
	Views       []plugins.View
	ThreadLines []int // line numbers with at least one thread, for gutter highlighting
	// Surface is the first view's selection model ("" for line-addressed
	// text, plugins.SurfaceRegion for images); Marks are the file's
	// region threads, drawn as overlay boxes on that surface.
	Surface      string
	Marks        []mediaMark
	Threads      []*threadViewData
	StatusFilter string
	ReturnTo     string
}

// mediaMark is a region, time or region-time thread as the UI draws it
// over a media surface. Geometry is fractions (0-1) of the displayed
// asset; times are seconds. N is the thread's
// 1-based position in the panel, shown on the box so the two can be
// matched by eye.
type mediaMark struct {
	ID     string  `json:"id"`
	N      int     `json:"n"`
	Status string  `json:"status"`
	Type   string  `json:"type"` // region, time or region_time
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Start  float64 `json:"start"` // seconds; time and region_time
	End    float64 `json:"end"`
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
		views, err := s.renderFile(path)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		main.Path = path
		main.Views = views
		if len(views) > 0 {
			main.Surface = views[0].Surface
		}
		main.ReturnTo = fileReturnTo(sessionID, path, statusFilter)

		threads, err := s.cfg.Store.ListThreads(ctx, sessionID, store.ListThreadsFilter{FilePath: path, AnchorStatus: statusFilter})
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		main.Threads, main.ThreadLines, main.Marks, err = s.loadThreadViews(ctx, sessionID, threads, main.ReturnTo, false)
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
		main.Threads, _, _, err = s.loadThreadViews(ctx, sessionID, threads, main.ReturnTo, true)
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
// gutter highlighting in the single-file view, and the region marks to
// draw over a media surface.
func (s *Server) loadThreadViews(ctx context.Context, sessionID string, threads []*store.Thread, returnTo string, includeFilePath bool) ([]*threadViewData, []int, []mediaMark, error) {
	var views []*threadViewData
	var marks []mediaMark
	lineSet := map[int]bool{}
	for _, t := range threads {
		comments, err := s.cfg.Store.ListComments(ctx, t.ID)
		if err != nil {
			return nil, nil, nil, err
		}
		replyAction := "/s/" + sessionID + "/threads/" + t.ID + "/comments"
		tv := &threadViewData{Thread: t, Comments: buildCommentTree(comments, replyAction, returnTo)}
		if includeFilePath {
			if snap, err := s.cfg.Store.GetFileSnapshot(ctx, t.FileSnapshotID); err == nil {
				tv.FilePath = snap.Path
			}
		}
		views = append(views, tv)
		switch a := t.Anchor.(type) {
		case store.LineAnchor:
			for n := a.StartLine; n <= a.EndLine; n++ {
				lineSet[n] = true
			}
		case store.CellAnchor:
			for n := a.StartLine; n <= a.EndLine; n++ {
				lineSet[n] = true
			}
		case store.RegionAnchor:
			marks = append(marks, mediaMark{
				ID: t.ID, N: len(views), Status: string(t.AnchorStatus), Type: string(a.AnchorType()),
				X: a.X, Y: a.Y, Width: a.Width, Height: a.Height,
			})
		case store.RegionTimeAnchor:
			marks = append(marks, mediaMark{
				ID: t.ID, N: len(views), Status: string(t.AnchorStatus), Type: string(a.AnchorType()),
				X: a.X, Y: a.Y, Width: a.Width, Height: a.Height, Start: a.StartTime, End: a.EndTime,
			})
		case store.TimeAnchor:
			marks = append(marks, mediaMark{
				ID: t.ID, N: len(views), Status: string(t.AnchorStatus), Type: string(a.AnchorType()),
				Start: a.StartTime, End: a.EndTime,
			})
		}
	}
	lines := make([]int, 0, len(lineSet))
	for n := range lineSet {
		lines = append(lines, n)
	}
	sort.Ints(lines)
	return views, lines, marks, nil
}

// handleCreateThread creates a thread anchored to a line range in the
// currently viewed file, from the composer's selection: a line range, or
// -- when a column ("col") is also submitted -- a cell of a delimited
// file.
func (s *Server) handleCreateThread(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	path := r.FormValue("file_path")
	body := strings.TrimSpace(r.FormValue("body"))

	// A selection on a media surface submits x/y/width/height (fractions
	// of the asset) and/or start_time/end_time (seconds) instead of a
	// line range.
	if r.FormValue("x") != "" || r.FormValue("start_time") != "" {
		s.createMediaThread(w, r, sessionID, path, body)
		return
	}

	startLine, errS := strconv.Atoi(r.FormValue("start_line"))
	endLine, errE := strconv.Atoi(r.FormValue("end_line"))
	if path == "" || body == "" || errS != nil || errE != nil || startLine < 1 || endLine < startLine {
		http.Error(w, "invalid thread parameters", http.StatusBadRequest)
		return
	}

	if !s.requireTracked(w, r, path) {
		return
	}
	content, err := os.ReadFile(filepath.Join(s.cfg.Root, filepath.FromSlash(path)))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	lines := delta.SplitLines(content)
	ctxBefore, ctxAfter := delta.ExtractContext(lines, startLine, endLine, delta.ContextLines)

	var anchor store.Anchor = store.LineAnchor{
		StartLine:     startLine,
		EndLine:       endLine,
		ContextBefore: ctxBefore,
		ContextAfter:  ctxAfter,
	}
	if colStr := r.FormValue("col"); colStr != "" {
		col, err := strconv.Atoi(colStr)
		if err != nil || col < 1 {
			http.Error(w, "invalid thread parameters", http.StatusBadRequest)
			return
		}
		anchor = store.CellAnchor{
			StartLine:     startLine,
			EndLine:       endLine,
			Col:           col,
			Header:        delta.HeaderFor(content, col),
			ContextBefore: ctxBefore,
			ContextAfter:  ctxAfter,
		}
	}

	s.storeThread(w, r, sessionID, path, content, body, anchor)
}

// storeThread snapshots content for path, creates the human-authored
// thread with anchor and body, and redirects back to the page the
// composer was on.
func (s *Server) storeThread(w http.ResponseWriter, r *http.Request, sessionID, path string, content []byte, body string, anchor store.Anchor) {
	ctx := r.Context()
	snap, err := s.resolveFileSnapshot(ctx, path, content)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if _, _, err := s.cfg.Store.CreateThread(ctx, store.CreateThreadParams{
		SessionID:      sessionID,
		FileSnapshotID: snap.ID,
		Anchor:         anchor,
		CreatedBy:      s.cfg.Author,
		Body:           body,
		AuthorType:     store.AuthorHuman,
		AuthorAgent:    s.cfg.Author,
	}); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	redirectBack(w, r, returnTo(r))
}

// createMediaThread creates a thread anchored to a media position: a
// region of an image (x/y/width/height), a time range of audio
// (start_time/end_time), or a region at a time range of video (both).
// Geometry outside the asset, a non-numeric field, or an anchor kind the
// file's media type doesn't support is a 400, not a clamp: the UI already
// clamps, so bad values mean a broken or hostile client.
func (s *Server) createMediaThread(w http.ResponseWriter, r *http.Request, sessionID, path, body string) {
	num := func(name string) (float64, bool) {
		f, err := strconv.ParseFloat(r.FormValue(name), 64)
		return f, err == nil
	}
	var anchor store.Anchor
	var kind plugins.MediaKind
	hasRegion, hasTime := r.FormValue("x") != "", r.FormValue("start_time") != ""
	var v [6]float64
	ok := true
	for i, name := range []string{"x", "y", "width", "height", "start_time", "end_time"} {
		if (i < 4 && !hasRegion) || (i >= 4 && !hasTime) {
			continue
		}
		var good bool
		if v[i], good = num(name); !good {
			ok = false
		}
	}
	switch {
	case hasRegion && hasTime:
		anchor = store.RegionTimeAnchor{X: v[0], Y: v[1], Width: v[2], Height: v[3], StartTime: v[4], EndTime: v[5]}
		kind = plugins.MediaVideo
	case hasRegion:
		anchor = store.RegionAnchor{X: v[0], Y: v[1], Width: v[2], Height: v[3]}
		kind = plugins.MediaImage
	default:
		anchor = store.TimeAnchor{StartTime: v[4], EndTime: v[5]}
		kind = plugins.MediaAudio
	}
	if !ok || path == "" || body == "" || plugins.MediaKindOf(path) != kind || store.ValidateAnchor(anchor) != nil {
		http.Error(w, "invalid thread parameters", http.StatusBadRequest)
		return
	}
	if !s.requireTracked(w, r, path) {
		return
	}
	content, err := os.ReadFile(filepath.Join(s.cfg.Root, filepath.FromSlash(path)))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.storeThread(w, r, sessionID, path, content, body, anchor)
}

// requireTracked reports whether path is in the tracked file set, writing
// a 404 (or 500 if the walk fails) when it is not. Handlers that read a
// client-supplied file_path must call it first: the path is joined onto
// the root unvalidated otherwise, so "../" or an ignored file would be
// read from disk.
func (s *Server) requireTracked(w http.ResponseWriter, r *http.Request, path string) bool {
	paths, err := connector.ListFiles(s.cfg.Root, s.cfg.ConnectorType)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return false
	}
	if !containsPath(paths, path) {
		http.NotFound(w, r)
		return false
	}
	return true
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
