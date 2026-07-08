package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// CreateSession inserts a new session in the open state.
func (s *Store) CreateSession(ctx context.Context, repoID, name, description, createdBy string) (*Session, error) {
	now := time.Now().UTC()
	sess := &Session{
		ID:          newID(),
		RepoID:      repoID,
		Name:        name,
		Description: description,
		Status:      SessionOpen,
		CreatedBy:   createdBy,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, repo_id, name, description, status, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.RepoID, sess.Name, sess.Description, string(sess.Status),
		sess.CreatedBy, formatTime(sess.CreatedAt), formatTime(sess.UpdatedAt))
	if err != nil {
		return nil, fmt.Errorf("store: create session: %w", err)
	}
	return sess, nil
}

// GetSession fetches a session by ID.
func (s *Store) GetSession(ctx context.Context, id string) (*Session, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, repo_id, name, description, status, created_by, created_at, updated_at
		 FROM sessions WHERE id = ?`, id)
	sess, err := scanSession(row)
	if err != nil {
		return nil, fmt.Errorf("store: get session %s: %w", id, err)
	}
	return sess, nil
}

// ListSessions returns a repository's sessions, oldest first. A zero
// status means no status filter.
func (s *Store) ListSessions(ctx context.Context, repoID string, status SessionStatus) ([]*Session, error) {
	query := `SELECT id, repo_id, name, description, status, created_by, created_at, updated_at
		 FROM sessions WHERE repo_id = ?`
	args := []any{repoID}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, string(status))
	}
	query += ` ORDER BY created_at, id`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list sessions: %w", err)
		}
		sessions = append(sessions, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list sessions: %w", err)
	}
	return sessions, nil
}

// UpdateSession sets a session's name, description, and status, and
// bumps its updated_at.
func (s *Store) UpdateSession(ctx context.Context, id, name, description string, status SessionStatus) (*Session, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET name = ?, description = ?, status = ?, updated_at = ? WHERE id = ?`,
		name, description, string(status), formatTime(time.Now()), id)
	if err != nil {
		return nil, fmt.Errorf("store: update session %s: %w", id, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return nil, fmt.Errorf("store: update session %s: %w", id, err)
	} else if n == 0 {
		return nil, fmt.Errorf("store: update session %s: %w", id, ErrNotFound)
	}
	return s.GetSession(ctx, id)
}

// DeleteSession hard-deletes a session; its threads and their comments
// go with it via ON DELETE CASCADE.
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete session %s: %w", id, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("store: delete session %s: %w", id, err)
	} else if n == 0 {
		return fmt.Errorf("store: delete session %s: %w", id, ErrNotFound)
	}
	return nil
}

// GetSessionStats summarises a session's threads and comments.
func (s *Store) GetSessionStats(ctx context.Context, id string) (*SessionStats, error) {
	if _, err := s.GetSession(ctx, id); err != nil {
		return nil, err
	}

	var stats SessionStats
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*),
		        COALESCE(SUM(anchor_status = 'active'), 0),
		        COALESCE(SUM(anchor_status = 'orphaned'), 0),
		        COALESCE(SUM(anchor_status = 'resolved'), 0)
		 FROM threads WHERE session_id = ?`, id).
		Scan(&stats.TotalThreads, &stats.ActiveThreads, &stats.OrphanedThreads, &stats.ResolvedThreads)
	if err != nil {
		return nil, fmt.Errorf("store: session %s stats: %w", id, err)
	}

	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM comments c
		 JOIN threads t ON t.id = c.thread_id
		 WHERE t.session_id = ? AND c.deleted_at IS NULL`, id).
		Scan(&stats.TotalComments)
	if err != nil {
		return nil, fmt.Errorf("store: session %s stats: %w", id, err)
	}
	return &stats, nil
}

func scanSession(row scanner) (*Session, error) {
	var sess Session
	var createdAt, updatedAt string
	err := row.Scan(&sess.ID, &sess.RepoID, &sess.Name, &sess.Description,
		(*string)(&sess.Status), &sess.CreatedBy, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if sess.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if sess.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, err
	}
	return &sess, nil
}
