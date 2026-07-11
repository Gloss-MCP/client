package connector

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gloss-mcp/client/internal/store"
)

func TestListFilesRespectsGlossignore(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "keep.txt", "k")
	writeFixtureFile(t, root, "scratch.draft", "s")
	writeFixtureFile(t, root, ".glossignore", "*.draft\n")

	got, err := ListFiles(root, store.ConnectorLocal)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	want := []string{".glossignore", "keep.txt"}
	if !equalStrings(got, want) {
		t.Errorf("ListFiles = %v, want %v", got, want)
	}
}

func TestListFilesGitRespectsGitignore(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "keep.txt", "k")
	writeFixtureFile(t, root, "secret.log", "s")
	writeFixtureFile(t, root, ".gitignore", "*.log\n")

	got, err := ListFiles(root, store.ConnectorGit)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	want := []string{".gitignore", "keep.txt"}
	if !equalStrings(got, want) {
		t.Errorf("ListFiles = %v, want %v", got, want)
	}
}

func TestListFilesLocalIgnoresGitignore(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "keep.txt", "k")
	writeFixtureFile(t, root, "secret.log", "s")
	writeFixtureFile(t, root, ".gitignore", "*.log\n")

	got, err := ListFiles(root, store.ConnectorLocal)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	want := []string{".gitignore", "keep.txt", "secret.log"}
	if !equalStrings(got, want) {
		t.Errorf("ListFiles = %v, want %v (local connector must not read .gitignore)", got, want)
	}
}

func TestListFilesSorted(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "z.txt", "z")
	writeFixtureFile(t, root, "a.txt", "a")
	writeFixtureFile(t, root, "sub/m.txt", "m")

	got, err := ListFiles(root, store.ConnectorLocal)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	want := []string{"a.txt", "sub/m.txt", "z.txt"}
	if !equalStrings(got, want) {
		t.Errorf("ListFiles = %v, want %v (sorted)", got, want)
	}
}

func TestListFilesAlwaysPrunesGitAndGloss(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "keep.txt", "k")
	writeFixtureFile(t, root, ".git/HEAD", "ref: refs/heads/main")
	writeFixtureFile(t, root, ".gloss/gloss.db", "binary")

	got, err := ListFiles(root, store.ConnectorGit)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	want := []string{"keep.txt"}
	if !equalStrings(got, want) {
		t.Errorf("ListFiles = %v, want %v", got, want)
	}
}

func TestListFilesEmptyDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := ListFiles(root, store.ConnectorLocal)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListFiles = %v, want empty", got)
	}
}
