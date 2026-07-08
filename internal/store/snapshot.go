package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// CreateFileSnapshot captures the state of a file at anchoring time.
// gitCommitSHA may be empty when the repository is not git-backed.
func (s *Store) CreateFileSnapshot(ctx context.Context, repoID, path, contentHash, gitCommitSHA string) (*FileSnapshot, error) {
	snap := &FileSnapshot{
		ID:           newID(),
		RepoID:       repoID,
		Path:         path,
		ContentHash:  contentHash,
		CapturedAt:   time.Now().UTC(),
		GitCommitSHA: gitCommitSHA,
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO file_snapshots (id, repo_id, path, content_hash, captured_at, git_commit_sha)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		snap.ID, snap.RepoID, snap.Path, snap.ContentHash,
		formatTime(snap.CapturedAt), nullable(snap.GitCommitSHA))
	if err != nil {
		return nil, fmt.Errorf("store: create file snapshot: %w", err)
	}
	return snap, nil
}

// GetFileSnapshot fetches a file snapshot by ID.
func (s *Store) GetFileSnapshot(ctx context.Context, id string) (*FileSnapshot, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, repo_id, path, content_hash, captured_at, git_commit_sha
		 FROM file_snapshots WHERE id = ?`, id)
	snap, err := scanFileSnapshot(row)
	if err != nil {
		return nil, fmt.Errorf("store: get file snapshot %s: %w", id, err)
	}
	return snap, nil
}

// FindFileSnapshot returns the most recent snapshot of a path with the
// given content hash, so unchanged files reuse their snapshot instead of
// accumulating duplicates.
func (s *Store) FindFileSnapshot(ctx context.Context, repoID, path, contentHash string) (*FileSnapshot, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, repo_id, path, content_hash, captured_at, git_commit_sha
		 FROM file_snapshots
		 WHERE repo_id = ? AND path = ? AND content_hash = ?
		 ORDER BY captured_at DESC, id LIMIT 1`,
		repoID, path, contentHash)
	snap, err := scanFileSnapshot(row)
	if err != nil {
		return nil, fmt.Errorf("store: find file snapshot %s@%s: %w", path, contentHash, err)
	}
	return snap, nil
}

// ListFileSnapshots returns a repository's snapshots, oldest first. An
// empty path means no path filter.
func (s *Store) ListFileSnapshots(ctx context.Context, repoID, path string) ([]*FileSnapshot, error) {
	query := `SELECT id, repo_id, path, content_hash, captured_at, git_commit_sha
		 FROM file_snapshots WHERE repo_id = ?`
	args := []any{repoID}
	if path != "" {
		query += ` AND path = ?`
		args = append(args, path)
	}
	query += ` ORDER BY captured_at, id`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list file snapshots: %w", err)
	}
	defer rows.Close()

	var snaps []*FileSnapshot
	for rows.Next() {
		snap, err := scanFileSnapshot(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list file snapshots: %w", err)
		}
		snaps = append(snaps, snap)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list file snapshots: %w", err)
	}
	return snaps, nil
}

func scanFileSnapshot(row scanner) (*FileSnapshot, error) {
	var snap FileSnapshot
	var capturedAt string
	var gitSHA sql.NullString
	err := row.Scan(&snap.ID, &snap.RepoID, &snap.Path, &snap.ContentHash, &capturedAt, &gitSHA)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if snap.CapturedAt, err = parseTime(capturedAt); err != nil {
		return nil, err
	}
	snap.GitCommitSHA = gitSHA.String
	return &snap, nil
}
