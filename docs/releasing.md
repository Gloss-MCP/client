# Releasing

How a Gloss release is cut. Releases are tag-driven: pushing a `v*` tag
runs the whole pipeline, and nothing reaches users until you publish the
draft it produces.

## What the pipeline does

1. **Gate.** [release.yml](../.github/workflows/release.yml) runs the same
   jobs as CI (lint, tests, Playwright e2e, GoReleaser snapshot). A red gate
   stops the release.
2. **Draft release.** GoReleaser (`make release`) builds Linux, macOS and
   Windows binaries for amd64 and arm64, packs them
   (`gloss_<version>_<os>_<arch>.tar.gz`, `.zip` on Windows), writes
   `checksums.txt` and uploads everything to a **draft** GitHub release. The
   notes are the matching section of [CHANGELOG.md](../CHANGELOG.md); the run
   fails if there is none. Tags with a suffix (`v0.1.0-rc1`) are marked
   pre-release.
3. **Publish (you).** Check the draft's assets, then publish it in the
   GitHub UI.
4. **Homebrew tap.** Publishing a non-pre-release triggers the `tap` job,
   which renders the formula from the *published* `checksums.txt` and pushes
   it to `gloss-mcp/homebrew-tap`. Pre-releases skip it, so brew users never
   see a release candidate. The install script's "latest" lookup also ignores
   pre-releases.

## One-time setup

These need repository or organisation admin rights and are done by hand:

- Create the public repository `gloss-mcp/homebrew-tap` with a default
  branch and a `Formula/` directory (an initial README commit is enough).
- Create a fine-grained personal access token with **Contents: read and
  write** on `gloss-mcp/homebrew-tap` only, and store it as the Actions
  secret `HOMEBREW_TAP_TOKEN` on `gloss-mcp/client`.
- Set the repository's Pages source to "GitHub Actions" (also needed for the
  docs site and `install.sh`).

## Cutting a release

1. On master with green CI, finalise the changelog: move entries from
   `[Unreleased]` into a `## [x.y.z] - YYYY-MM-DD` section. Check the notes
   locally with `make release-notes VERSION_TAG=vx.y.z`.
2. Rehearse with a release candidate first: tag `vx.y.z-rc1`
   (`git tag -a vx.y.z-rc1 -m "vx.y.z-rc1" && git push origin vx.y.z-rc1`).
   Check the draft, install from it with `GLOSS_VERSION=x.y.z-rc1`, then
   publish or delete it. The tap is untouched.
3. Tag `vx.y.z` the same way. Wait for the workflow, check the draft,
   publish it.
4. Confirm the `tap` job went green and that
   `brew install gloss-mcp/tap/gloss && gloss -version` works.

## Verifying locally

`make release-snapshot` builds every target and the archives into `dist/`
without tagging or publishing; CI runs it on every push. `make release-check`
validates `.goreleaser.yaml`. Both download a pinned GoReleaser into
`.goreleaser-bin/` and fail if they cannot.

## If something goes wrong

- **Gate or GoReleaser failed.** Fix on master, delete the tag
  (`git push --delete origin vx.y.z` and `git tag -d vx.y.z`), delete any
  partial draft, and tag again.
- **A published release is bad.** Mark it as a pre-release or delete it so
  the install script stops resolving it as latest, revert the tap commit, and
  release a fixed patch version. Do not reuse a version number: Homebrew and
  the install script verify against the checksums of whatever was
  published.
- **Tap job failed.** Fix the cause (usually the token), then re-run the job
  from the Actions tab; it is idempotent.

## Not done yet

- macOS binaries are not signed or notarised. Homebrew and the install
  script are unaffected; archives downloaded in a browser need
  `xattr -d com.apple.quarantine gloss`.
- No SBOM or cosign signatures. `checksums.txt` is the only integrity
  data, and the install script and formula verify against it.
- No Windows installer; Windows users download the `.zip`.
