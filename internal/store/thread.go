package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// CreateThreadParams collects the inputs for CreateThread. A thread is
// never created empty: Body and its attribution become the thread's
// root comment, mirroring the MCP create_thread tool
// (docs/mcp-api.md#threads).
type CreateThreadParams struct {
	SessionID      string
	FileSnapshotID string
	Anchor         Anchor
	CreatedBy      string
	Body           string
	AuthorType     AuthorType
	AuthorAgent    string // optional
}

// CreateThread inserts a thread and its root comment atomically.
func (s *Store) CreateThread(ctx context.Context, p CreateThreadParams) (*Thread, *Comment, error) {
	anchorType, anchorJSON, err := marshalAnchor(p.Anchor)
	if err != nil {
		return nil, nil, err
	}

	now := time.Now().UTC()
	t := &Thread{
		ID:             newID(),
		SessionID:      p.SessionID,
		FileSnapshotID: p.FileSnapshotID,
		Anchor:         p.Anchor,
		AnchorStatus:   AnchorActive,
		CreatedBy:      p.CreatedBy,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	c := &Comment{
		ID:          newID(),
		ThreadID:    t.ID,
		AuthorType:  p.AuthorType,
		AuthorAgent: p.AuthorAgent,
		Body:        p.Body,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("store: create thread: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO threads (id, session_id, file_snapshot_id, anchor_type, anchor, anchor_status, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.SessionID, t.FileSnapshotID, string(anchorType), anchorJSON,
		string(t.AnchorStatus), t.CreatedBy, formatTime(t.CreatedAt), formatTime(t.UpdatedAt)); err != nil {
		return nil, nil, fmt.Errorf("store: create thread: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO comments (id, thread_id, parent_comment_id, author_type, author_agent, body, created_at, updated_at)
		 VALUES (?, ?, NULL, ?, ?, ?, ?, ?)`,
		c.ID, c.ThreadID, string(c.AuthorType), nullable(c.AuthorAgent), c.Body,
		formatTime(c.CreatedAt), formatTime(c.UpdatedAt)); err != nil {
		return nil, nil, fmt.Errorf("store: create thread root comment: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("store: create thread: %w", err)
	}
	return t, c, nil
}

// GetThread fetches a thread and its full comment chain (including
// soft-deleted comments, so reply structure survives deletions),
// ordered oldest first.
func (s *Store) GetThread(ctx context.Context, id string) (*Thread, []*Comment, error) {
	row := s.db.QueryRowContext(ctx, selectThreads+` WHERE t.id = ?`, id)
	t, err := scanThread(row)
	if err != nil {
		return nil, nil, fmt.Errorf("store: get thread %s: %w", id, err)
	}
	comments, err := s.ListComments(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return t, comments, nil
}

// ListThreadsFilter narrows ListThreads results; zero-valued fields do
// not filter. The fields mirror the MCP list_threads tool parameters
// (docs/mcp-api.md#threads).
type ListThreadsFilter struct {
	// FilePath matches the snapshot path exactly.
	FilePath string
	// Directory matches snapshot paths under a directory prefix, e.g.
	// "src" matches "src/a.go" and "src/deep/b.go".
	Directory string
	// FileType matches the snapshot path's extension, with or without
	// the leading dot: "go" or ".go".
	FileType string
	// AnchorStatus matches the thread's anchor status.
	AnchorStatus AnchorStatus
	// AuthorType and AuthorAgent match the author of the thread's root
	// (initial) comment — "threads started by X".
	AuthorType  AuthorType
	AuthorAgent string
}

// rootCommentField selects a column of the thread's root (earliest)
// comment as a correlated subquery.
func rootCommentField(column string) string {
	return `(SELECT c.` + column + ` FROM comments c WHERE c.thread_id = t.id ORDER BY c.created_at, c.id LIMIT 1)`
}

// ListThreads returns a session's threads matching the filter, oldest
// first.
func (s *Store) ListThreads(ctx context.Context, sessionID string, f ListThreadsFilter) ([]*Thread, error) {
	query := selectThreads + `
		 JOIN file_snapshots fs ON fs.id = t.file_snapshot_id
		 WHERE t.session_id = ?`
	args := []any{sessionID}

	if f.FilePath != "" {
		query += ` AND fs.path = ?`
		args = append(args, f.FilePath)
	}
	if f.Directory != "" {
		prefix := strings.TrimSuffix(f.Directory, "/") + "/"
		query += ` AND fs.path LIKE ? ESCAPE '\'`
		args = append(args, escapeLike(prefix)+"%")
	}
	if f.FileType != "" {
		ext := strings.TrimPrefix(f.FileType, ".")
		query += ` AND fs.path LIKE ? ESCAPE '\'`
		args = append(args, "%."+escapeLike(ext))
	}
	if f.AnchorStatus != "" {
		query += ` AND t.anchor_status = ?`
		args = append(args, string(f.AnchorStatus))
	}
	if f.AuthorType != "" {
		query += ` AND ` + rootCommentField("author_type") + ` = ?`
		args = append(args, string(f.AuthorType))
	}
	if f.AuthorAgent != "" {
		query += ` AND ` + rootCommentField("author_agent") + ` = ?`
		args = append(args, f.AuthorAgent)
	}
	query += ` ORDER BY t.created_at, t.id`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list threads: %w", err)
	}
	defer rows.Close()

	var threads []*Thread
	for rows.Next() {
		t, err := scanThread(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list threads: %w", err)
		}
		threads = append(threads, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list threads: %w", err)
	}
	return threads, nil
}

// SetAnchorStatus transitions a thread's anchor status — resolve,
// reopen, or orphan — and bumps its updated_at.
func (s *Store) SetAnchorStatus(ctx context.Context, id string, status AnchorStatus) (*Thread, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE threads SET anchor_status = ?, updated_at = ? WHERE id = ?`,
		string(status), formatTime(time.Now()), id)
	if err != nil {
		return nil, fmt.Errorf("store: set thread %s anchor status: %w", id, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return nil, fmt.Errorf("store: set thread %s anchor status: %w", id, err)
	} else if n == 0 {
		return nil, fmt.Errorf("store: set thread %s anchor status: %w", id, ErrNotFound)
	}
	return s.getThread(ctx, id)
}

// UpdateThreadAnchor replaces a thread's anchor — the delta remapper
// (milestone 7) calls this after re-anchoring — and bumps updated_at.
func (s *Store) UpdateThreadAnchor(ctx context.Context, id string, anchor Anchor) (*Thread, error) {
	anchorType, anchorJSON, err := marshalAnchor(anchor)
	if err != nil {
		return nil, err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE threads SET anchor_type = ?, anchor = ?, updated_at = ? WHERE id = ?`,
		string(anchorType), anchorJSON, formatTime(time.Now()), id)
	if err != nil {
		return nil, fmt.Errorf("store: update thread %s anchor: %w", id, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return nil, fmt.Errorf("store: update thread %s anchor: %w", id, err)
	} else if n == 0 {
		return nil, fmt.Errorf("store: update thread %s anchor: %w", id, ErrNotFound)
	}
	return s.getThread(ctx, id)
}

const selectThreads = `SELECT t.id, t.session_id, t.file_snapshot_id, t.anchor_type, t.anchor, t.anchor_status, t.created_by, t.created_at, t.updated_at
		 FROM threads t`

// getThread fetches a thread without its comments.
func (s *Store) getThread(ctx context.Context, id string) (*Thread, error) {
	row := s.db.QueryRowContext(ctx, selectThreads+` WHERE t.id = ?`, id)
	t, err := scanThread(row)
	if err != nil {
		return nil, fmt.Errorf("store: get thread %s: %w", id, err)
	}
	return t, nil
}

func scanThread(row scanner) (*Thread, error) {
	var t Thread
	var anchorType, anchorJSON, createdAt, updatedAt string
	err := row.Scan(&t.ID, &t.SessionID, &t.FileSnapshotID, &anchorType, &anchorJSON,
		(*string)(&t.AnchorStatus), &t.CreatedBy, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if t.Anchor, err = unmarshalAnchor(AnchorType(anchorType), anchorJSON); err != nil {
		return nil, err
	}
	if t.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if t.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, err
	}
	return &t, nil
}

// escapeLike escapes LIKE wildcards so user-supplied fragments match
// literally; pair with ESCAPE '\'.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
