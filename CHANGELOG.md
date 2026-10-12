# Changelog

All notable changes to Gloss are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/). The section for a version becomes
its GitHub release notes, so write entries for people installing Gloss, not
for people reading the commit log.

## [Unreleased]

## [0.1.0] - unreleased

First public release: a single, CGO-free binary. Run `gloss .` in a
directory to get a localhost web UI and an MCP server backed by SQLite. No
account, no auth, no internet connection.

### Added

- Review UI: file browser, review sessions, threads anchored to a line
  range, replies, resolve and reopen.
- MCP server at `/mcp` on the same port (default 4747) with 13 tools for
  sessions, threads, comments and repositories, so an AI can read your
  threads and reply in context.
- Format plugins: syntax-highlighted code, markdown (rendered and source),
  JSON, CSV (cell anchors), plain text, images (region anchors), audio (time
  anchors) and video (region and time anchors).
- Anchor tracking: threads re-anchor when a file changes on disk and are
  marked orphaned, never deleted, when their text is gone.
- `.gitignore` and `.glossignore` support, and git metadata for tracked
  files.
- `-version` prints the version, commit and build date.
- Releases for Linux, macOS and Windows on amd64 and arm64, a Homebrew tap
  and an install script.

### Security

- Creating a line or cell thread now requires the file to be in the
  tracked set, so a crafted `file_path` can no longer read files outside the
  reviewed directory.

### Known limitations

- PDF annotation is not supported yet.
- CSV is comma-separated only, and a cell thread highlights its whole row.
- macOS binaries are not signed or notarised; see the install page.

[Unreleased]: https://github.com/gloss-mcp/client/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/gloss-mcp/client/releases/tag/v0.1.0
