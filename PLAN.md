# Gloss Client — Delivery Plan

Delivery plan for this repo, the open-source half of Gloss. The product
design this plan implements lives in the business repo
(`gloss-mcp/business`, `ideas/gloss.md`). A single Go binary: run
`gloss .` against any directory and it boots a localhost HTMX web UI plus
an MCP server, backed by SQLite. No auth, no account, no internet
connection.

**End state of this plan:** a stranger can install the binary (Homebrew
or curl script), run `gloss .`, get a browser tab with the review UI,
point Claude Code at the MCP endpoint, and run a full human-reviews →
AI-responds annotation loop — with tests, CI, and a public docs website
behind it.

## Scope

**In scope:** local server mode only — web UI, MCP server, SQLite store,
local directory + git connectors, plugin architecture, delta tracking,
cross-platform distribution, project website.

**Out of scope:** everything that needs the cloud. Proxy mode
(`gloss --cloud`) is deferred until the cloud backend exists — the CLI
flag surface is reserved (it exists and errors with "not yet available")
and internal boundaries stay clean, but no tunnel gets built. Auth,
billing, orgs, GitHub/GitLab/Obsidian connectors: all cloud-side, all
later.

**Protobufs deliberately deferred.** This is a conscious deviation from
the "protobufs for contracts" preference: protobuf is the agent↔cloud
wire contract, and local-only scope has no cross-service boundary. MCP
dictates JSON-RPC; SQLite needs no serialisation contract. The `proto/`
submodule arrives with proxy-mode work.

## Key Technical Decisions

Recorded up front so every milestone inherits them.

- **SQLite as the runtime store** (decided 2026-07-07, over a text/JSON
  file store). Pure-Go driver (`modernc.org/sqlite`) so the build stays
  CGO-free — that is what makes GoReleaser multi-platform builds
  trivial, so the decision is load-bearing. Plain `database/sql`,
  portable SQL where it's free, no dialect abstraction until the cloud
  backend actually needs one. `.gloss/` is gitignored by default. The
  git-committable appeal of a text store is captured as a future
  direction instead: `gloss export` / `gloss import` of sessions as
  clean markdown/JSONL — a deliberate portable artifact rather than the
  database doubling as a file format. Not scoped in this delivery.
- **MCP transport: streamable HTTP** served by the already-running
  binary (single owner of the SQLite handle). The docs ship a Claude
  Code config snippet. A stdio mode is deferred — it would mean a second
  process contending for the store.
- **Server-rendered everywhere.** Stdlib `html/template`, HTMX +
  Alpine.js vendored locally (no CDN, ever), Tailwind compiled via the
  standalone CLI binary in the build step — toolchain stays Go plus one
  binary, no Node.
- **Boring dependencies:** chroma for server-side syntax highlighting,
  goldmark for markdown, fsnotify for file watching.
- **Repo layout:** `cmd/gloss`,
  `internal/{server,store,plugins,connector,delta}` (plus
  `internal/proxy` reserved for the cloud phase).

## Agentic-First Setup

Cross-cutting — this is how the repo is built, not a milestone.

- **CLAUDE.md from day one.** Updated in the same PR whenever a
  milestone changes how the repo works — context drift is treated as a
  bug.
- **Makefile as the single entry point.** `make dev / test / lint / e2e
  / build`. CI and agents use only these targets.
- **Skills and commands are extracted when a recurring workflow
  crystallises, not speculatively:** a project `verify` skill once the
  app is runnable (milestone 4+); a `/new-plugin` command when plugin #2
  proves the pattern (milestone 8); a `/release` command when GoReleaser
  lands (milestone 10).
- **Dogfooding from milestone 6:** Gloss reviews its own AI-generated
  output. The repo's `.claude/` config registers the local Gloss MCP
  server, and design docs get reviewed through Gloss threads instead of
  PR comments. This doubles as continuous integration-testing of the MCP
  surface, and produces real anchor-drift cases that milestone 7 tests
  against.
- **Shared fixture corpus.** `testdata/` holds a representative file
  corpus (code, markdown, json, csv, images, media) used by unit tests,
  Playwright e2e, and agent verify runs alike.

## Testing & CI Strategy

Per PRINCIPLES.md (business repo) — no test suite, no merge.

- **Unit tests** for business logic. Delta tracking / anchor remap gets
  the heaviest coverage (property/table tests) — it's the hardest
  correctness problem in the product.
- **Integration tests** at every boundary: store (against real SQLite),
  connectors (against fixture repos), MCP (driven by a real MCP client).
- **Playwright e2e** for the web UI, from the first milestone that has
  one.
- **CI from the first commit** (GitHub Actions): markdownlint + yamllint
  baseline, plus Go build, `go test`, golangci-lint, gofmt check. The
  Playwright job joins at milestone 4; a GoReleaser snapshot build joins
  at milestone 10 so cross-compilation breakage is caught pre-tag.

## Milestones

