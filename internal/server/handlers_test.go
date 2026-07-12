package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gloss-mcp/client/internal/connector"
	"github.com/gloss-mcp/client/internal/plugins"
	"github.com/gloss-mcp/client/internal/store"
)

// testAuthor is the fixture human identity attributed to threads and
// comments created through these tests.
const testAuthor = "fixture-user"

func testServer(t *testing.T) (*Server, string) {
	t.Helper()
	root := t.TempDir()
	write := func(rel, content string) {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("README.md", "# hello\n")
	write("notes/todo.txt", "buy milk")
	write("secret.local", "shh")
	write(".glossignore", "*.local\n")

	st, err := store.Open(filepath.Join(t.TempDir(), "gloss.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	ctx := context.Background()
	repo, err := st.CreateRepository(ctx, "fixture", store.ConnectorLocal, "")
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	if _, err := connector.New(root, store.ConnectorLocal).Snapshot(ctx, st, repo.ID); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	srv := New(Config{
		Root:          root,
		RepoName:      repo.Name,
		RepoID:        repo.ID,
		ConnectorType: store.ConnectorLocal,
		Registry:      plugins.NewRegistry(plugins.NewPlaintext()),
		Store:         st,
		Author:        testAuthor,
	})
	return srv, root
}

func TestHandleBrowseIndexShowsTreeNoFile(t *testing.T) {
	srv, _ := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	body := readBody(t, resp)
	if !strings.Contains(body, "README.md") {
		t.Errorf("body missing README.md in tree: %s", body)
	}
	if !strings.Contains(body, "notes") {
		t.Errorf("body missing notes dir in tree: %s", body)
	}
	if strings.Contains(body, "secret.local") {
		t.Errorf("body must not list .glossignore'd secret.local: %s", body)
	}
	if !strings.Contains(body, "Select a file") {
		t.Errorf("body should show the empty-selection placeholder: %s", body)
	}
}

func TestHandleBrowseFileRendersContent(t *testing.T) {
	srv, _ := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/files/notes/todo.txt")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	body := readBody(t, resp)
	if !strings.Contains(body, "buy milk") {
		t.Errorf("body missing file content: %s", body)
	}
	if !strings.Contains(body, "notes/todo.txt") {
		t.Errorf("body missing file path header: %s", body)
	}
}

func TestHandleBrowseHTMXRequestReturnsPartialOnly(t *testing.T) {
	srv, _ := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/files/README.md", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("HX-Request", "true")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body := readBody(t, resp)
	if strings.Contains(body, "<html") {
		t.Errorf("HTMX partial response should not include the full page shell: %s", body)
	}
	if !strings.Contains(body, "hello") {
		t.Errorf("partial should contain the rendered file content: %s", body)
	}
}

func TestHandleBrowseUnknownFileNotFound(t *testing.T) {
	srv, _ := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/files/does/not/exist.txt")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestHandleBrowseIgnoredFileNotFound(t *testing.T) {
	srv, _ := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/files/secret.local")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (ignored files must not be servable)", resp.StatusCode)
	}
}

func TestHandleBrowsePathTraversalRejected(t *testing.T) {
	srv, _ := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/files/../../../../etc/passwd")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	// net/http's ServeMux cleans "..", so this either 404s directly or
	// redirects to the cleaned (still non-existent/ignored) path -- either
	// way root:x:0:0 must never appear in the body.
	body := readBody(t, resp)
	if strings.Contains(body, "root:") {
		t.Fatalf("path traversal served file content outside the repo root: %s", body)
	}
}

func TestHandleBrowseStaticAssetsServed(t *testing.T) {
	srv, _ := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/static/vendor/htmx.min.js")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body := readBody(t, resp)
	if !strings.Contains(body, "htmx") {
		t.Errorf("static asset body doesn't look like htmx: first 100 chars: %.100s", body)
	}
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}
