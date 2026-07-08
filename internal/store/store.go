// Package store is the SQLite-backed persistence layer for the local
// Gloss client. It persists the local subset of the Gloss data model
// (docs/data-model.md): repositories, sessions, file snapshots, threads
// with embedded polymorphic anchors, and comments.
//
// The driver is modernc.org/sqlite (pure Go); the build must stay
// CGO-free.
package store

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

// ErrNotFound is returned (wrapped) when a requested record does not
// exist.
var ErrNotFound = errors.New("not found")

// ErrCommentDeleted is returned when mutating a soft-deleted comment.
var ErrCommentDeleted = errors.New("comment is deleted")

// Store provides CRUD over the Gloss SQLite database. It is safe for
// concurrent use; the underlying pool applies the connection pragmas to
// every connection.
type Store struct {
	db *sql.DB
}

// Open opens (creating it if necessary) the SQLite database at path and
// applies any pending migrations. The parent directory must already
// exist.
func Open(path string) (*Store, error) {
	// Foreign keys are off by default in SQLite and must be enabled per
	// connection, hence the DSN pragma rather than a one-off Exec.
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database handle.
func (s *Store) Close() error {
	return s.db.Close()
}

// timeFormat is RFC3339 UTC with fixed-width nanoseconds so that stored
// timestamps sort lexicographically in chronological order.
const timeFormat = "2006-01-02T15:04:05.000000000Z07:00"

func formatTime(t time.Time) string {
	return t.UTC().Format(timeFormat)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}

// nullable maps the empty string to SQL NULL for optional text columns.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// newID returns a random UUIDv4 string. Hand-rolled to avoid a
// dependency for sixteen random bytes.
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("store: read random bytes: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