Eleven parts, each an independently mergeable slice with tests and CI
green at exit.

### 1. Bootstrap & agentic scaffolding — complete

Public repo, Go module, `cmd/gloss` walking skeleton (`--version`,
mode-flag surface reserved), Makefile, CI baseline (lint + build +
test), CLAUDE.md filled in, `.claude/` directory established.

*Exit: CI green on a hello-world binary.*

### 2. Domain model & SQLite store — complete

Schema: sessions, threads, comments, polymorphic anchors,
file_snapshots — the local subset only, no users/orgs — as specified
in [docs/data-model.md](docs/data-model.md). Embedded migrations.
`internal/store` with full CRUD and integration tests against real
SQLite.

*Exit: store passes integration tests; `gloss .` creates
`.gloss/gloss.db`.*

### 3. Connectors — local directory + git — complete

Tree walking with ignore rules (`.gitignore` + `.glossignore`), content
hashing, snapshot capture, git metadata (commit SHA as anchor fallback).

*Exit: fixture-repo integration tests pass.*

### 4. Web server shell + plugin interface

`gloss .` boots the HTTP server (port pick, browser open). HTMX /
Tailwind / Alpine asset pipeline, all vendored. File-browser UI. Plugin
interface + registry, with **plaintext** (the catch-all) as the first
implementation. Playwright harness starts here and runs in CI.

*Exit: browse and read any file in a directory via localhost.*

### 5. Annotation core — complete

The heart of the product. Session CRUD + switcher, thread creation from
line selection (Alpine selection/highlight), comment composer, nested
replies, resolve/reopen, thread list + filters.

*Exit: full human review loop works end-to-end under Playwright.*

### 6. MCP server

Streamable HTTP endpoint on the running server. Full tool surface
(sessions / threads / comments / repos) as specified in
[docs/mcp-api.md](docs/mcp-api.md), with `author_type` /
`author_agent` attribution. Integration tests driven by a real MCP
client. **Dogfooding begins here.**

*Exit: Claude Code can read threads and reply into the UI live.*

### 7. Delta tracking & anchor remap

Context-line capture, fuzzy re-match on file change, orphaned-thread
state and its UI treatment, fsnotify live re-anchoring, git-SHA
fallback. Heaviest unit-test milestone.

*Exit: edit a file mid-review; threads follow or orphan visibly — never
vanish.*

### 8. Text-format plugins

Code (chroma highlighting), markdown (goldmark, rendered + source
views), json, csv (cell anchors). The `/new-plugin` command is extracted
here, once the second plugin proves the pattern.

*Exit: fixture corpus renders and annotates correctly per format.*

### 9. Media plugins

Image first and fully — XY-region anchors with an Alpine region-draw
UI — since it proves the non-line anchor path. Audio (time), video
(region+time), and pdf are trailing scope within the milestone, reusing
what image established.

*Exit: region/time anchors round-trip through UI, store, and MCP.*

### 10. Distribution

GoReleaser: darwin/linux/windows, arm64 + amd64. Tag-driven release
workflow, Homebrew tap, curl install script, version embedding,
CHANGELOG.

*Exit: `brew install` → `gloss .` works on a clean machine.*

### 11. Website & launch polish

Static docs site in-repo (`site/`, hand-written HTML + the same Tailwind
build), GitHub Pages deploy workflow. Landing, install, quickstart, MCP
setup guide, plugin reference, screenshots/demo GIF. README brought up
to launch quality. The full marketing site remains a separate later
concern.

*Exit: a public URL a stranger can install from.*

## Sequencing Rationale

- Store → connector → UI → MCP is straight dependency order.
- Delta tracking lands *after* MCP because dogfooding produces real
  anchor-drift cases to test the remapper against.
- Distribution lands *before* the website because install docs need real
  release artifacts to point at.

## Status Log

- 2026-07-07: Initial plan. Scope questions settled: ~10 mergeable
  milestones (landed on 11); website = OSS docs site in-repo via GitHub
  Pages, not the marketing site; proxy mode out of scope; SQLite over a
  text/JSON file store, with committable export/import noted as a future
  direction.
- 2026-07-08: Plan committed to the repo. Milestone 1 started: license
  settled (MIT), CLAUDE.md checklist walked and filled; walking
  skeleton, Makefile, and CI baseline under way.
- 2026-07-08: Design reference docs added — the data model
  (local subset) and MCP tool surface now live in
  [docs/data-model.md](docs/data-model.md) and
  [docs/mcp-api.md](docs/mcp-api.md), so milestones 2 and 6 can be
  implemented from this repo alone.
- 2026-07-08: Milestone 1 complete — walking skeleton, Makefile, and CI
  baseline merged.
