package store

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrate applies, in order, every embedded migration with a version
// greater than the current schema version. Each migration runs in its
// own transaction together with the version bookkeeping.
func migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("store: create schema_migrations: %w", err)
	}

	var current int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return fmt.Errorf("store: read schema version: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("store: read migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		version, err := migrationVersion(name)
		if err != nil {
			return err
		}
		if version <= current {
			continue
		}
		if err := applyMigration(db, version, name); err != nil {
			return err
		}
	}
	return nil
}

// migrationVersion extracts the numeric prefix of a migration filename,
// e.g. "0001_init.sql" -> 1.
func migrationVersion(name string) (int, error) {
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("store: migration %s: name must be <version>_<label>.sql", name)
	}
	version, err := strconv.Atoi(prefix)
	if err != nil || version <= 0 {
		return 0, fmt.Errorf("store: migration %s: invalid version prefix %q", name, prefix)
	}
	return version, nil
}

func applyMigration(db *sql.DB, version int, name string) error {
	ddl, err := migrationsFS.ReadFile("migrations/" + name)
	if err != nil {
		return fmt.Errorf("store: read migration %s: %w", name, err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("store: migration %s: %w", name, err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit

	if _, err := tx.Exec(string(ddl)); err != nil {
		return fmt.Errorf("store: apply migration %s: %w", name, err)
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
		version, formatTime(time.Now())); err != nil {
		return fmt.Errorf("store: record migration %s: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit migration %s: %w", name, err)
	}
	return nil
}
