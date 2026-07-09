package connector

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gloss-mcp/client/internal/store"
)

const fixtureDir = "../../testdata/connector/fixture"

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "gloss.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return s
}

func testRepo(t *testing.T, s *store.Store, connectorType store.ConnectorType) *store.Repository {
	t.Helper()
	repo, err := s.CreateRepository(context.Background(), "fixture", connectorType, "")
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	return repo
}

// copyFixture copies the connector fixture corpus into dst.
func copyFixture(t *testing.T, dst string) {
	t.Helper()
	err := filepath.WalkDir(fixtureDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(fixtureDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
	if err != nil {
		t.Fatalf("copyFixture: %v", err)
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, in)
	return err
}

func snapshotPaths(snaps []*store.FileSnapshot) []string {
	paths := make([]string, len(snaps))
	for i, s := range snaps {
		paths[i] = s.Path
	}
	sort.Strings(paths)
	return paths
}

func TestLocalConnectorSnapshotPlainFiles(t *testing.T) {
	dir := t.TempDir()
	copyFixture(t, dir)

	s := newTestStore(t)
	repo := testRepo(t, s, store.ConnectorLocal)
	ctx := context.Background()

	result, err := New(dir, store.ConnectorLocal).Snapshot(ctx, s, repo.ID)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	snaps, err := s.ListFileSnapshots(ctx, repo.ID, "")
	if err != nil {
		t.Fatalf("ListFileSnapshots: %v", err)
	}
	// .gitignore is not loaded by the local connector, so vendor/ and
	// *.local files are present -- only .glossignore (notes/, *.draft)
	// is applied.
	want := []string{
		".gitignore",
		".glossignore",
		"README.md",
		"build/output.bin",
		"important.local",
		"main.go",
		"secrets.local",
		"vendor/thirdparty.go",
	}
	if got := snapshotPaths(snaps); !equalStrings(got, want) {
		t.Errorf("local snapshot paths = %v, want %v", got, want)
	}
	if result.Files != len(want) {
		t.Errorf("Result.Files = %d, want %d", result.Files, len(want))
	}
	if result.Created != len(want) {
		t.Errorf("Result.Created = %d, want %d", result.Created, len(want))
	}
	for _, snap := range snaps {
		if snap.GitCommitSHA != "" {
			t.Errorf("local snapshot %s has GitCommitSHA %q, want empty", snap.Path, snap.GitCommitSHA)
		}
	}
}

func TestGitConnectorSnapshotRespectsGitignore(t *testing.T) {
	dir := t.TempDir()
	copyFixture(t, dir)
	initGitRepo(t, dir)
	commitAll(t, dir, "initial")

	s := newTestStore(t)
	repo := testRepo(t, s, store.ConnectorGit)
	ctx := context.Background()

	if _, err := New(dir, store.ConnectorGit).Snapshot(ctx, s, repo.ID); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	snaps, err := s.ListFileSnapshots(ctx, repo.ID, "")
	if err != nil {
		t.Fatalf("ListFileSnapshots: %v", err)
	}
	// Both .gitignore (vendor/, *.local, !important.local) and
	// .glossignore (notes/, *.draft) apply.
	want := []string{
		".gitignore",
		".glossignore",
		"README.md",
		"build/output.bin",
		"important.local",
		"main.go",
	}
	if got := snapshotPaths(snaps); !equalStrings(got, want) {
		t.Errorf("git snapshot paths = %v, want %v", got, want)
	}

	wantSHA := strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))
	for _, snap := range snaps {
		if snap.GitCommitSHA != wantSHA {
			t.Errorf("snapshot %s GitCommitSHA = %q, want %q", snap.Path, snap.GitCommitSHA, wantSHA)
		}
	}
}

func TestSnapshotReusesUnchangedContent(t *testing.T) {
	dir := t.TempDir()
	copyFixture(t, dir)

	s := newTestStore(t)
	repo := testRepo(t, s, store.ConnectorLocal)
	ctx := context.Background()
	conn := New(dir, store.ConnectorLocal)

	first, err := conn.Snapshot(ctx, s, repo.ID)
	if err != nil {
		t.Fatalf("first Snapshot: %v", err)
	}

	second, err := conn.Snapshot(ctx, s, repo.ID)
	if err != nil {
		t.Fatalf("second Snapshot: %v", err)
	}
	if second.Created != 0 {
		t.Errorf("second run Created = %d, want 0", second.Created)
	}
	if second.Reused != first.Files {
		t.Errorf("second run Reused = %d, want %d", second.Reused, first.Files)
	}

	snaps, err := s.ListFileSnapshots(ctx, repo.ID, "")
	if err != nil {
		t.Fatalf("ListFileSnapshots: %v", err)
	}
	if len(snaps) != first.Files {
		t.Errorf("snapshot rows = %d, want %d (no duplicates)", len(snaps), first.Files)
	}
}

