# Data Model (local subset)

The entities the local client persists in SQLite. This is the local
subset of the full Gloss product design (business repo,
`ideas/gloss.md`): cloud-only entities — users, organisations,
subscriptions, billing — are deliberately absent. See
[Local adaptations](#local-adaptations) for the deltas.

Implemented by milestone 2 (see [PLAN.md](../PLAN.md)); anchors are
exercised by milestones 5, 7, 8, and 9.

## Repository

The content source a session reviews. In local mode there is a single
repository record representing the directory `gloss .` was run
against.

| Field | Notes |
|---|---|
| `id` | |
| `name` | Defaults to the directory name |
| `connector_type` | `local` \| `git` (a local directory that is also a git repo) |
| `connector_config` | JSON; connector-specific settings |
| `created_at` | |

## Session

The top-level container for a review — e.g. "architecture review June
2025" or "API design concerns".

| Field | Notes |
|---|---|
| `id` | |
| `repo_id` | |
| `name` | |
| `description` | |
| `status` | `open` \| `resolved` \| `archived` |
| `created_by` | Author identity value (see Local adaptations) |
| `created_at`, `updated_at` | |

## FileSnapshot

The captured state of a file at the moment a thread was anchored to
it. Delta tracking (milestone 7) compares live content against this.

| Field | Notes |
|---|---|
| `id` | |
| `repo_id` | |
| `path` | Relative to the repository root |
| `content_hash` | |
| `captured_at` | |
| `git_commit_sha` | Optional; anchor fallback when the repo is git |

## Anchor

Polymorphic position of a thread within a file, embedded in Thread —
a dedicated type, not a grab-bag of nullable columns. The rendering
plugin decides which variant applies to its file type; core persists
and renders whatever anchor shape the plugin hands it, opaquely.

| Field | Variants | Notes |
|---|---|---|
| `type` | | `line` \| `region` \| `time` \| `region_time` |
| `start_line`, `end_line` | `line` | |
| `context_before`, `context_after` | `line` | Surrounding context lines captured for delta remapping |
| `x`, `y`, `width`, `height` | `region`, `region_time` | Percentages of the rendered asset |
| `start_time`, `end_time` | `time`, `region_time` | |

Variant usage by built-in plugins: text formats (code, markdown, json,
csv) use `line`; image uses `region`; audio uses `time`; video uses
`region_time` (a region *at* a point in time).

### Delta tracking

When a file changes, line numbers shift. On change, the stored context
lines are fuzzy-matched against the new content to remap `line`
anchors. If remapping fails, the thread's `anchor_status` becomes
`orphaned` — visible but flagged; comments are never silently lost.
For git repos, `FileSnapshot.git_commit_sha` is the additional
fallback. Implemented in milestone 7 (`internal/delta`).

## Thread

An annotation conversation anchored to a position in a file. A thread
belongs to exactly one session for its lifetime; comments bind to a
session only implicitly, through their thread.

| Field | Notes |
|---|---|
| `id` | |
| `session_id` | |
| `file_snapshot_id` | |
| `anchor` | Embedded Anchor (above) |
| `anchor_status` | `active` \| `orphaned` \| `resolved` |
| `created_by` | Author identity value |
| `created_at`, `updated_at` | |

## Comment

A message in a thread, from a human or an AI. Nested replies via
`parent_comment_id` give a full conversation history per annotation.

| Field | Notes |
|---|---|
| `id` | |
| `thread_id` | |
| `parent_comment_id` | Optional; nesting |
| `author_type` | `human` \| `ai` |
| `author_agent` | Optional; e.g. `claude-opus-4`, `claude-code-session-xyz` |
| `body` | |
| `created_at`, `updated_at` | |
| `deleted_at` | Soft delete |

## Local adaptations

Deliberate deltas from the full cloud data model:

- **No users or organisations.** Local mode has no auth and a single
  implicit user, so there are no `User`/`Organisation` tables.
  `created_by` and comment author identity are plain stored values
  (e.g. an author label or agent string), not foreign keys.
- **No subscriptions or billing.** Cloud-side entirely.
- The cloud superset — User, Organisation, OrganisationMember,
  Subscription, and the wider `connector_type` enum (github, gitlab,
  obsidian) — is documented in the business repo and arrives with
  `gloss-cloud`.
