---
description: Scaffold a new file-format plugin (renderer + anchors + tests + corpus)
argument-hint: <format name, e.g. "yaml"> [extensions]
---

# New plugin: $ARGUMENTS

Add a rendering plugin for **$ARGUMENTS** to `internal/plugins/`. This
checklist is extracted from how the code, json, markdown and csv plugins
were built; follow it rather than inventing new structure.

## 1. Decide the anchor

The plugin chooses the anchor variant (`docs/data-model.md#anchor`):

- Text addressed by line → `line`. Emit the shared markup (below). Nothing
  else to build: selection, highlighting, remap and MCP already work.
- A different position model → a new `store.Anchor` variant: type +
  JSON struct in `internal/store/anchor.go`, marshal/unmarshal cases, a
  migration widening the `threads.anchor_type` CHECK (rebuild `threads`
  **and** `comments` together — see `0002_cell_anchor.sql` for why),
  remap in `internal/delta`, parsing in `internal/mcp/threads.go`,
  `store.Label`, and docs. Only do this when `line` genuinely cannot
  express the position; csv's `cell` is the precedent.

## 2. Write the plugin

`internal/plugins/<name>.go`, implementing `Plugin`
(`Name`, `Accepts(path)`, `Render(File) ([]View, error)`):

- `Accepts` is by extension, case-insensitive (`path.Ext` +
  `strings.EqualFold`/`ToLower`).
- Reuse the shared helpers in `lines.go`: `renderLines` for the
  `[data-line]` markup, `splitLines`, `isBinary`. Reuse `Code` for any
  "Source"/"Raw" view (`NewCode().Render(f)`).
- **Line numbering must match plaintext**: no phantom line after a trailing
  newline, CRLF handled. `corpus_test.go` enforces this.
- Elements spanning several lines carry `data-line` (first) and
  `data-line-end` (last). Cell-level elements carry `data-col`
  (`gloss.js` turns a click on one into a cell selection).
- Multi-view plugins return named `View`s; the first is the default tab.
  The annotatable view comes first. Read-only views must emit no
  `data-line`.
- Binary content → fall through to plaintext's placeholder. Unparseable
  input → render the source view with a `gloss-note` explaining why.
  Never return an error for bad user content.
- Content is untrusted (often AI-generated): escape everything.
  `html/template` for markup (note it forbids dynamic tag names — branch
  instead); never `template.HTML` raw input.

## 3. Register it

Add `sizeCapped{New<Name>()}` to `NewDefaultRegistry()` in
`internal/plugins/default.go`. **Order is precedence**: specific formats
before `Code` (whose lexers accept almost any extension), `Plaintext`
last. Add the path to `TestDefaultRegistryPrecedence`.

## 4. Style

Plugin-specific CSS goes in `internal/server/static/css/input.css`
(no CDN, no new tooling), then `make assets` to regenerate the committed
`app.css`.

## 5. Tests (no tests, no merge)

- Unit tests beside the plugin: `Accepts`, each view's markup, escaping
  of hostile content, empty and binary input, line-span edge cases.
- Fixtures in `testdata/plugins/` (not `testdata/connector/fixture`,
  whose file list connector tests pin). Don't name fixtures `*.go` unless
  they are valid, gofmt-clean Go — `make lint` runs `gofmt -l .`.
  Add rows to `corpus_test.go`.
- If the plugin has a UI behaviour, a case in `e2e/tests/formats.spec.js`:
  annotate, edit the file on disk, assert the thread follows (or
  orphans visibly). Reload inside `expectAfterReload` — the UI does not
  live-refresh. Use `exact: true` on button names.

## 6. Finish

Run `make lint`, `make test`, `make e2e` (never the underlying commands
by hand). Update `CLAUDE.md` if dependencies or conventions changed and
`PLAN.md` if a milestone moved. One logical change per commit.
