package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gloss-mcp/client/internal/store"
)

func TestCreateThreadAnchorsToLineRangeAndShowsInFileView(t *testing.T) {
	srv, _ := testServer(t)
	ctx := context.Background()
	sess, err := srv.cfg.Store.CreateSession(ctx, srv.cfg.RepoID, "review", "", testAuthor)
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	fileURL := ts.URL + "/s/" + sess.ID + "/files/README.md"
	resp, err := http.PostForm(ts.URL+"/s/"+sess.ID+"/threads", url.Values{
		"file_path": {"README.md"}, "start_line": {"1"}, "end_line": {"1"},
		"body": {"what does this mean?"}, "return_to": {fileURL},
	})
	if err != nil {
		t.Fatalf("POST threads: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 after following the redirect back to the file view", resp.StatusCode)
	}

	body := readBody(t, resp)
	if !strings.Contains(body, "what does this mean?") {
		t.Errorf("file view missing the new thread's comment: %s", body)
	}
	if !strings.Contains(body, `data-line="1"`) {
		t.Errorf("file view missing the annotated line's data-line attribute: %s", body)
	}

	threads, err := srv.cfg.Store.ListThreads(ctx, sess.ID, store.ListThreadsFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 1 {
		t.Fatalf("len(threads) = %d, want 1", len(threads))
	}
	if threads[0].CreatedBy != testAuthor {
		t.Errorf("CreatedBy = %q, want %q", threads[0].CreatedBy, testAuthor)
	}
}

func TestCreateThreadRejectsInvalidLineRange(t *testing.T) {
	srv, _ := testServer(t)
	ctx := context.Background()
	sess, err := srv.cfg.Store.CreateSession(ctx, srv.cfg.RepoID, "review", "", testAuthor)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	for name, form := range map[string]url.Values{
		"end before start":  {"file_path": {"README.md"}, "start_line": {"3"}, "end_line": {"1"}, "body": {"x"}},
		"empty body":        {"file_path": {"README.md"}, "start_line": {"1"}, "end_line": {"1"}, "body": {" "}},
		"missing file_path": {"start_line": {"1"}, "end_line": {"1"}, "body": {"x"}},
		"non-numeric line":  {"file_path": {"README.md"}, "start_line": {"a"}, "end_line": {"1"}, "body": {"x"}},
	} {
		t.Run(name, func(t *testing.T) {
			resp, err := http.PostForm(ts.URL+"/s/"+sess.ID+"/threads", form)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", resp.StatusCode)
			}
		})
	}
}

func TestResolveThenReopenThread(t *testing.T) {
	srv, root := testServer(t)
	ctx := context.Background()
	sess, err := srv.cfg.Store.CreateSession(ctx, srv.cfg.RepoID, "review", "", testAuthor)
	if err != nil {
		t.Fatal(err)
	}
	thread := createTestThread(t, srv, root, sess.ID, "README.md", 1, 1, "resolve me")

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resolveResp, err := http.PostForm(ts.URL+"/s/"+sess.ID+"/threads/"+thread.ID+"/resolve", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resolveResp.Body.Close()

	threadAfterResolve, _, err := srv.cfg.Store.GetThread(ctx, thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if threadAfterResolve.AnchorStatus != store.AnchorResolved {
		t.Errorf("AnchorStatus = %q, want resolved", threadAfterResolve.AnchorStatus)
	}

	reopenResp, err := http.PostForm(ts.URL+"/s/"+sess.ID+"/threads/"+thread.ID+"/reopen", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = reopenResp.Body.Close()

	threadAfterReopen, _, err := srv.cfg.Store.GetThread(ctx, thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if threadAfterReopen.AnchorStatus != store.AnchorActive {
		t.Errorf("AnchorStatus = %q, want active", threadAfterReopen.AnchorStatus)
	}
}

func TestListThreadsStatusFilter(t *testing.T) {
	srv, root := testServer(t)
	ctx := context.Background()
	sess, err := srv.cfg.Store.CreateSession(ctx, srv.cfg.RepoID, "review", "", testAuthor)
	if err != nil {
		t.Fatal(err)
	}
	createTestThread(t, srv, root, sess.ID, "README.md", 1, 1, "still active")
	resolved := createTestThread(t, srv, root, sess.ID, "README.md", 1, 1, "already resolved")
	if _, err := srv.cfg.Store.SetAnchorStatus(ctx, resolved.ID, store.AnchorResolved); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/s/" + sess.ID + "/files/README.md?status=active")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body := readBody(t, resp)
	if !strings.Contains(body, "still active") {
		t.Errorf("status=active view missing the active thread: %s", body)
	}
	if strings.Contains(body, "already resolved") {
		t.Errorf("status=active view must not show the resolved thread: %s", body)
	}
}

func TestThreadsIsolatedPerSession(t *testing.T) {
	srv, root := testServer(t)
	ctx := context.Background()
	sessA, err := srv.cfg.Store.CreateSession(ctx, srv.cfg.RepoID, "session A", "", testAuthor)
	if err != nil {
		t.Fatal(err)
	}
	sessB, err := srv.cfg.Store.CreateSession(ctx, srv.cfg.RepoID, "session B", "", testAuthor)
	if err != nil {
		t.Fatal(err)
	}
	createTestThread(t, srv, root, sessA.ID, "README.md", 1, 1, "only in session A")

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/s/" + sessB.ID + "/files/README.md")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body := readBody(t, resp)
	if strings.Contains(body, "only in session A") {
		t.Errorf("session B's file view leaked a thread from session A: %s", body)
	}
}

func TestSessionHomeShowsThreadsAcrossFiles(t *testing.T) {
	srv, root := testServer(t)
	ctx := context.Background()
	sess, err := srv.cfg.Store.CreateSession(ctx, srv.cfg.RepoID, "review", "", testAuthor)
	if err != nil {
		t.Fatal(err)
	}
	createTestThread(t, srv, root, sess.ID, "README.md", 1, 1, "about the readme")
	createTestThread(t, srv, root, sess.ID, "notes/todo.txt", 1, 1, "about the todo")

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/s/" + sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body := readBody(t, resp)
	for _, want := range []string{"about the readme", "README.md", "about the todo", "notes/todo.txt"} {
		if !strings.Contains(body, want) {
			t.Errorf("session home missing %q: %s", want, body)
		}
	}
}
