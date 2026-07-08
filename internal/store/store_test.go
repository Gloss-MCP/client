package store

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// newTestStore opens a real SQLite database in a temp dir; migrations
// run on open.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "gloss.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return s
}

// Fixture helpers: each creates a minimal valid record, failing the
// test on error.

func testRepo(t *testing.T, s *Store) *Repository {
	t.Helper()
	r, err := s.CreateRepository(context.Background(), "fixture-repo", ConnectorLocal, "")
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	return r
}

func testSession(t *testing.T, s *Store, repoID string) *Session {
	t.Helper()
	sess, err := s.CreateSession(context.Background(), repoID, "fixture session", "", "ben")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return sess
}

func testSnapshot(t *testing.T, s *Store, repoID, path string) *FileSnapshot {
	t.Helper()
	snap, err := s.CreateFileSnapshot(context.Background(), repoID, path, "hash-"+path, "")
	if err != nil {
		t.Fatalf("CreateFileSnapshot: %v", err)
	}
	return snap
}

func testLineAnchor() LineAnchor {
	return LineAnchor{
		StartLine:     10,
		EndLine:       12,
		ContextBefore: []string{"func main() {"},
		ContextAfter:  []string{"}"},
	}
}

func testThread(t *testing.T, s *Store, sessionID, snapshotID string) *Thread {
	t.Helper()
	thread, _, err := s.CreateThread(context.Background(), CreateThreadParams{
		SessionID:      sessionID,
		FileSnapshotID: snapshotID,
		Anchor:         testLineAnchor(),
		CreatedBy:      "ben",
		Body:           "fixture root comment",
		AuthorType:     AuthorHuman,
	})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	return thread
}

func TestOpenAppliesMigrationsFromEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gloss.db")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("database file not created: %v", err)
	}

	var version int
	if err := s.db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != 1 {
		t.Errorf("schema version = %d, want 1", version)
	}

	for _, table := range []string{"repositories", "sessions", "file_snapshots", "threads", "comments"} {
		var n int
		err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&n)
		if err != nil || n != 1 {
			t.Errorf("table %s missing after migration (n=%d, err=%v)", table, n, err)
		}
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gloss.db")

	s1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	repo := testRepo(t, s1)
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopening an already-migrated database must not re-apply
	// migrations or disturb data.
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer func() { _ = s2.Close() }()

	got, err := s2.GetRepository(context.Background(), repo.ID)
	if err != nil {
		t.Fatalf("GetRepository after reopen: %v", err)
	}
	if got.Name != repo.Name {
		t.Errorf("repository name = %q, want %q", got.Name, repo.Name)
	}

	var count int
	if err := s2.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != 1 {
		t.Errorf("schema_migrations rows = %d, want 1", count)
	}
}

func TestOpenMissingParentDirFails(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "missing", "gloss.db")); err == nil {
		t.Fatal("Open with missing parent directory succeeded, want error")
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateSession(context.Background(), "no-such-repo", "s", "", "ben"); err == nil {
		t.Fatal("CreateSession with dangling repo_id succeeded, want FK error")
	}
}

func TestNewID(t *testing.T) {
	uuidRE := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	seen := make(map[string]bool)
	for range 100 {
		id := newID()
		if !uuidRE.MatchString(id) {
			t.Fatalf("newID() = %q, not a UUIDv4", id)
		}
		if seen[id] {
			t.Fatalf("newID() repeated %q", id)
		}
		seen[id] = true
	}
}