- 2026-07-08: Milestone 2 complete — `internal/store` lands the SQLite
  schema (embedded migrations, applied on open), text-UUID keys, the
  polymorphic anchor as a Go sum type persisted as opaque JSON, full
  CRUD sized for the milestone 5–6 surface (including the `list_threads`
  filters, which match author on a thread's root comment), and
  integration tests against real SQLite. `gloss .` now initialises
  `.gloss/gloss.db`. Timestamps (`created_at`/`updated_at`) were added
  to repository/session/thread in docs/data-model.md — the only schema
  deviation, needed for UI ordering.
- 2026-07-09: Milestone 3 complete — `internal/connector` walks a
  repository, hashes tracked files, and creates/reuses `FileSnapshot`
  rows via the milestone-2 store. Local and git connectors share one
  walk+hash+persist core and differ only in which ignore files apply
  (`.glossignore` only vs `.gitignore` + `.glossignore`) and whether a
  commit SHA is attached. Deliberate scope calls: a hand-rolled
  gitignore-subset matcher instead of a dependency (comments, negation,
  dir-only, root anchoring, `*`/`**`/`?`; no character classes, no
  nested per-directory ignore files — root-level only); git metadata
  via shelling out to `git rev-parse HEAD` rather than a git-in-Go
  library, keeping the build CGO-free. `gloss .` now snapshots tracked
  files on every startup, reporting indexed/new/reused/skipped counts.
  Integration tests run against a fixture corpus in
  `testdata/connector/fixture/` (copied per-test, with a real `git init`
  and commit for git-connector cases).
- 2026-07-11: Milestone 4 complete — `gloss .` boots a real HTTP server
  (`internal/server`): fixed default port `4747` (override with `-port`,
  `-port 0` for an OS-assigned port), best-effort cross-platform browser
  open (`-no-open` to skip), graceful shutdown on Ctrl+C via
  `context.Context`. Routing is stdlib `net/http.ServeMux` only. The
  file-browser UI is server-rendered `html/template` + HTMX + Alpine.js
  (vendored from npm tarballs into `internal/server/static/vendor/` —
  `unpkg`/`jsdelivr`/raw GitHub weren't reachable, the npm registry was);
  the tree and file-content requests share one path-safety check: a file
  is only ever read from disk if `connector.ListFiles` (new, exported —
  reuses the milestone-3 ignore-matching walk) lists it, which is both
  the ignore-rule boundary and the traversal defense. `internal/plugins`
  introduces the plugin interface — `Render(File) ([]View, error)`,
  where `View{Name, HTML}` is a slice rather than a single string so
  milestone 8's markdown "rendered + source" tabs don't force a breaking
  change — with `plaintext` as the catch-all (HTML-escaped, with binary-
  content detection). Tailwind compiles via the standalone CLI
  (`make assets`, new Makefile target, prerequisite of `build`/`test`/
  `lint`); the compiled `app.css` is committed as a fallback so
  `go build`/`go test` never require network access — `make assets`
  overwrites it when the CLI can be downloaded, and degrades to a no-op
  otherwise (GitHub Releases wasn't reachable from this session's sandbox
  network; CI and real dev machines are unaffected). `make e2e` now runs
  Playwright (`@playwright/test`, scoped to `e2e/`'s own `package.json`)
  against the milestone-3 fixture corpus, joining CI as a new `e2e` job.
- 2026-07-12: Milestone 5 complete — session CRUD, thread creation from a
  line-range selection, nested comment replies, resolve/reopen, and a
  thread list with an `anchor_status` filter, all server-rendered.
  `internal/plugins`'s plaintext render changed from one escaped
  `<pre><code>` blob to one `<div data-line="N">` per line — a
  documented convention any line-anchoring plugin follows (the `Plugin`
  interface itself is unchanged), so the UI can select/highlight ranges
  generically without knowing which plugin rendered a file. Routing adds
  a session-scoped tree (`/s/{sessionID}`, `/s/{sessionID}/files/{path...}`)
  alongside milestone 4's plain `/`/`/files/{path...}` routes, which stay
  exactly as they were — no session, no thread UI — so existing behavior
  and tests are untouched; a session switcher in the nav (visible on both)
  is how you get from one to the other. Every mutation (create/rename/
  delete session, create thread, add/reply comment, resolve/reopen)
  follows validate-then-redirect-back — `HX-Redirect` for HTMX requests,
  a real 303 otherwise — rather than partial swaps, so each `GET` handler
  stays the single rendering source of truth and the forms degrade to
  plain HTML POSTs. Line selection and gutter highlighting of annotated
  lines are driven by a small hand-written Alpine helper
  (`internal/server/static/js/gloss.js`, same status as `app.css`: not
  vendored, same-origin, no build step) operating generically on
  `[data-line]` elements via one delegated click handler. Author identity
  for human writes comes from a new `-author` flag on `gloss`, defaulting
  to the local OS username (`os/user`); local mode still has no accounts,
  so this is a plain configured value, not a foreign key. Delta-tracking
  fields on `LineAnchor` (`context_before`/`context_after`) are left
  empty at creation time — milestone 7 owns context-line capture.
