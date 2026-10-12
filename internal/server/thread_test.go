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

	"github.com/gloss-mcp/client/internal/plugins"
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

func TestCreateThreadWithColumnAnchorsToCell(t *testing.T) {
	srv, _ := testServer(t)
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(srv.cfg.Root, "prices.csv"), []byte("id,name,price\n1,apple,0.5\n2,pear,0.7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess, err := srv.cfg.Store.CreateSession(ctx, srv.cfg.RepoID, "review", "", testAuthor)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.PostForm(ts.URL+"/s/"+sess.ID+"/threads", url.Values{
		"file_path": {"prices.csv"}, "start_line": {"3"}, "end_line": {"3"}, "col": {"3"},
		"body": {"check this"}, "return_to": {"/s/" + sess.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := readBody(t, resp)
	_ = resp.Body.Close()
	if !strings.Contains(body, "line 3, col 3 (price)") {
		t.Errorf("session view missing cell label: %s", body)
	}

	threads, err := srv.cfg.Store.ListThreads(ctx, sess.ID, store.ListThreadsFilter{})
	if err != nil || len(threads) != 1 {
		t.Fatalf("threads = %d, err %v", len(threads), err)
	}
	ca, ok := threads[0].Anchor.(store.CellAnchor)
	if !ok || ca.Col != 3 || ca.Header != "price" || ca.StartLine != 3 {
		t.Errorf("anchor = %#v, want cell line 3 col 3 header price", threads[0].Anchor)
	}
}

func TestCreateThreadRejectsInvalidColumn(t *testing.T) {
	srv, _ := testServer(t)
	sess, err := srv.cfg.Store.CreateSession(context.Background(), srv.cfg.RepoID, "review", "", testAuthor)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	for _, col := range []string{"0", "-1", "x"} {
		resp, err := http.PostForm(ts.URL+"/s/"+sess.ID+"/threads", url.Values{
			"file_path": {"README.md"}, "start_line": {"1"}, "end_line": {"1"}, "col": {col}, "body": {"x"},
		})
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("col=%q: status = %d, want 400", col, resp.StatusCode)
		}
	}
}

func newImageSession(t *testing.T) (*Server, *httptest.Server, string) {
	t.Helper()
	srv, root := testServer(t)
	srv.cfg.Registry = plugins.NewDefaultRegistry()
	for name, content := range map[string]string{"shot.png": "png-bytes", "talk.mp3": "mp3-bytes", "demo.mp4": "mp4-bytes"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sess, err := srv.cfg.Store.CreateSession(context.Background(), srv.cfg.RepoID, "review", "", testAuthor)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return srv, ts, sess.ID
}

func TestCreateRegionThreadStoresRegionAnchor(t *testing.T) {
	srv, ts, sessID := newImageSession(t)
	fileURL := ts.URL + "/s/" + sessID + "/files/shot.png"
	resp, err := http.PostForm(ts.URL+"/s/"+sessID+"/threads", url.Values{
		"file_path": {"shot.png"}, "x": {"0.1"}, "y": {"0.2"}, "width": {"0.3"}, "height": {"0.4"},
		"body": {"what is this blob?"}, "return_to": {fileURL},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 after redirect", resp.StatusCode)
	}
	page := readBody(t, resp)
	if !strings.Contains(page, "what is this blob?") || !strings.Contains(page, "region 10%,20% 30%×40%") {
		t.Errorf("page missing the thread's comment or region label:\n%s", page)
	}

	// The page uses the region component (not the line one) and ships the
	// thread as a mark for the overlay.
	for _, want := range []string{"glossRegionView(", `name="width"`, "&#34;n&#34;:1", "&#34;status&#34;:&#34;active&#34;"} {
		if !strings.Contains(page, want) {
			t.Errorf("page missing %q", want)
		}
	}
	if strings.Contains(page, "glossCodeView(") {
		t.Errorf("image page must not use the line-selection component")
	}

	threads, err := srv.cfg.Store.ListThreads(context.Background(), sessID, store.ListThreadsFilter{})
	if err != nil || len(threads) != 1 {
		t.Fatalf("threads = %v, err %v", threads, err)
	}
	want := store.RegionAnchor{X: 0.1, Y: 0.2, Width: 0.3, Height: 0.4}
	if threads[0].Anchor != want {
		t.Errorf("anchor = %#v, want %#v", threads[0].Anchor, want)
	}
}

func TestCreateRegionThreadRejectsBadInput(t *testing.T) {
	_, ts, sessID := newImageSession(t)
	valid := url.Values{"file_path": {"shot.png"}, "x": {"0.1"}, "y": {"0.1"}, "width": {"0.2"}, "height": {"0.2"}, "body": {"hi"}}
	with := func(k, v string) url.Values {
		c := url.Values{}
		for kk, vv := range valid {
			c[kk] = vv
		}
		c.Set(k, v)
		return c
	}
	for name, form := range map[string]url.Values{
		"overflow":       with("width", "0.95"),
		"negative":       with("x", "-0.1"),
		"zero size":      with("height", "0"),
		"not a number":   with("y", "abc"),
		"missing height": with("height", ""),
		"empty body":     with("body", "  "),
		"not an image":   with("file_path", "README.md"),
		"untracked":      with("file_path", "nope.png"),
		"traversal":      with("file_path", "../../etc/passwd.png"),
	} {
		t.Run(name, func(t *testing.T) {
			resp, err := http.PostForm(ts.URL+"/s/"+sessID+"/threads", form)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode < 400 {
				t.Errorf("status = %d, want a 4xx", resp.StatusCode)
			}
		})
	}
}

func TestLoadThreadViewsReturnsRegionMarks(t *testing.T) {
	srv, _, sessID := newImageSession(t)
	ctx := context.Background()
	snap, err := srv.cfg.Store.CreateFileSnapshot(ctx, srv.cfg.RepoID, "shot.png", "h", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []store.Anchor{
		store.RegionAnchor{X: 0.1, Y: 0.1, Width: 0.2, Height: 0.2},
		store.RegionAnchor{X: 0.5, Y: 0.5, Width: 0.25, Height: 0.25},
	} {
		if _, _, err := srv.cfg.Store.CreateThread(ctx, store.CreateThreadParams{
			SessionID: sessID, FileSnapshotID: snap.ID, Anchor: a, CreatedBy: "x", Body: "b", AuthorType: store.AuthorHuman,
		}); err != nil {
			t.Fatal(err)
		}
	}
	threads, _ := srv.cfg.Store.ListThreads(ctx, sessID, store.ListThreadsFilter{})
	_, lines, marks, err := srv.loadThreadViews(ctx, sessID, threads, "/", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 0 || len(marks) != 2 {
		t.Fatalf("lines %v marks %v, want no lines and 2 marks", lines, marks)
	}
	if marks[0].N != 1 || marks[1].N != 2 || marks[1].X != 0.5 || marks[0].Status != "active" {
		t.Errorf("marks = %+v", marks)
	}
}

func TestCreateTimeThreadStoresTimeAnchor(t *testing.T) {
	srv, ts, sessID := newImageSession(t)
	resp, err := http.PostForm(ts.URL+"/s/"+sessID+"/threads", url.Values{
		"file_path": {"talk.mp3"}, "start_time": {"1.5"}, "end_time": {"4.25"},
		"body": {"cut this ramble"}, "return_to": {ts.URL + "/s/" + sessID + "/files/talk.mp3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	page := readBody(t, resp)
	for _, want := range []string{"cut this ramble", "1.5s-4.25s", "glossTimeView(", "data-time-surface", "&#34;type&#34;:&#34;time&#34;"} {
		if !strings.Contains(page, want) {
			t.Errorf("page missing %q", want)
		}
	}
	threads, _ := srv.cfg.Store.ListThreads(context.Background(), sessID, store.ListThreadsFilter{})
	if len(threads) != 1 || threads[0].Anchor != (store.TimeAnchor{StartTime: 1.5, EndTime: 4.25}) {
		t.Fatalf("threads = %+v", threads)
	}
}

func TestCreateMediaThreadRejectsMismatchedOrBadAnchors(t *testing.T) {
	_, ts, sessID := newImageSession(t)
	for name, form := range map[string]url.Values{
		"time on an image":     {"file_path": {"shot.png"}, "start_time": {"1"}, "end_time": {"2"}},
		"region on audio":      {"file_path": {"talk.mp3"}, "x": {"0.1"}, "y": {"0.1"}, "width": {"0.2"}, "height": {"0.2"}},
		"region_time on audio": {"file_path": {"talk.mp3"}, "x": {"0.1"}, "y": {"0.1"}, "width": {"0.2"}, "height": {"0.2"}, "start_time": {"1"}, "end_time": {"2"}},
		"time on video":        {"file_path": {"demo.mp4"}, "start_time": {"1"}, "end_time": {"2"}},
		"reversed":             {"file_path": {"talk.mp3"}, "start_time": {"5"}, "end_time": {"2"}},
		"negative":             {"file_path": {"talk.mp3"}, "start_time": {"-1"}, "end_time": {"2"}},
		"missing end":          {"file_path": {"talk.mp3"}, "start_time": {"1"}},
		"not a number":         {"file_path": {"talk.mp3"}, "start_time": {"abc"}, "end_time": {"2"}},
	} {
		t.Run(name, func(t *testing.T) {
			form.Set("body", "x")
			resp, err := http.PostForm(ts.URL+"/s/"+sessID+"/threads", form)
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

func TestCreateRegionTimeThreadStoresAnchorAndRendersVideoPane(t *testing.T) {
	srv, ts, sessID := newImageSession(t)
	resp, err := http.PostForm(ts.URL+"/s/"+sessID+"/threads", url.Values{
		"file_path": {"demo.mp4"}, "x": {"0.1"}, "y": {"0.2"}, "width": {"0.3"}, "height": {"0.4"},
		"start_time": {"2"}, "end_time": {"3.5"}, "body": {"logo flickers here"},
		"return_to": {ts.URL + "/s/" + sessID + "/files/demo.mp4"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	page := readBody(t, resp)
	for _, want := range []string{"logo flickers here", "glossVideoView(", "&#34;type&#34;:&#34;region_time&#34;", "region 10%,20% 30%×40% at 2s-3.5s"} {
		if !strings.Contains(page, want) {
			t.Errorf("page missing %q", want)
		}
	}
	threads, _ := srv.cfg.Store.ListThreads(context.Background(), sessID, store.ListThreadsFilter{})
	want := store.RegionTimeAnchor{X: 0.1, Y: 0.2, Width: 0.3, Height: 0.4, StartTime: 2, EndTime: 3.5}
	if len(threads) != 1 || threads[0].Anchor != want {
		t.Fatalf("threads = %+v", threads)
	}
}

func TestCreateThreadRejectsUntrackedPaths(t *testing.T) {
	srv, _ := testServer(t)
	ctx := context.Background()
	sess, err := srv.cfg.Store.CreateSession(ctx, srv.cfg.RepoID, "review", "", testAuthor)
	if err != nil {
		t.Fatal(err)
	}
	// A readable file outside the root, one ignored via .glossignore, and
	// one that simply is not there.
	outside := filepath.Join(filepath.Dir(srv.cfg.Root), "outside-secret.txt")
	t.Cleanup(func() { _ = os.Remove(outside) })
	if err := os.WriteFile(outside, []byte("top secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srv.cfg.Root, "ignored.txt"), []byte("hidden\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srv.cfg.Root, ".glossignore"), []byte("ignored.txt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	for name, path := range map[string]string{
		"traversal": "../outside-secret.txt",
		"absolute":  outside,
		"ignored":   "ignored.txt",
		"missing":   "nope.txt",
	} {
		t.Run(name, func(t *testing.T) {
			for _, extra := range []url.Values{{}, {"col": {"1"}}} {
				form := url.Values{"file_path": {path}, "start_line": {"1"}, "end_line": {"1"}, "body": {"x"}}
				for k, v := range extra {
					form[k] = v
				}
				resp, err := http.PostForm(ts.URL+"/s/"+sess.ID+"/threads", form)
				if err != nil {
					t.Fatal(err)
				}
				_ = resp.Body.Close()
				if resp.StatusCode != http.StatusNotFound {
					t.Errorf("status = %d, want 404", resp.StatusCode)
				}
			}
		})
	}
	threads, err := srv.cfg.Store.ListThreads(ctx, sess.ID, store.ListThreadsFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 0 {
		t.Errorf("created %d threads from rejected requests, want 0", len(threads))
	}
}
