-- Allow the 'cell' anchor type (CSV cells; docs/data-model.md#anchor).
--
-- SQLite cannot alter a CHECK constraint, so threads is rebuilt. With
-- foreign keys enforced, dropping threads while comments still references
-- it would cascade-delete every comment, so comments is rebuilt in the
-- same migration: copy both tables, drop children first, then rename.
-- Foreign keys are deferred to commit so row copy order is irrelevant.
-- comments_new references the *_new tables, never the old ones: a
-- reference to the old threads table would cascade-delete the copied
-- comments when it is dropped. SQLite rewrites the references when the
-- new tables are renamed into place.

PRAGMA defer_foreign_keys = ON;

CREATE TABLE threads_new (
    id               TEXT PRIMARY KEY,
    session_id       TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    file_snapshot_id TEXT NOT NULL REFERENCES file_snapshots(id),
    anchor_type      TEXT NOT NULL CHECK (anchor_type IN ('line', 'cell', 'region', 'time', 'region_time')),
    anchor           TEXT NOT NULL,
    anchor_status    TEXT NOT NULL DEFAULT 'active' CHECK (anchor_status IN ('active', 'orphaned', 'resolved')),
    created_by       TEXT NOT NULL,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);

INSERT INTO threads_new
    (id, session_id, file_snapshot_id, anchor_type, anchor, anchor_status, created_by, created_at, updated_at)
SELECT id, session_id, file_snapshot_id, anchor_type, anchor, anchor_status, created_by, created_at, updated_at
FROM threads;

CREATE TABLE comments_new (
    id                TEXT PRIMARY KEY,
    thread_id         TEXT NOT NULL REFERENCES threads_new(id) ON DELETE CASCADE,
    parent_comment_id TEXT REFERENCES comments_new(id) ON DELETE CASCADE,
    author_type       TEXT NOT NULL CHECK (author_type IN ('human', 'ai')),
    author_agent      TEXT,
    body              TEXT NOT NULL,
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL,
    deleted_at        TEXT
);

INSERT INTO comments_new
    (id, thread_id, parent_comment_id, author_type, author_agent, body, created_at, updated_at, deleted_at)
SELECT id, thread_id, parent_comment_id, author_type, author_agent, body, created_at, updated_at, deleted_at
FROM comments;

DROP TABLE comments;
DROP TABLE threads;

ALTER TABLE threads_new RENAME TO threads;
ALTER TABLE comments_new RENAME TO comments;

CREATE INDEX idx_threads_session ON threads(session_id);
CREATE INDEX idx_threads_file_snapshot ON threads(file_snapshot_id);
CREATE INDEX idx_comments_thread ON comments(thread_id);
CREATE INDEX idx_comments_parent ON comments(parent_comment_id);
