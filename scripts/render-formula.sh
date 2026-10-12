#!/bin/sh
# Render the Homebrew formula for a published release.
#
#   scripts/render-formula.sh 0.1.0 checksums.txt > gloss.rb
#
# Archive URLs and sha256s come from the release's own checksums.txt, so
# the formula can only reference assets that were actually published.
set -eu

version="${1:?usage: render-formula.sh VERSION checksums.txt}"
version="${version#v}"
sums="${2:?usage: render-formula.sh VERSION checksums.txt}"
base="${GLOSS_RELEASE_URL:-https://github.com/gloss-mcp/client/releases/download}/v$version"

# sum OS ARCH prints the sha256 of the OS/ARCH archive, or fails.
sum() {
  name="gloss_${version}_$1_$2.tar.gz"
  s="$(awk -v f="$name" '$2 == f { print $1 }' "$sums")"
  [ -n "$s" ] || { echo "render-formula: $name not in $sums" >&2; exit 1; }
  printf '%s' "$s"
}

# Resolve every checksum up front: a failed assignment aborts under set -e,
# whereas a failure inside a printf argument would render an empty sha256.
darwin_arm64="$(sum darwin arm64)"
darwin_amd64="$(sum darwin amd64)"
linux_arm64="$(sum linux arm64)"
linux_amd64="$(sum linux amd64)"

stanza() {
  printf '    if Hardware::CPU.arm?\n'
  printf '      url "%s/gloss_%s_%s_arm64.tar.gz"\n' "$base" "$version" "$1"
  printf '      sha256 "%s"\n' "$2"
  printf '    else\n'
  printf '      url "%s/gloss_%s_%s_amd64.tar.gz"\n' "$base" "$version" "$1"
  printf '      sha256 "%s"\n' "$3"
  printf '    end\n'
}

darwin="$(stanza darwin "$darwin_arm64" "$darwin_amd64")"
linux="$(stanza linux "$linux_arm64" "$linux_amd64")"

cat <<FORMULA
class Gloss < Formula
  desc "Async annotation and review for AI-generated content"
  homepage "https://gloss-mcp.github.io/client/"
  version "$version"
  license "MIT"

  on_macos do
$darwin
  end

  on_linux do
$linux
  end

  def install
    bin.install "gloss"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/gloss -version")
  end
end
FORMULA
