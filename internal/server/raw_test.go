package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gloss-mcp/client/internal/connector"
	"github.com/gloss-mcp/client/internal/plugins"
)

// rawServer is testServer plus media files, started as an HTTP server.
func rawServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	srv, root := testServer(t)
	for rel, content := range map[string][]byte{
		"pics/shot.png":  []byte("0123456789"),
		"pics/logo.SVG":  []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"clip.mp4":       []byte("abcdefghijklmnopqrstuvwxyz"),
		"ignored.local":  []byte("shh"),
		"pics/notes.png": []byte("png-ish"),
	} {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, root
}

func get(t *testing.T, url string, hdr map[string]string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func TestRawServesMediaWithLockedDownHeaders(t *testing.T) {
	ts, _ := rawServer(t)
	resp, body := get(t, ts.URL+"/raw/pics/shot.png", nil)
	if resp.StatusCode != 200 || body != "0123456789" {
		t.Fatalf("status %d body %q", resp.StatusCode, body)
	}
	for k, want := range map[string]string{
		"Content-Type":           "image/png",
		"X-Content-Type-Options": "nosniff",
	} {
		if got := resp.Header.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if csp := resp.Header.Get("Content-Security-Policy"); csp == "" || csp[:7] != "sandbox" {
		t.Errorf("CSP = %q, want a sandbox policy", csp)
	}
}

func TestRawSVGIsServedAsImageNotHTML(t *testing.T) {
	ts, _ := rawServer(t)
	resp, _ := get(t, ts.URL+"/raw/pics/logo.SVG", nil) // extension match is case-insensitive
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/svg+xml" {
		t.Errorf("status %d type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
}

func TestRawSupportsRangeRequests(t *testing.T) {
	ts, _ := rawServer(t)
	resp, body := get(t, ts.URL+"/raw/clip.mp4", map[string]string{"Range": "bytes=2-5"})
	if resp.StatusCode != http.StatusPartialContent || body != "cdef" {
		t.Fatalf("status %d body %q, want 206 cdef", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Range"); got != "bytes 2-5/26" {
		t.Errorf("Content-Range = %q", got)
	}
}

func TestRawRejectsNonMediaUntrackedIgnoredAndTraversal(t *testing.T) {
	ts, _ := rawServer(t)
	for name, path := range map[string]string{
		"text file":      "/raw/README.md",
		"untracked":      "/raw/pics/missing.png",
		"ignored":        "/raw/ignored.local",
		"traversal":      "/raw/../../../../etc/passwd",
		"encoded dotdot": "/raw/pics/..%2f..%2f..%2fetc%2fpasswd",
		"directory":      "/raw/pics",
	} {
		t.Run(name, func(t *testing.T) {
			resp, body := get(t, ts.URL+path, nil)
			if resp.StatusCode == 200 {
				t.Errorf("status 200, body %q; want a refusal", body)
			}
		})
	}
}

func TestRawRefusesOversizedFiles(t *testing.T) {
	ts, root := rawServer(t)
	big := filepath.Join(root, "big.png")
	f, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(plugins.MaxMediaBytes + 1); err != nil { // sparse
		t.Fatal(err)
	}
	_ = f.Close()
	// Sanity: the file is in the tracked set, so only the cap refuses it.
	paths, _ := connector.ListFiles(root, "local")
	if !containsPath(paths, "big.png") {
		t.Fatal("big.png not tracked")
	}
	resp, _ := get(t, ts.URL+"/raw/big.png", nil)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", resp.StatusCode)
	}
}
