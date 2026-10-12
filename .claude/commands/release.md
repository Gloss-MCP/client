---
description: Walk through cutting a release (changelog, rc, tag, publish)
argument-hint: <version, e.g. 0.1.0>
---

# Release $ARGUMENTS

Follow [docs/releasing.md](../../docs/releasing.md). It is the source of
truth; this is the order of play.

1. Confirm master is green and the working tree is clean.
2. Check `CHANGELOG.md`: move `[Unreleased]` entries into a
   `## [$ARGUMENTS] - YYYY-MM-DD` section, and run
   `make release-notes VERSION_TAG=v$ARGUMENTS` to see the notes as they
   will be published. Open a PR for the changelog; do not tag until merged.
3. Run `make release-snapshot` and spot-check `dist/` (archives, checksums,
   `./dist/.../gloss -version`).
4. Tag a release candidate (`v$ARGUMENTS-rc1`), push it, wait for the
   Release workflow, and check the draft. Tagging and publishing are the
   user's call: confirm before pushing any tag.
5. Repeat with `v$ARGUMENTS`, publish the draft (user), then confirm the
   `tap` job and `brew install gloss-mcp/tap/gloss`.
