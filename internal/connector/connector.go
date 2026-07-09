// Package connector walks a repository's working tree and captures
// store.FileSnapshot rows for its tracked files. Two connector types
// share one walk+hash+persist core (internal/connector/snapshot.go) and
// differ only in which ignore files apply and whether a git commit SHA
// is attached:
//
//   - local: plain directory walk, respects only .glossignore.
//   - git: adds .gitignore respect and records the current commit SHA
//     (store.FileSnapshot.GitCommitSHA) as an anchor fallback.
package connector

import (
	"context"
	"os"
	"path/filepath"

	"github.com/gloss-mcp/client/internal/store"
)

// Result summarises one Snapshot run.
type Result struct {
	Root    string  // absolute path snapshotted
	Files   int     // tracked files considered after ignore-filtering
	Created int     // new FileSnapshot rows created
	Reused  int     // existing rows reused (content unchanged)
	Skipped int     // symlinks and unreadable files/dirs
	Errors  []error // non-fatal per-file/per-dir errors collected along the way
}

// Connector captures a repository's tracked files as store.FileSnapshot
// rows.
type Connector interface {
	Type() store.ConnectorType
	Snapshot(ctx context.Context, st *store.Store, repoID string) (Result, error)
}

// Detect inspects root and reports which connector type gloss should
// use: ConnectorGit when root/.git exists and is a directory,
// ConnectorLocal otherwise.
func Detect(root string) store.ConnectorType {
	if info, err := os.Stat(filepath.Join(root, ".git")); err == nil && info.IsDir() {
		return store.ConnectorGit
	}
	return store.ConnectorLocal
}

// New constructs the Connector implementation for connectorType, rooted
// at root.
func New(root string, connectorType store.ConnectorType) Connector {
	if connectorType == store.ConnectorGit {
		return &gitConnector{root: root}
	}
	return &localConnector{root: root}
}

type localConnector struct{ root string }

func (c *localConnector) Type() store.ConnectorType { return store.ConnectorLocal }

func (c *localConnector) Snapshot(ctx context.Context, st *store.Store, repoID string) (Result, error) {
	return snapshot(ctx, st, repoID, c.root, false, "")
}

type gitConnector struct{ root string }

func (c *gitConnector) Type() store.ConnectorType { return store.ConnectorGit }

func (c *gitConnector) Snapshot(ctx context.Context, st *store.Store, repoID string) (Result, error) {
	// Best-effort: a fresh repo with no commits, or no git binary, still
	// snapshots successfully with an empty commit SHA.
	sha, _ := headCommitSHA(ctx, c.root)
	return snapshot(ctx, st, repoID, c.root, true, sha)
}
