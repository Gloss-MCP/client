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
  binary itself is still Go plus one binary, no Node. Hand-written app
  JavaScript (Alpine component helpers too stateful for inline
  `x-data`) lives at `internal/server/static/js/` — same status as
  `app.css`: same-origin, no build step, not vendored because it isn't
  a third-party library.
- **MCP server runs at `/mcp` on the same port as the web UI (default
  4747).** Transport is streamable HTTP via
  `github.com/modelcontextprotocol/go-sdk` v1.6.1. The 13-tool surface
  (sessions, threads, comments, repos) lives in `internal/mcp/`. All MCP
  writes attribute `author_type=ai`; `author_agent` defaults to
  `clientInfo.name` from the MCP initialize handshake. `.mcp.json` at the
  repo root registers `http://localhost:4747/mcp` so Claude Code in this
  repo can read and reply through the UI live (dogfooding from milestone 6).
- **Dark mode follows `prefers-color-scheme`; there is no toggle.** In the
  app, the `@media (prefers-color-scheme: dark)` block at the end of
  `internal/server/static/css/input.css` redefines Tailwind's palette
  variables (gray scale inverted, white and the few accent colours
  re-tinted), so templates, plugin-generated HTML and classes toggled in
  `gloss.js` need no `dark:` variants — use gray/white utilities and the
  accents already remapped there. Hand-written `.gloss-*` CSS and the chroma
  theme (`github-dark`) get explicit overrides in the same block; a new
  `.gloss-*` rule with a hard-coded colour needs one too. The site (`site/`)
  is the opposite: explicit `dark:` variants, because its code blocks stay
  dark in both modes. Pages carry `<meta name="color-scheme">`.
- **Format plugins live in `internal/plugins/`** (code via chroma,
  markdown via goldmark, json, csv; plaintext is the catch-all).
  `NewDefaultRegistry()` order is precedence — specific formats before
  `Code`, whose lexers accept almost any extension — and format plugins
  fall back to plaintext above 1 MB. Line-addressable output follows the
  shared `[data-line]` convention (`lines.go`); multi-line elements add
  `data-line-end`, csv cells add `data-col`. Anchors: text formats use
  `line`, csv uses `cell` (row = the record's physical line span, column
  re-resolved by header text on remap). Adding a plugin: run
  `/new-plugin` (`.claude/commands/new-plugin.md`). Per-format fixtures
  live in `testdata/plugins/`, not `testdata/connector/fixture`, whose
  file list connector tests pin.
- **Media plugins (image, audio, video) are `plugins.Streamed`.** They
  render `<img>/<audio>/<video src="/raw/{path}">` instead of file bytes,
  so `Server.renderFile` stats rather than reads them. `GET /raw/` serves
  tracked files with an allowlisted media extension only
  (`plugins/media.go`), up to `plugins.MaxMediaBytes` (25 MB; above that
  the plugin shows a too-large note with no annotation surface), via
  `http.ServeContent` (Range for seeking) with `nosniff` and a sandbox CSP
  — SVG is image-only, never inlined. A view's `Surface` (`region`,
  `time`, `region_time`) picks the Alpine component in `session.html`:
  `glossRegionView`, `glossTimeView`, `glossVideoView` in `gloss.js`
  (`glossCodeView` knows nothing about media). Anchors are fractions 0–1
  and seconds, validated by `store.ValidateAnchor` at marshal time, so
  there is no migration (0002 already allows the types). Region/time
  threads orphan whenever the file's hash changes — there is no media
  remap. Alpine gotcha: in a click handler `$el` is the clicked element;
  use `$root` for the component. Native `<video controls>` swallow
  pointer down/up, hence the explicit "draw" mode. pdf is deferred (needs
  a page anchor + migration + renderer).
- **Schema changes are migrations in `internal/store/migrations/`.**
  Widening a CHECK means rebuilding the table; rebuild `threads` and
  `comments` together or dropping `threads` cascade-deletes every comment
  (see `0002_cell_anchor.sql` and its test).
- **Playwright e2e is the one sanctioned Node dependency, scoped to
  `e2e/`.** Its own `package.json`/`package-lock.json`, not part of the
  shipped binary's build. `make e2e` runs `npm ci` + `npx playwright
  test` there. The config is evaluated in every worker, so the fixture
  dir is a deterministic path created only by the main process. The UI
  does not live-refresh: specs that edit files on disk reload the page
  until the watcher's result appears. In cloud sandboxes set
  `PW_EXECUTABLE_PATH` to the pre-installed Chromium (see the config).
- **The docs site lives in `site/`.** Hand-written HTML pages (no
  templating, no JS), styled by `site/css/input.css` through the same
  standalone Tailwind CLI. `make site` compiles `site/css/site.css`,
  which is gitignored (unlike `app.css`) and built by
  `.github/workflows/pages.yml`, which deploys to GitHub Pages on pushes
  to `master` that touch `site/`. It is a project site under `/client/`,
  so every link is relative. `site/site_test.go` checks titles,
  descriptions and that relative links resolve. Screenshots in
  `site/img/` are captured by hand from a real run. Update the install
  and plugin pages when milestones 9/10 change them.
- **Releases are tag-driven and drafted.** A `v*` tag runs
  `.github/workflows/release.yml`: the full CI gate (`ci.yml` is reusable),
  then GoReleaser (`.goreleaser.yaml`: CGO-free, linux/darwin/windows ×
  amd64/arm64, `gloss_<version>_<os>_<arch>` archives, `checksums.txt`)
  uploads a **draft** GitHub release whose notes are the `CHANGELOG.md`
  section for the tag. A human publishes the draft; publishing a
  non-pre-release runs the `tap` job, which renders the Homebrew formula
  from the published `checksums.txt` (`scripts/render-formula.sh`) and
  pushes it to `gloss-mcp/homebrew-tap` (secret `HOMEBREW_TAP_TOKEN`).
  Pre-release tags (`v0.1.0-rc1`) skip the tap and are never "latest".
  Release builds embed the committed `app.css`, so they need no network.
  GoReleaser is pinned and checksum-verified in the Makefile and lands in
  gitignored `.goreleaser-bin/`. Steps and one-time setup:
  `docs/releasing.md`. macOS signing, SBOM and cosign are deferred.
- **Version embedding.** `main.version/commit/date` are set by ldflags
  (GoReleaser and `make build`); `cmd/gloss/version.go` formats
  `gloss 0.1.0 (commit abc1234, built <UTC>)`, strips a leading `v`, and
  falls back to `runtime/debug` build info for plain `go build`/`go install`.
- **CHANGELOG.md is hand-written (Keep a Changelog).** User-visible changes
  go under `[Unreleased]` in the same PR; the release fails if the tag has
  no section (`scripts/release-notes.sh`).
- **`site/install.sh` is the curl installer**, deployed with the Pages
  site. POSIX sh, verifies sha256 against `checksums.txt`, tested against a
  fake release server in `site/install_test.go`. `GLOSS_VERSION`,
  `GLOSS_INSTALL_DIR` and `GLOSS_BASE_URL` (test override) are its only
  knobs. Windows is a documented zip download, not scripted.
- **The Makefile is the single entry point.** `make run / test / lint /
  e2e / build / assets / site / release-check / release-snapshot` (plus
  `release*` helpers used by the release workflow). CI and agents use only
  these targets. Always
  invoke Makefile targets directly — never construct the underlying
  commands by hand.
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
