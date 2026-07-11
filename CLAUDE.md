# CLAUDE.md

Instructions for AI assistants (Claude and others) working in this repository.

## Who This Is For

This repo belongs to Ben Rowe — a software engineer with 25 years of
experience across PHP, JavaScript, Go, and .NET. He prefers small business
environments and is building a portfolio of side businesses. Direct, no
fluff. Prefers honest pushback over validation.

## Project

Gloss is async annotation and review for AI-generated content. AI produces
documents, code, and data that are easy to review live in a session but have
no offline review experience — Gloss is that missing layer: read files in
your own time, comment on specific lines/regions, and feed those threads back
to the AI via MCP. This repo is the open-source client: a single Go binary
(`gloss .`) that boots a localhost web UI + MCP server backed by SQLite — no
auth, no account, no internet connection required.

- **Status**: implementing
- **Stack**: Go, HTMX + Alpine.js (vendored, no CDN), stdlib
  `html/template`, Tailwind via standalone CLI (no Node), SQLite via
  `modernc.org/sqlite` (CGO-free), chroma / goldmark / fsnotify, GoReleaser
  for distribution, Playwright for e2e
- **Repo structure**: one of five repos in the `gloss-mcp` org — `client`
  (this repo, public), `cloud`, `marketing`, `infrastructure`, `business`
  (all private). This repo covers local server mode and, later, proxy mode.
  Cross-repo docs — product design, delivery plan, process — live in the
  business repo.

## Technical Preferences

These are strong preferences, not suggestions. Do not propose alternatives
unless there is a compelling reason.

- **Language**: Go (primary), JavaScript where necessary
- **Frontend**: HTMX, server-rendered. No SPA frameworks.
- **Data**: Protobufs for contracts and serialisation
- **Infrastructure**: Infrastructure as code. No clicking in consoles.
- **Philosophy**: Keep it simple, minimal dependencies

Project-specific overrides and additions:

- **No protobufs yet — deliberate deviation.** Local-only scope has no
  cross-service boundary: MCP dictates JSON-RPC and SQLite needs no
  serialisation contract. The `proto/` submodule arrives with proxy-mode
  work. Do not reintroduce protobufs earlier.
- **CGO-free is load-bearing.** The pure-Go SQLite driver
  (`modernc.org/sqlite`) is what keeps GoReleaser cross-platform builds
  trivial. Never add a dependency that requires CGO.
- **No CDN, ever.** HTMX and Alpine.js are committed as real files under
  `internal/server/static/vendor/` (fetched once from their npm registry
  tarballs — `unpkg`/`jsdelivr`/raw GitHub weren't reachable from this
  environment's network, `registry.npmjs.org` was; see
  `internal/server/static/vendor/VENDORED.md` for exact versions/update
  steps). Tailwind compiles via the standalone CLI (`make assets`,
  downloaded from GitHub Releases into `.tailwindcli/`, gitignored) from
  `internal/server/static/css/input.css` into `app.css`, which **is**
  committed — a fallback so `go build`/`go test` never require network
  access; `make assets` (a prerequisite of `build`/`test`/`lint`)
  overwrites it with a real compile whenever the CLI can be downloaded,
  and silently no-ops otherwise. The toolchain for the shipped `gloss`
  binary itself is still Go plus one binary, no Node.
- **Playwright e2e is the one sanctioned Node dependency, scoped to
  `e2e/`.** Its own `package.json`/`package-lock.json`, not part of the
  shipped binary's build. `make e2e` runs `npm ci` + `npx playwright
  test` there.
- **The Makefile is the single entry point.** `make dev / test / lint /
  e2e / build / assets`. CI and agents use only these targets.
- **CLAUDE.md stays current.** When a milestone changes how the repo works,
  update CLAUDE.md in the same PR — context drift is a bug.
- **Testing per PRINCIPLES.md (business repo): no test suite, no merge.**
  Unit tests for business logic; integration tests at every storage,
  network, or process boundary.

## How to Work Here

- Execute work directly — don't over-explain or ask for permission on
  obvious steps
- Use granular commits — one logical change per commit, not batched work
- All merges via rebase only — no merge commits, no squash
- Be direct. Flag risks. Push back when something is wrong or a better
  approach exists — but then execute what's decided
- For non-trivial changes — new structural decisions, rewrites of existing
  meaning — discuss and agree before implementing. Small, clearly-scoped
  edits don't need this; use judgement.

## Working with GitHub PRs

Whenever you push a branch, open a PR for it (against `master`) right after
the push — don't wait to be asked.

The user leaves PR comments as feedback to think about, not instructions to
execute literally:

- Read the comment, the surrounding diff, and relevant context before
  deciding what it means.
- If the right response is a code change, make it.
- If the comment is ambiguous or raises a tradeoff, reply with clarifying
  questions — that's a complete response on its own.
- Either way, acknowledge the comment with a reply so the user knows it was
  seen (e.g. "done in `<commit>`" or the clarifying question itself).

When the user approves a PR, merge it — don't wait for a separate
instruction.
