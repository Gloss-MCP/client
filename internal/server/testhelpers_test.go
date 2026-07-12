package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/gloss-mcp/client/internal/store"
)

// createTestThread creates a thread anchored to [startLine, endLine] in
// path directly via the store, for tests that need a fixture thread
// without exercising the create-thread HTTP handler themselves. path
// must already exist under root (testServer's fixture files).
func createTestThread(t *testing.T, srv *Server, root, sessionID, path string, startLine, endLine int, body string) *store.Thread {
	t.Helper()
	ctx := context.Background()

	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		t.Fatalf("read fixture file %s: %v", path, err)
	}
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])

	snap, err := srv.cfg.Store.FindFileSnapshot(ctx, srv.cfg.RepoID, path, hash)
	if err != nil {
		snap, err = srv.cfg.Store.CreateFileSnapshot(ctx, srv.cfg.RepoID, path, hash, "")
		if err != nil {
			t.Fatalf("CreateFileSnapshot: %v", err)
		}
	}

	thread, _, err := srv.cfg.Store.CreateThread(ctx, store.CreateThreadParams{
		SessionID:      sessionID,
		FileSnapshotID: snap.ID,
		Anchor:         store.LineAnchor{StartLine: startLine, EndLine: endLine},
		CreatedBy:      testAuthor,
		Body:           body,
		AuthorType:     store.AuthorHuman,
		AuthorAgent:    testAuthor,
	})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	return thread
}
