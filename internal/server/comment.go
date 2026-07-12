package server

import (
	"net/http"
	"strings"

	"github.com/gloss-mcp/client/internal/store"
)

// commentNode is one comment in a thread's reply tree. ReplyAction and
// ReturnTo are carried on every node (not just the root) because a
// {{template}} invocation in html/template resets "$", so a recursively
// rendered reply can't reach back up to data set on an ancestor node.
type commentNode struct {
	Comment     *store.Comment
	Children    []*commentNode
	ReplyAction string // form action URL for replying to this thread
	ReturnTo    string
}

// buildCommentTree groups a thread's flat, oldest-first comment list
// (store.ListComments) into a reply tree via ParentCommentID. A comment
// whose parent is missing (shouldn't happen -- comments cascade-delete
// with their thread) is treated as top-level rather than dropped.
func buildCommentTree(comments []*store.Comment, replyAction, returnTo string) []*commentNode {
	byID := make(map[string]*commentNode, len(comments))
	for _, c := range comments {
		byID[c.ID] = &commentNode{Comment: c, ReplyAction: replyAction, ReturnTo: returnTo}
	}
	var roots []*commentNode
	for _, c := range comments {
		node := byID[c.ID]
		parent, ok := byID[c.ParentCommentID]
		if c.ParentCommentID == "" || !ok {
			roots = append(roots, node)
			continue
		}
		parent.Children = append(parent.Children, node)
	}
	return roots
}

// handleAddComment appends a top-level reply to a thread, or a nested
// reply when parent_comment_id is set.
func (s *Server) handleAddComment(w http.ResponseWriter, r *http.Request) {
	threadID := r.PathValue("threadID")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	body := strings.TrimSpace(r.FormValue("body"))
	if body == "" {
		http.Error(w, "body is required", http.StatusBadRequest)
		return
	}

	if _, err := s.cfg.Store.AddComment(r.Context(), store.AddCommentParams{
		ThreadID:        threadID,
		ParentCommentID: r.FormValue("parent_comment_id"),
		AuthorType:      store.AuthorHuman,
		AuthorAgent:     s.cfg.Author,
		Body:            body,
	}); err != nil {
		writeStoreError(w, r, err)
		return
	}
	redirectBack(w, r, returnTo(r))
}
