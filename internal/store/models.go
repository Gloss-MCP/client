package store

import "time"

// ConnectorType identifies the content source backing a repository.
type ConnectorType string

// Connector types (docs/data-model.md#repository).
const (
	ConnectorLocal ConnectorType = "local"
	ConnectorGit   ConnectorType = "git"
)

// SessionStatus is the lifecycle state of a review session.
type SessionStatus string

// Session statuses (docs/data-model.md#session).
const (
	SessionOpen     SessionStatus = "open"
	SessionResolved SessionStatus = "resolved"
	SessionArchived SessionStatus = "archived"
)

// AnchorStatus is the lifecycle state of a thread's anchor.
type AnchorStatus string

// Anchor statuses (docs/data-model.md#thread).
const (
	AnchorActive   AnchorStatus = "active"
	AnchorOrphaned AnchorStatus = "orphaned"
	AnchorResolved AnchorStatus = "resolved"
)

// AuthorType distinguishes human comments from AI ones.
type AuthorType string

// Author types (docs/data-model.md#comment).
const (
	AuthorHuman AuthorType = "human"
	AuthorAI    AuthorType = "ai"
)

// Repository is the content source a session reviews. In local mode
// there is a single repository record: the directory `gloss .` was run
// against.
type Repository struct {
	ID              string        `json:"id"`
	Name            string        `json:"name"`
	ConnectorType   ConnectorType `json:"connector_type"`
	ConnectorConfig string        `json:"connector_config"` // JSON; connector-specific settings
	CreatedAt       time.Time     `json:"created_at"`
}

// Session is the top-level container for a review.
type Session struct {
	ID          string        `json:"id"`
	RepoID      string        `json:"repo_id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Status      SessionStatus `json:"status"`
	CreatedBy   string        `json:"created_by"` // plain identity value; local mode has no users
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// SessionStats summarises a session for get_session-style views.
type SessionStats struct {
	TotalThreads    int `json:"total_threads"`
	ActiveThreads   int `json:"active_threads"`
	OrphanedThreads int `json:"orphaned_threads"`
	ResolvedThreads int `json:"resolved_threads"`
	// TotalComments counts non-deleted comments across the session's
	// threads.
	TotalComments int `json:"total_comments"`
}

// FileSnapshot is the captured state of a file at the moment a thread
// was anchored to it. Delta tracking (milestone 7) compares live content
// against this.
type FileSnapshot struct {
	ID           string    `json:"id"`
	RepoID       string    `json:"repo_id"`
	Path         string    `json:"path"` // relative to the repository root, slash-separated
	ContentHash  string    `json:"content_hash"`
	CapturedAt   time.Time `json:"captured_at"`
	GitCommitSHA string    `json:"git_commit_sha"` // optional; anchor fallback when the repo is git
}

// Thread is an annotation conversation anchored to a position in a
// file. A thread belongs to exactly one session for its lifetime.
type Thread struct {
	ID             string       `json:"id"`
	SessionID      string       `json:"session_id"`
	FileSnapshotID string       `json:"file_snapshot_id"`
	Anchor         Anchor       `json:"anchor"`
	AnchorStatus   AnchorStatus `json:"anchor_status"`
	CreatedBy      string       `json:"created_by"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// Comment is a message in a thread, from a human or an AI. Nested
// replies via ParentCommentID give a full conversation history per
// annotation.
type Comment struct {
	ID              string     `json:"id"`
	ThreadID        string     `json:"thread_id"`
	ParentCommentID string     `json:"parent_comment_id,omitempty"` // empty for top-level comments
	AuthorType      AuthorType `json:"author_type"`
	AuthorAgent     string     `json:"author_agent,omitempty"` // optional; e.g. "claude-opus-4"
	Body            string     `json:"body"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"` // soft delete
}
