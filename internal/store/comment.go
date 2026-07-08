package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// AddCommentParams collects the inputs for AddComment.
type AddCommentParams struct {
	ThreadID string
	// ParentCommentID nests the comment under an existing comment in
	// the same thread; empty means a top-level reply to the thread.
	ParentCommentID string
	AuthorType      AuthorType
	AuthorAgent     string // optional
	Body            string
}

// AddComment appends a comment to a thread.
func (s *Store) AddComment(ctx context.Context, p AddCommentParams) (*Comment, error) {
	if _, err := s.getThread(ctx, p.ThreadID); err != nil {
		return nil, err
	}
	if p.ParentCommentID != "" {
		parent, err := s.GetComment(ctx, p.ParentCommentID)
		if err != nil {
			return nil, err
		}
		if parent.ThreadID != p.ThreadID {
			return nil, fmt.Errorf("store: parent comment %s belongs to thread %s, not %s",
				p.ParentCommentID, parent.ThreadID, p.ThreadID)
		}
	}

	now := time.Now().UTC()
	c := &Comment{
		ID:              newID(),
		ThreadID:        p.ThreadID,
		ParentCommentID: p.ParentCommentID,
		AuthorType:      p.AuthorType,
		AuthorAgent:     p.AuthorAgent,
		Body:            p.Body,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO comments (id, thread_id, parent_comment_id, author_type, author_agent, body, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.ThreadID, nullable(c.ParentCommentID), string(c.AuthorType),
		nullable(c.AuthorAgent), c.Body, formatTime(c.CreatedAt), formatTime(c.UpdatedAt))
	if err != nil {
		return nil, fmt.Errorf("store: add comment: %w", err)
	}
	return c, nil
}

// GetComment fetches a comment by ID, soft-deleted or not.
func (s *Store) GetComment(ctx context.Context, id string) (*Comment, error) {
	row := s.db.QueryRowContext(ctx, selectComments+` WHERE id = ?`, id)
	c, err := scanComment(row)
	if err != nil {
		return nil, fmt.Errorf("store: get comment %s: %w", id, err)
	}
	return c, nil
}

// UpdateCommentBody replaces a comment's body and bumps its updated_at.
// Soft-deleted comments cannot be edited.
func (s *Store) UpdateCommentBody(ctx context.Context, id, body string) (*Comment, error) {
	c, err := s.GetComment(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.DeletedAt != nil {
		return nil, fmt.Errorf("store: update comment %s: %w", id, ErrCommentDeleted)
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE comments SET body = ?, updated_at = ? WHERE id = ?`,
		body, formatTime(time.Now()), id); err != nil {
		return nil, fmt.Errorf("store: update comment %s: %w", id, err)
	}
	return s.GetComment(ctx, id)
}

// SoftDeleteComment marks a comment deleted, keeping the row so nested
// replies stay attached. Deleting an already-deleted comment is a
// no-op.
func (s *Store) SoftDeleteComment(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE comments SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL`,
		formatTime(time.Now()), id)
	if err != nil {
		return fmt.Errorf("store: delete comment %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete comment %s: %w", id, err)
	}
	if n == 0 {
		// Distinguish "already deleted" (idempotent success) from
		// "no such comment".
		if _, err := s.GetComment(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// ListComments returns a thread's comments oldest first, including
// soft-deleted ones so the reply structure stays intact; callers decide
// how to render deletions.
func (s *Store) ListComments(ctx context.Context, threadID string) ([]*Comment, error) {
	rows, err := s.db.QueryContext(ctx,
		selectComments+` WHERE thread_id = ? ORDER BY created_at, id`, threadID)
	if err != nil {
		return nil, fmt.Errorf("store: list comments: %w", err)
	}
	defer rows.Close()

	var comments []*Comment
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list comments: %w", err)
		}
		comments = append(comments, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list comments: %w", err)
	}
	return comments, nil
}

const selectComments = `SELECT id, thread_id, parent_comment_id, author_type, author_agent, body, created_at, updated_at, deleted_at
	 FROM comments`

func scanComment(row scanner) (*Comment, error) {
	var c Comment
	var parentID, authorAgent, deletedAt sql.NullString
	var createdAt, updatedAt string
	err := row.Scan(&c.ID, &c.ThreadID, &parentID, (*string)(&c.AuthorType),
		&authorAgent, &c.Body, &createdAt, &updatedAt, &deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.ParentCommentID = parentID.String
	c.AuthorAgent = authorAgent.String
	if c.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if c.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, err
	}
	if deletedAt.Valid {
		t, err := parseTime(deletedAt.String)
		if err != nil {
			return nil, err
		}
		c.DeletedAt = &t
	}
	return &c, nil
}
