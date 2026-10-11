package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gloss-mcp/client/internal/connector"
	"github.com/gloss-mcp/client/internal/plugins"
	"github.com/gloss-mcp/client/internal/store"
)

// corpusServer serves a copy of testdata/plugins through the default
// plugin registry -- the production wiring -- with a session ready.
func corpusServer(t *testing.T) (ts *httptest.Server, srv *Server, sessionID string) {
	t.Helper()
	root := t.TempDir()
	const corpus = "../../testdata/plugins"
	entries, err := os.ReadDir(corpus)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(corpus, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, e.Name()), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	st, err := store.Open(filepath.Join(t.TempDir(), "gloss.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	repo, err := st.CreateRepository(ctx, "corpus", store.ConnectorLocal, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connector.New(root, store.ConnectorLocal).Snapshot(ctx, st, repo.ID); err != nil {
		t.Fatal(err)
	}
	srv = New(Config{
		Root: root, RepoName: repo.Name, RepoID: repo.ID, ConnectorType: store.ConnectorLocal,
		Registry: plugins.NewDefaultRegistry(), Store: st, Author: testAuthor,
	})
	sess, err := st.CreateSession(ctx, repo.ID, "review", "", testAuthor)
	if err != nil {
		t.Fatal(err)
	}
	ts = httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, srv, sess.ID
}

func getBody(t *testing.T, u string) string {
	t.Helper()
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d", u, resp.StatusCode)
	}
	return readBody(t, resp)
}

func TestSessionFileViewShowsTabsForMultiViewPlugins(t *testing.T) {
	ts, _, sid := corpusServer(t)
	for file, tabs := range map[string][]string{
		"doc.md":      {"Rendered", "Source"},
		"config.json": {"Raw", "Formatted"},
		"prices.csv":  {"Table", "Source"},
	} {
		body := getBody(t, ts.URL+"/s/"+sid+"/files/"+file)
		for _, tab := range tabs {
			if !strings.Contains(body, ">"+tab+"</button>") {
				t.Errorf("%s: missing %q tab in session view", file, tab)
			}
		}
	}
	// Single-view plugins get no tab strip.
	if body := getBody(t, ts.URL+"/s/"+sid+"/files/code.go"); strings.Contains(body, "tab = 0") {
		t.Errorf("code.go should not render view tabs")
	}
}

func TestMarkdownBlockSelectionCreatesLineAnchorOverItsSpan(t *testing.T) {
	ts, srv, sid := corpusServer(t)
	// The intro paragraph spans source lines 3-4; the UI submits the
	// block's data-line / data-line-end.
	body := getBody(t, ts.URL+"/s/"+sid+"/files/doc.md")
	if !strings.Contains(body, `data-line="3" data-line-end="4"`) {
		t.Fatalf("rendered view lacks the paragraph's span: %s", body)
	}
	resp, err := http.PostForm(ts.URL+"/s/"+sid+"/threads", url.Values{
		"file_path": {"doc.md"}, "start_line": {"3"}, "end_line": {"4"}, "body": {"clarify?"},
		"return_to": {"/s/" + sid + "/files/doc.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	threads, err := srv.cfg.Store.ListThreads(context.Background(), sid, store.ListThreadsFilter{})
	if err != nil || len(threads) != 1 {
		t.Fatalf("threads=%d err=%v", len(threads), err)
	}
	la, ok := threads[0].Anchor.(store.LineAnchor)
	if !ok || la.StartLine != 3 || la.EndLine != 4 {
		t.Errorf("anchor = %#v, want line anchor 3-4", threads[0].Anchor)
	}

	// The thread highlights the block when the file is re-rendered.
	after := getBody(t, ts.URL+"/s/"+sid+"/files/doc.md")
	if !strings.Contains(after, "clarify?") || !strings.Contains(after, "line 3-4") {
		t.Errorf("thread panel missing after annotation: %s", after)
	}
}

func TestCSVCellSelectionCreatesCellAnchorOnMultiLineRecord(t *testing.T) {
	ts, srv, sid := corpusServer(t)
	// Record 2 (banana) spans lines 3-4 because of a quoted newline;
	// select its "notes" cell (column 4).
	resp, err := http.PostForm(ts.URL+"/s/"+sid+"/threads", url.Values{
		"file_path": {"prices.csv"}, "start_line": {"3"}, "end_line": {"4"}, "col": {"4"},
		"body": {"two lines?"}, "return_to": {"/s/" + sid},
	})
	if err != nil {
		t.Fatal(err)
	}
	page := readBody(t, resp)
	_ = resp.Body.Close()
	if !strings.Contains(page, "line 3-4, col 4 (notes)") {
		t.Errorf("session view missing cell label: %s", page)
	}

	threads, _ := srv.cfg.Store.ListThreads(context.Background(), sid, store.ListThreadsFilter{})
	ca, ok := threads[0].Anchor.(store.CellAnchor)
	if !ok || ca.StartLine != 3 || ca.EndLine != 4 || ca.Col != 4 || ca.Header != "notes" {
		t.Errorf("anchor = %#v", threads[0].Anchor)
	}
	if len(ca.ContextBefore) == 0 || len(ca.ContextAfter) == 0 {
		t.Errorf("context lines not captured: %+v", ca)
	}
}
