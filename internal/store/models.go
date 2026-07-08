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
	ID              string
	Name            string
	ConnectorType   ConnectorType
	ConnectorConfig string // JSON; connector-specific settings
	CreatedAt       time.Time
}

// Session is the top-level container for a review.
type Session struct {
	ID          string
	RepoID      string
	Name        string
	Description string
	Status      SessionStatus
	CreatedBy   string // plain identity value; local mode has no users
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// SessionStats summarises a session for get_session-style views.
type SessionStats struct {
	TotalThreads    int
	ActiveThreads   int
	OrphanedThreads int
	ResolvedThreads int
	// TotalComments counts non-deleted comments across the session's
	// threads.
	TotalComments int
}

// FileSnapshot is the captured state of a file at the moment a thread
// was anchored to it. Delta tracking (milestone 7) compares live content
// against this.
type FileSnapshot struct {
	ID           string
	RepoID       string
	Path         string // relative to the repository root, slash-separated
	ContentHash  string
	CapturedAt   time.Time
	GitCommitSHA string // optional; anchor fallback when the repo is git
}

// Thread is an annotation conversation anchored to a position in a
// file. A thread belongs to exactly one session for its lifetime.
type Thread struct {
	ID             string
	SessionID      string
	FileSnapshotID string
	Anchor         Anchor
	AnchorStatus   AnchorStatus
	CreatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Comment is a message in a thread, from a human or an AI. Nested
// replies via ParentCommentID give a full conversation history per
// annotation.
type Comment struct {
	ID              string
	ThreadID        string
	ParentCommentID string // empty for top-level comments
	AuthorType      AuthorType
	AuthorAgent     string // optional; e.g. "claude-opus-4"
	Body            string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time // soft delete
}
