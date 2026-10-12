// Package scripts tests the release helper scripts.
package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const changelog = `# Changelog

## [Unreleased]

- Not yet.

## [0.2.0] - 2026-11-01

- Second release.

## [0.1.0] - 2026-10-12

### Added

- First release.

[Unreleased]: https://example.com/compare
`

func run(t *testing.T, script string, args ...string) (string, error) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	out, err := exec.Command("sh", append([]string{script}, args...)...).CombinedOutput()
	return string(out), err
}

func writeFile(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReleaseNotes(t *testing.T) {
	cl := writeFile(t, "CHANGELOG.md", changelog)
	tests := []struct {
		version string
		want    string
		notWant string
		fail    bool
	}{
		{"0.1.0", "First release.", "Second release.", false},
		{"v0.2.0", "Second release.", "First release.", false},
		{"0.1.0-rc1", "First release.", "Second release.", false},
		{"0.3.0", "", "", true},
		{"0.3.0-rc1", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			out, err := run(t, "release-notes.sh", tt.version, cl)
			if (err != nil) != tt.fail {
				t.Fatalf("err = %v, want failure=%v\n%s", err, tt.fail, out)
			}
			if tt.want != "" && !strings.Contains(out, tt.want) {
				t.Errorf("output missing %q:\n%s", tt.want, out)
			}
			if tt.notWant != "" && strings.Contains(out, tt.notWant) {
				t.Errorf("output contains %q:\n%s", tt.notWant, out)
			}
			if strings.Contains(out, "[Unreleased]:") {
				t.Errorf("link reference leaked into notes:\n%s", out)
			}
		})
	}
}

func TestReleaseNotesEmptySectionFails(t *testing.T) {
	cl := writeFile(t, "CHANGELOG.md", "# Changelog\n\n## [0.1.0] - 2026-10-12\n\n## [0.0.1] - 2026-01-01\n\n- Old.\n")
	if out, err := run(t, "release-notes.sh", "0.1.0", cl); err == nil {
		t.Errorf("empty section succeeded:\n%s", out)
	}
}

func TestRenderFormula(t *testing.T) {
	var sums strings.Builder
	for i, target := range []string{"darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64"} {
		sums.WriteString(strings.Repeat(string(rune('a'+i)), 64) + "  gloss_0.1.0_" + target + ".tar.gz\n")
	}
	sums.WriteString(strings.Repeat("f", 64) + "  gloss_0.1.0_windows_amd64.zip\n")
	f := writeFile(t, "checksums.txt", sums.String())

	out, err := run(t, "render-formula.sh", "v0.1.0", f)
	if err != nil {
		t.Fatalf("render failed: %v\n%s", err, out)
	}
	for _, want := range []string{
		`version "0.1.0"`,
		`url "https://github.com/gloss-mcp/client/releases/download/v0.1.0/gloss_0.1.0_darwin_arm64.tar.gz"`,
		`sha256 "` + strings.Repeat("b", 64) + `"`,
		`url "https://github.com/gloss-mcp/client/releases/download/v0.1.0/gloss_0.1.0_linux_amd64.tar.gz"`,
		`sha256 "` + strings.Repeat("c", 64) + `"`,
		`bin.install "gloss"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("formula missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "windows") {
		t.Errorf("formula mentions windows:\n%s", out)
	}

	// A release missing a platform must not render a broken formula.
	partial := writeFile(t, "partial.txt", strings.Repeat("a", 64)+"  gloss_0.1.0_darwin_amd64.tar.gz\n")
	if out, err := run(t, "render-formula.sh", "0.1.0", partial); err == nil {
		t.Errorf("partial checksums succeeded:\n%s", out)
	}
}
