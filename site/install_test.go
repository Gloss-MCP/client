package site

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeBinary stands in for the gloss binary inside the fake release.
const fakeBinary = "#!/bin/sh\necho \"gloss 9.9.9 (commit abcdef0, built test)\"\n"

// fakeRelease serves the layout of a GitHub releases page: a /latest
// redirect to /tag/<tag>, plus /download/<tag>/{archive,checksums.txt}
// for each version, built for the host's os/arch.
type fakeRelease struct {
	versions []string
	latest   string
	badSum   bool
	// noLatest mimics a repo with no published non-pre-release: GitHub
	// redirects /releases/latest to the releases list, not a tag page.
	noLatest bool
}

func (f fakeRelease) archiveName(version string) string {
	return fmt.Sprintf("gloss_%s_%s_%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)
}

func (f fakeRelease) archive(t *testing.T) []byte {
	t.Helper()
	var buf strings.Builder
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{"gloss": fakeBinary, "LICENSE": "MIT\n"} {
		hdr := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(body))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return []byte(buf.String())
}

func (f fakeRelease) handler(t *testing.T) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		if f.noLatest {
			http.Redirect(w, r, "/releases", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/releases/tag/v"+f.latest, http.StatusFound)
	})
	mux.HandleFunc("/releases", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("releases list"))
	})
	mux.HandleFunc("/releases/tag/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("release page"))
	})
	data := f.archive(t)
	sum := sha256.Sum256(data)
	hexSum := hex.EncodeToString(sum[:])
	if f.badSum {
		hexSum = strings.Repeat("0", 64)
	}
	for _, v := range f.versions {
		prefix := "/releases/download/v" + v + "/"
		mux.HandleFunc(prefix+f.archiveName(v), func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(data)
		})
		mux.HandleFunc(prefix+"checksums.txt", func(w http.ResponseWriter, r *http.Request) {
			// Another platform's line first, to prove the script picks its own.
			fmt.Fprintf(w, "%s  gloss_%s_plan9_mips.tar.gz\n%s  %s\n",
				strings.Repeat("1", 64), v, hexSum, f.archiveName(v))
		})
	}
	return mux
}

// runInstall runs install.sh against srv with extra env, returning the
// install dir, combined output and the run error.
func runInstall(t *testing.T, srv *httptest.Server, pathPrefix string, env ...string) (string, string, error) {
	t.Helper()
	for _, tool := range []string{"sh", "curl", "tar"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not available", tool)
		}
	}
	dir := filepath.Join(t.TempDir(), "bin")
	cmd := exec.Command("sh", "install.sh")
	cmd.Env = append(os.Environ(),
		"GLOSS_BASE_URL="+srv.URL+"/releases",
		"GLOSS_INSTALL_DIR="+dir,
		"GLOSS_VERSION=",
	)
	cmd.Env = append(cmd.Env, env...)
	if pathPrefix != "" {
		cmd.Env = append(cmd.Env, "PATH="+pathPrefix+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	out, err := cmd.CombinedOutput()
	return dir, string(out), err
}

func TestInstallScriptInstallsLatest(t *testing.T) {
	rel := fakeRelease{versions: []string{"0.1.0", "0.2.0"}, latest: "0.2.0"}
	srv := httptest.NewServer(rel.handler(t))
	defer srv.Close()

	dir, out, err := runInstall(t, srv, "")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "installing gloss 0.2.0") {
		t.Errorf("did not resolve latest to 0.2.0:\n%s", out)
	}
	got, err := exec.Command(filepath.Join(dir, "gloss")).Output()
	if err != nil {
		t.Fatalf("installed binary does not run: %v", err)
	}
	if !strings.Contains(string(got), "gloss 9.9.9") {
		t.Errorf("binary output = %q", got)
	}
	if !strings.Contains(out, "to your PATH") {
		t.Errorf("expected a PATH hint for an install dir not on PATH:\n%s", out)
	}
}

