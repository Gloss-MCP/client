package connector

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gloss-mcp/client/internal/store"
)

func TestDetectGitDirPresent(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := Detect(dir); got != store.ConnectorGit {
		t.Errorf("Detect = %q, want %q", got, store.ConnectorGit)
	}
}

func TestDetectNoGitDir(t *testing.T) {
	dir := t.TempDir()
	if got := Detect(dir); got != store.ConnectorLocal {
		t.Errorf("Detect = %q, want %q", got, store.ConnectorLocal)
	}
}

func TestDetectGitFileNotDir(t *testing.T) {
	// A .git file (not directory) is a submodule/worktree gitlink, not a
	// full repo checkout -- Detect treats it as local.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: ../x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Detect(dir); got != store.ConnectorLocal {
		t.Errorf("Detect = %q, want %q", got, store.ConnectorLocal)
	}
}

func TestNewReturnsMatchingType(t *testing.T) {
	if got := New(t.TempDir(), store.ConnectorGit).Type(); got != store.ConnectorGit {
		t.Errorf("New(git).Type() = %q, want %q", got, store.ConnectorGit)
	}
	if got := New(t.TempDir(), store.ConnectorLocal).Type(); got != store.ConnectorLocal {
		t.Errorf("New(local).Type() = %q, want %q", got, store.ConnectorLocal)
	}
}
