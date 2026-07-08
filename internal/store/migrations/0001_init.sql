-- Initial schema: the local subset of the Gloss data model, as specified
-- in docs/data-model.md. Timestamps are RFC3339 UTC strings; enums are
-- enforced with CHECK constraints.

CREATE TABLE repositories (
    id               TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    connector_type   TEXT NOT NULL CHECK (connector_type IN ('local', 'git')),
    connector_config TEXT NOT NULL DEFAULT '{}',
    created_at       TEXT NOT NULL
);

CREATE TABLE sessions (
    id          TEXT PRIMARY KEY,
    repo_id     TEXT NOT NULL REFERENCES repositories(id),
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved', 'archived')),
    created_by  TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE INDEX idx_sessions_repo ON sessions(repo_id);

CREATE TABLE file_snapshots (
    id             TEXT PRIMARY KEY,
    repo_id        TEXT NOT NULL REFERENCES repositories(id),
    path           TEXT NOT NULL,
    content_hash   TEXT NOT NULL,
    captured_at    TEXT NOT NULL,
    git_commit_sha TEXT
);

CREATE INDEX idx_file_snapshots_repo_path ON file_snapshots(repo_id, path);

-- The polymorphic anchor is embedded as a type discriminator plus an
-- opaque JSON payload: core persists whatever anchor shape the plugin
-- hands it (docs/data-model.md#anchor).
CREATE TABLE threads (
    id               TEXT PRIMARY KEY,
    session_id       TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    file_snapshot_id TEXT NOT NULL REFERENCES file_snapshots(id),
    anchor_type      TEXT NOT NULL CHECK (anchor_type IN ('line', 'region', 'time', 'region_time')),
    anchor           TEXT NOT NULL,
    anchor_status    TEXT NOT NULL DEFAULT 'active' CHECK (anchor_status IN ('active', 'orphaned', 'resolved')),
    created_by       TEXT NOT NULL,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);

CREATE INDEX idx_threads_session ON threads(session_id);
CREATE INDEX idx_threads_file_snapshot ON threads(file_snapshot_id);

CREATE TABLE comments (
    id                TEXT PRIMARY KEY,
    thread_id         TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    parent_comment_id TEXT REFERENCES comments(id) ON DELETE CASCADE,
    author_type       TEXT NOT NULL CHECK (author_type IN ('human', 'ai')),
    author_agent      TEXT,
    body              TEXT NOT NULL,
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL,
    deleted_at        TEXT
);

CREATE INDEX idx_comments_thread ON comments(thread_id);
CREATE INDEX idx_comments_parent ON comments(parent_comment_id);
