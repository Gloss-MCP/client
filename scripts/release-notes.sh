#!/bin/sh
# Print the CHANGELOG.md section for a release version.
#
#   scripts/release-notes.sh 0.1.0 [CHANGELOG.md]
#
# Matches the "## [0.1.0]" heading regardless of its date. A pre-release
# version (0.1.0-rc1) falls back to its base version's section, so release
# candidates can be rehearsed against the notes being written for the final
# release. Fails when no section exists or it is empty, so a tag cannot
# ship without release notes.
set -eu

version="${1:?usage: release-notes.sh VERSION [CHANGELOG.md]}"
version="${version#v}"
file="${2:-CHANGELOG.md}"

section() {
  awk -v want="## [$1]" '
    index($0, want) == 1 { on = 1; next }
    on && /^## / { exit }
    on && /^\[[^]]+\]: / { next }
    on { print }
  ' "$file"
}

notes="$(section "$version")"
if [ -z "$(printf '%s' "$notes" | tr -d '[:space:]')" ]; then
  base="${version%%-*}"
  if [ "$base" != "$version" ]; then
    notes="$(section "$base")"
  fi
fi

if [ -z "$(printf '%s' "$notes" | tr -d '[:space:]')" ]; then
  echo "release-notes: no CHANGELOG.md section for $version" >&2
  exit 1
fi
printf '%s\n' "$notes"
