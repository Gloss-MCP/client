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

### 1. Bootstrap & agentic scaffolding — in progress

Public repo, Go module, `cmd/gloss` walking skeleton (`--version`,
mode-flag surface reserved), Makefile, CI baseline (lint + build +
test), CLAUDE.md filled in, `.claude/` directory established.

*Exit: CI green on a hello-world binary.*

### 2. Domain model & SQLite store

Schema: sessions, threads, comments, polymorphic anchors,
file_snapshots — the local subset only, no users/orgs. Embedded
migrations. `internal/store` with full CRUD and integration tests
against real SQLite.

*Exit: store passes integration tests; `gloss .` creates
`.gloss/gloss.db`.*

### 3. Connectors — local directory + git

Tree walking with ignore rules (`.gitignore` + `.glossignore`), content
hashing, snapshot capture, git metadata (commit SHA as anchor fallback).

*Exit: fixture-repo integration tests pass.*

### 4. Web server shell + plugin interface

`gloss .` boots the HTTP server (port pick, browser open). HTMX /
Tailwind / Alpine asset pipeline, all vendored. File-browser UI. Plugin
interface + registry, with **plaintext** (the catch-all) as the first
implementation. Playwright harness starts here and runs in CI.

*Exit: browse and read any file in a directory via localhost.*

### 5. Annotation core

The heart of the product. Session CRUD + switcher, thread creation from
line selection (Alpine selection/highlight), comment composer, nested
replies, resolve/reopen, thread list + filters.

*Exit: full human review loop works end-to-end under Playwright.*

### 6. MCP server

Streamable HTTP endpoint on the running server. Full tool surface
(sessions / threads / comments / repos), with `author_type` /
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
