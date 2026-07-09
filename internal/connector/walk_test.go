package connector

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func writeFixtureFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func relPaths(entries []fileEntry) []string {
	paths := make([]string, len(entries))
	for i, e := range entries {
		paths[i] = e.relPath
	}
	sort.Strings(paths)
	return paths
}

func TestWalkTreePlainFiles(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "a.txt", "a")
	writeFixtureFile(t, root, "sub/b.txt", "b")

	entries, skipped, errs := walkTree(root, nil)
	if len(errs) != 0 {
		t.Fatalf("errs = %v, want none", errs)
	}
	if skipped != 0 {
		t.Fatalf("skipped = %d, want 0", skipped)
	}
	want := []string{"a.txt", "sub/b.txt"}
	if got := relPaths(entries); !equalStrings(got, want) {
		t.Errorf("entries = %v, want %v", got, want)
	}
}

func TestWalkTreeAlwaysPrunesGitAndGloss(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "keep.txt", "k")
	writeFixtureFile(t, root, ".git/HEAD", "ref: refs/heads/main")
	writeFixtureFile(t, root, ".gloss/gloss.db", "binary")

	entries, _, errs := walkTree(root, nil)
	if len(errs) != 0 {
		t.Fatalf("errs = %v, want none", errs)
	}
	want := []string{"keep.txt"}
	if got := relPaths(entries); !equalStrings(got, want) {
		t.Errorf("entries = %v, want %v (.git/.gloss must be pruned)", got, want)
	}
}

func TestWalkTreeAppliesIgnoreMatcher(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "keep.txt", "k")
	writeFixtureFile(t, root, "vendor/lib.go", "v")

	m := &ignoreMatcher{rules: parseIgnoreLines([]string{"vendor/"})}
	entries, _, errs := walkTree(root, m)
	if len(errs) != 0 {
		t.Fatalf("errs = %v, want none", errs)
	}
	want := []string{"keep.txt"}
	if got := relPaths(entries); !equalStrings(got, want) {
		t.Errorf("entries = %v, want %v", got, want)
	}
}

func TestWalkTreeSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "real.txt", "r")
	if err := os.Symlink(filepath.Join(root, "real.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	entries, skipped, errs := walkTree(root, nil)
	if len(errs) != 0 {
		t.Fatalf("errs = %v, want none", errs)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
	want := []string{"real.txt"}
	if got := relPaths(entries); !equalStrings(got, want) {
		t.Errorf("entries = %v, want %v (symlink must be excluded)", got, want)
	}
}

func TestWalkTreeEmptyDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	entries, skipped, errs := walkTree(root, nil)
	if len(entries) != 0 || skipped != 0 || len(errs) != 0 {
		t.Errorf("empty tree walk = entries:%v skipped:%d errs:%v, want all zero", entries, skipped, errs)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
