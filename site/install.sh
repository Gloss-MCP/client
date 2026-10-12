#!/bin/sh
# Gloss installer: downloads a release archive, verifies it against the
# release's checksums.txt and installs the binary.
#
#   curl -fsSL https://gloss-mcp.github.io/client/install.sh | sh
#
# Environment:
#   GLOSS_VERSION      version to install, e.g. 0.1.0 or v0.1.0 (default: latest release)
#   GLOSS_INSTALL_DIR  where to put the binary (default: $HOME/.local/bin)
#   GLOSS_BASE_URL     releases URL to download from
#                      (default: https://github.com/gloss-mcp/client/releases)

set -eu

BASE_URL="${GLOSS_BASE_URL:-https://github.com/gloss-mcp/client/releases}"
INSTALL_DIR="${GLOSS_INSTALL_DIR:-${HOME:-}/.local/bin}"

say() { printf 'gloss-install: %s\n' "$*"; }
die() { printf 'gloss-install: error: %s\n' "$*" >&2; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || die "$1 is required but not installed"; }

[ -n "$INSTALL_DIR" ] && [ "$INSTALL_DIR" != "/.local/bin" ] || die "HOME is not set; set GLOSS_INSTALL_DIR"
need curl
need tar

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  MINGW* | MSYS* | CYGWIN* | Windows_NT)
    die "Windows is not supported by this script; download the .zip from $BASE_URL" ;;
  *) die "unsupported operating system: $(uname -s)" ;;
esac

case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac

if [ -n "${GLOSS_VERSION:-}" ]; then
  version="${GLOSS_VERSION#v}"
else
  # /releases/latest redirects to /releases/tag/<tag>; reading the redirect
  # avoids the GitHub API and its rate limit. Pre-releases are never
  # "latest", so a default install never picks a release candidate.
  final="$(curl -fsSL -o /dev/null -w '%{url_effective}' "$BASE_URL/latest")" ||
    die "could not resolve the latest release from $BASE_URL/latest"
  tag="${final##*/}"
  [ -n "$tag" ] && [ "$tag" != "latest" ] || die "could not determine the latest version from $final"
  version="${tag#v}"
fi

archive="gloss_${version}_${os}_${arch}.tar.gz"
download="$BASE_URL/download/v$version"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

say "installing gloss $version ($os/$arch)"
curl -fsSL -o "$tmp/$archive" "$download/$archive" || die "download failed: $download/$archive"
curl -fsSL -o "$tmp/checksums.txt" "$download/checksums.txt" || die "download failed: $download/checksums.txt"

want="$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")"
[ -n "$want" ] || die "$archive is not listed in checksums.txt"
if command -v sha256sum >/dev/null 2>&1; then
  got="$(sha256sum "$tmp/$archive" | awk '{ print $1 }')"
elif command -v shasum >/dev/null 2>&1; then
  got="$(shasum -a 256 "$tmp/$archive" | awk '{ print $1 }')"
else
  die "sha256sum or shasum is required to verify the download"
fi
[ "$got" = "$want" ] || die "checksum mismatch for $archive (expected $want, got $got)"

tar -xzf "$tmp/$archive" -C "$tmp" gloss || die "could not extract $archive"
mkdir -p "$INSTALL_DIR"
mv "$tmp/gloss" "$INSTALL_DIR/gloss"
chmod 755 "$INSTALL_DIR/gloss"

# Binaries fetched with curl are not quarantined, but clear the flag anyway
# in case the archive came through a browser or a proxy that sets it.
if [ "$os" = darwin ] && command -v xattr >/dev/null 2>&1; then
  xattr -d com.apple.quarantine "$INSTALL_DIR/gloss" >/dev/null 2>&1 || true
fi

say "installed $("$INSTALL_DIR/gloss" -version) to $INSTALL_DIR/gloss"
case ":${PATH:-}:" in
  *":$INSTALL_DIR:"*) ;;
  *) say "add $INSTALL_DIR to your PATH to run 'gloss' from anywhere" ;;
esac
