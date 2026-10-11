package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestMigration0002PreservesThreadsAndComments applies migration 1, loads
// data, then applies migration 2 against a foreign-keys-on connection
// (the production setting) and checks nothing was lost or cascade-deleted.
func TestMigration0002PreservesThreadsAndComments(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "gloss.db") + "?_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1) // keep one connection so the pragma and data are shared

	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := applyMigration(db, 1, "0001_init.sql"); err != nil {
		t.Fatalf("migration 1: %v", err)
	}

	const ts = "2026-01-01T00:00:00.000000000Z"
	for _, q := range []string{
		`INSERT INTO repositories (id, name, connector_type, created_at) VALUES ('r1','repo','local','` + ts + `')`,
		`INSERT INTO sessions (id, repo_id, name, created_by, created_at, updated_at) VALUES ('s1','r1','sess','me','` + ts + `','` + ts + `')`,
		`INSERT INTO file_snapshots (id, repo_id, path, content_hash, captured_at) VALUES ('f1','r1','a.go','h','` + ts + `')`,
		`INSERT INTO threads (id, session_id, file_snapshot_id, anchor_type, anchor, created_by, created_at, updated_at) VALUES ('t1','s1','f1','line','{"start_line":1,"end_line":1}','me','` + ts + `','` + ts + `')`,
		`INSERT INTO comments (id, thread_id, author_type, body, created_at, updated_at) VALUES ('c1','t1','human','root','` + ts + `','` + ts + `')`,
		`INSERT INTO comments (id, thread_id, parent_comment_id, author_type, author_agent, body, created_at, updated_at) VALUES ('c2','t1','c1','ai','claude','reply','` + ts + `','` + ts + `')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}

	if err := applyMigration(db, 2, "0002_cell_anchor.sql"); err != nil {
		t.Fatalf("migration 2: %v", err)
	}

	var threads, comments int
	if err := db.QueryRow(`SELECT COUNT(*) FROM threads`).Scan(&threads); err != nil || threads != 1 {
		t.Errorf("threads = %d (err %v), want 1", threads, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM comments`).Scan(&comments); err != nil || comments != 2 {
		t.Errorf("comments = %d (err %v), want 2 (migration must not cascade-delete)", comments, err)
	}
	var parent string
	if err := db.QueryRow(`SELECT parent_comment_id FROM comments WHERE id = 'c2'`).Scan(&parent); err != nil || parent != "c1" {
		t.Errorf("reply parent = %q (err %v), want c1", parent, err)
	}

	// 'cell' is now accepted; unknown types still rejected.
	if _, err := db.Exec(`INSERT INTO threads (id, session_id, file_snapshot_id, anchor_type, anchor, created_by, created_at, updated_at) VALUES ('t2','s1','f1','cell','{}','me','` + ts + `','` + ts + `')`); err != nil {
		t.Errorf("cell thread rejected: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO threads (id, session_id, file_snapshot_id, anchor_type, anchor, created_by, created_at, updated_at) VALUES ('t3','s1','f1','bogus','{}','me','` + ts + `','` + ts + `')`); err == nil {
		t.Error("bogus anchor type accepted")
	}

	// Foreign keys and cascades still work on the rebuilt tables.
	if _, err := db.Exec(`DELETE FROM threads WHERE id = 't1'`); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM comments`).Scan(&comments); err != nil || comments != 0 {
		t.Errorf("comments after thread delete = %d (err %v), want 0 (cascade intact)", comments, err)
	}
	if _, err := db.Exec(`INSERT INTO comments (id, thread_id, author_type, body, created_at, updated_at) VALUES ('c9','missing','human','x','` + ts + `','` + ts + `')`); err == nil {
		t.Error("comment with dangling thread_id accepted; foreign key lost")
	}

	for _, idx := range []string{"idx_threads_session", "idx_threads_file_snapshot", "idx_comments_thread", "idx_comments_parent"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, idx).Scan(&n); err != nil || n != 1 {
			t.Errorf("index %s missing", idx)
		}
	}
}
