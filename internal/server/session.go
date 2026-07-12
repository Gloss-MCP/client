package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/gloss-mcp/client/internal/store"
)

// listSessions returns the repo's sessions, oldest first, for the
// switcher and session-scoped views. A nil Store -- only possible in
// tests that don't exercise annotation features -- yields no sessions.
func (s *Server) listSessions(ctx context.Context) []*store.Session {
	if s.cfg.Store == nil {
		return nil
	}
	sessions, err := s.cfg.Store.ListSessions(ctx, s.cfg.RepoID, "")
	if err != nil {
		return nil
	}
	return sessions
}

// handleCreateSession creates a session and redirects into it.
func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	sess, err := s.cfg.Store.CreateSession(r.Context(), s.cfg.RepoID, name, r.FormValue("description"), s.cfg.Author)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	redirectBack(w, r, "/s/"+sess.ID)
}

// handleUpdateSession renames a session and/or changes its status.
func (s *Server) handleUpdateSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	status := store.SessionStatus(r.FormValue("status"))
	if status == "" {
		status = store.SessionOpen
	}

	if _, err := s.cfg.Store.UpdateSession(r.Context(), id, name, r.FormValue("description"), status); err != nil {
		writeStoreError(w, r, err)
		return
	}
	redirectBack(w, r, returnTo(r))
}

// handleDeleteSession hard-deletes a session and its threads/comments.
func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.cfg.Store.DeleteSession(r.Context(), id); err != nil {
		writeStoreError(w, r, err)
		return
	}
	redirectBack(w, r, returnTo(r))
}