func TestInstallScriptPinnedVersion(t *testing.T) {
	rel := fakeRelease{versions: []string{"0.1.0", "0.2.0"}, latest: "0.2.0"}
	srv := httptest.NewServer(rel.handler(t))
	defer srv.Close()

	for _, v := range []string{"0.1.0", "v0.1.0"} {
		_, out, err := runInstall(t, srv, "", "GLOSS_VERSION="+v)
		if err != nil {
			t.Fatalf("GLOSS_VERSION=%s: %v\n%s", v, err, out)
		}
		if !strings.Contains(out, "installing gloss 0.1.0") {
			t.Errorf("GLOSS_VERSION=%s installed the wrong version:\n%s", v, out)
		}
	}
}

func TestInstallScriptRejectsBadChecksum(t *testing.T) {
	rel := fakeRelease{versions: []string{"0.1.0"}, latest: "0.1.0", badSum: true}
	srv := httptest.NewServer(rel.handler(t))
	defer srv.Close()

	dir, out, err := runInstall(t, srv, "")
	if err == nil {
		t.Fatalf("install succeeded with a bad checksum:\n%s", out)
	}
	if !strings.Contains(out, "checksum mismatch") {
		t.Errorf("missing checksum error:\n%s", out)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "gloss")); statErr == nil {
		t.Error("binary was installed despite the checksum mismatch")
	}
}

func TestInstallScriptNoPublishedRelease(t *testing.T) {
	rel := fakeRelease{versions: []string{"0.1.0-rc1"}, noLatest: true}
	srv := httptest.NewServer(rel.handler(t))
	defer srv.Close()

	dir, out, err := runInstall(t, srv, "")
	if err == nil {
		t.Fatalf("install succeeded with no published release:\n%s", out)
	}
	if !strings.Contains(out, "no release has been published") || strings.Contains(out, "installing gloss") {
		t.Errorf("want a clear no-release error before any download:\n%s", out)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "gloss")); statErr == nil {
		t.Error("binary was installed")
	}

	// A pinned pre-release still installs: it does not need "latest".
	if _, out, err := runInstall(t, srv, "", "GLOSS_VERSION=0.1.0-rc1"); err != nil {
		t.Errorf("pinned pre-release failed: %v\n%s", err, out)
	}
}

func TestInstallScriptMissingRelease(t *testing.T) {
	rel := fakeRelease{versions: []string{"0.1.0"}, latest: "0.1.0"}
	srv := httptest.NewServer(rel.handler(t))
	defer srv.Close()

	_, out, err := runInstall(t, srv, "", "GLOSS_VERSION=9.9.9")
	if err == nil || !strings.Contains(out, "download failed") {
		t.Errorf("want a download failure, got err=%v\n%s", err, out)
	}
}

func TestInstallScriptUnsupportedPlatform(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	for name, tt := range map[string]struct{ uname, want string }{
		"freebsd":    {"-s=FreeBSD", "unsupported operating system"},
		"windows":    {"-s=MINGW64_NT-10.0", "Windows is not supported"},
		"32-bit arm": {"-m=armv7l", "unsupported architecture"},
	} {
		t.Run(name, func(t *testing.T) {
			bin := t.TempDir()
			// A fake uname answering only the flag under test; the other
			// flag reports a supported value.
			script := "#!/bin/sh\ncase \"$1\" in\n-s) echo " + unameValue(tt.uname, "-s", "Linux") +
				" ;;\n-m) echo " + unameValue(tt.uname, "-m", "x86_64") + " ;;\nesac\n"
			if err := os.WriteFile(filepath.Join(bin, "uname"), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			_, out, err := runInstall(t, srv, bin)
			if err == nil || !strings.Contains(out, tt.want) {
				t.Errorf("want %q failure, got err=%v\n%s", tt.want, err, out)
			}
		})
	}
}

// unameValue returns the override in spec ("-s=Value") for flag, or def.
func unameValue(spec, flag, def string) string {
	if f, v, ok := strings.Cut(spec, "="); ok && f == flag {
		return v
	}
	return def
}
