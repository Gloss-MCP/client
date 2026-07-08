# MCP API (design contract)

The tool surface the local MCP server exposes, from the Gloss product
design (business repo, `ideas/gloss.md`). This is the design contract
for milestone 6 (see [PLAN.md](../PLAN.md)) — parameter shapes may be
refined at implementation, and this doc is updated in the same PR when
they are.

The MCP server is **readable and writable**: AI can read sessions,
threads, and comment chains; reply to threads; create new threads
(AI-initiated review); and resolve threads. This enables the full
async loop: human reviews → AI responds in context → human continues
the thread → AI uses resolved threads as structured context for the
next generation pass.

**Transport:** streamable HTTP, served by the already-running `gloss`
binary — the single owner of the SQLite handle. A stdio mode is
deliberately deferred (it would mean a second process contending for
the store).

**Attribution:** every write accepts an optional `author_agent`
(e.g. `claude-opus-4`, `claude-code-session-xyz`); `author_type`
distinguishes `human` from `ai`. See
[data-model.md](data-model.md#comment).

## Sessions

| Tool | Parameters | Returns |
|---|---|---|
| `create_session` | `repo_id`, `name`, `description` | session |
| `list_sessions` | `repo_id`, `status?` | sessions |
| `get_session` | `session_id` | session + stats |

## Threads

| Tool | Parameters | Returns |
|---|---|---|
| `create_thread` | `session_id`, `file_path`, `anchor`, `body`, `author_agent?` | thread |
| `get_thread` | `thread_id` | thread + full comment chain |
| `list_threads` | `session_id`, `file_path?`, `directory?`, `file_type?`, `anchor_status?`, `author_type?`, `author_agent?` | threads |
| `resolve_thread` | `thread_id` | thread |
| `reopen_thread` | `thread_id` | thread |

The `anchor` parameter is the polymorphic Anchor shape defined in
[data-model.md](data-model.md#anchor) — the variant must match what
the file's plugin declares.

## Comments

| Tool | Parameters | Returns |
|---|---|---|
| `add_comment` | `thread_id`, `body`, `parent_comment_id?`, `author_agent?` | comment |
| `edit_comment` | `comment_id`, `body` | comment |
| `delete_comment` | `comment_id` | soft-deleted |

## Repositories

| Tool | Parameters | Returns |
|---|---|---|
| `list_repos` | — | repos |
| `get_repo` | `repo_id` | repo + connector_type + session count |

In local mode there is a single repository record (the directory
`gloss .` was run against), so the repos tools exist mainly for
surface compatibility with the cloud version.