func TestSnapshotCreatesNewOnChange(t *testing.T) {
	dir := t.TempDir()
	copyFixture(t, dir)

	s := newTestStore(t)
	repo := testRepo(t, s, store.ConnectorLocal)
	ctx := context.Background()
	conn := New(dir, store.ConnectorLocal)

	if _, err := conn.Snapshot(ctx, s, repo.ID); err != nil {
		t.Fatalf("first Snapshot: %v", err)
	}
	before, err := s.ListFileSnapshots(ctx, repo.ID, "main.go")
	if err != nil || len(before) != 1 {
		t.Fatalf("ListFileSnapshots before change: err=%v rows=%d", err, len(before))
	}

	changed := filepath.Join(dir, "main.go")
	if err := os.WriteFile(changed, []byte("package fixture\n\n// changed\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := conn.Snapshot(ctx, s, repo.ID)
	if err != nil {
		t.Fatalf("second Snapshot: %v", err)
	}
	if result.Created != 1 {
		t.Errorf("Created = %d, want 1", result.Created)
	}

	after, err := s.ListFileSnapshots(ctx, repo.ID, "main.go")
	if err != nil {
		t.Fatalf("ListFileSnapshots after change: %v", err)
	}
	if len(after) != 2 {
		t.Fatalf("main.go snapshot rows = %d, want 2 (old retained, new created)", len(after))
	}
	if after[0].ID != before[0].ID {
		t.Errorf("original snapshot row %s missing after change (got %s first)", before[0].ID, after[0].ID)
	}
}

func TestSnapshotSkipsGitAndGlossDirs(t *testing.T) {
	dir := t.TempDir()
	copyFixture(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, ".gloss"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gloss", "gloss.db"), []byte("binary"), 0o644); err != nil {
		t.Fatal(err)
	}
	initGitRepo(t, dir)
	commitAll(t, dir, "initial")

	s := newTestStore(t)
	repo := testRepo(t, s, store.ConnectorGit)
	ctx := context.Background()

	if _, err := New(dir, store.ConnectorGit).Snapshot(ctx, s, repo.ID); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	snaps, err := s.ListFileSnapshots(ctx, repo.ID, "")
	if err != nil {
		t.Fatalf("ListFileSnapshots: %v", err)
	}
	for _, snap := range snaps {
		if strings.HasPrefix(snap.Path, ".git/") || strings.HasPrefix(snap.Path, ".gloss/") {
			t.Errorf("snapshot includes housekeeping path %s", snap.Path)
		}
	}
}

func TestSnapshotSkipsSymlinks(t *testing.T) {
	dir := t.TempDir()
	copyFixture(t, dir)
	if err := os.Symlink(filepath.Join(dir, "main.go"), filepath.Join(dir, "main-link.go")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	s := newTestStore(t)
	repo := testRepo(t, s, store.ConnectorLocal)
	ctx := context.Background()

	result, err := New(dir, store.ConnectorLocal).Snapshot(ctx, s, repo.ID)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if result.Skipped != 1 {
		t.Errorf("Result.Skipped = %d, want 1", result.Skipped)
	}

	snaps, err := s.ListFileSnapshots(ctx, repo.ID, "main-link.go")
	if err != nil {
		t.Fatalf("ListFileSnapshots: %v", err)
	}
	if len(snaps) != 0 {
		t.Errorf("symlink was snapshotted as its own path: %v", snaps)
	}
}

func TestSnapshotSkipsUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: file permission bits are not enforced")
	}
	dir := t.TempDir()
	copyFixture(t, dir)
	path := filepath.Join(dir, "main.go")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	s := newTestStore(t)
	repo := testRepo(t, s, store.ConnectorLocal)
	ctx := context.Background()

	result, err := New(dir, store.ConnectorLocal).Snapshot(ctx, s, repo.ID)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if result.Skipped != 1 {
		t.Errorf("Result.Skipped = %d, want 1", result.Skipped)
	}
	if len(result.Errors) != 1 {
		t.Errorf("Result.Errors = %v, want 1 entry", result.Errors)
	}
}
