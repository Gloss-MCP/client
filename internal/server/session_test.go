package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCreateSessionRedirectsIntoNewSession(t *testing.T) {
	srv, _ := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.PostForm(ts.URL+"/sessions", url.Values{"name": {"My Review"}})
	if err != nil {
		t.Fatalf("POST /sessions: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (after following the redirect)", resp.StatusCode)
	}
	if !strings.HasPrefix(resp.Request.URL.Path, "/s/") {
		t.Fatalf("final URL = %s, want a redirect into /s/{id}", resp.Request.URL.Path)
	}
	body := readBody(t, resp)
	if !strings.Contains(body, "My Review") {
		t.Errorf("body missing new session name: %s", body)
	}
}

func TestCreateSessionRequiresName(t *testing.T) {
	srv, _ := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.PostForm(ts.URL+"/sessions", url.Values{"name": {"  "}})
	if err != nil {
		t.Fatalf("POST /sessions: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a blank name", resp.StatusCode)
	}
}

func TestUpdateSessionRenamesAndChangesStatus(t *testing.T) {
	srv, _ := testServer(t)
	ctx := context.Background()
	sess, err := srv.cfg.Store.CreateSession(ctx, srv.cfg.RepoID, "old name", "", testAuthor)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.PostForm(ts.URL+"/sessions/"+sess.ID, url.Values{
		"name": {"new name"}, "status": {"archived"},
	})
	if err != nil {
		t.Fatalf("POST /sessions/%s: %v", sess.ID, err)
	}
	defer func() { _ = resp.Body.Close() }()

	got, err := srv.cfg.Store.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "new name" {
		t.Errorf("Name = %q, want %q", got.Name, "new name")
	}
	if got.Status != "archived" {
		t.Errorf("Status = %q, want archived", got.Status)
	}
}

func TestDeleteSessionRemovesItAndItsThreads(t *testing.T) {
	srv, root := testServer(t)
	ctx := context.Background()
	sess, err := srv.cfg.Store.CreateSession(ctx, srv.cfg.RepoID, "doomed", "", testAuthor)
	if err != nil {
		t.Fatal(err)
	}
	createTestThread(t, srv, root, sess.ID, "README.md", 1, 1, "will be deleted")

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.PostForm(ts.URL+"/sessions/"+sess.ID+"/delete", nil)
	if err != nil {
		t.Fatalf("POST delete: %v", err)
	}
	_ = resp.Body.Close()

	if _, err := srv.cfg.Store.GetSession(ctx, sess.ID); err == nil {
		t.Error("session still exists after delete")
	}

	getResp, err := http.Get(ts.URL + "/s/" + sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = getResp.Body.Close() }()
	if getResp.StatusCode != http.StatusNotFound {
		t.Errorf("GET deleted session status = %d, want 404", getResp.StatusCode)
	}
}

func TestSessionSwitcherListsSessionsOnPlainBrowse(t *testing.T) {
	srv, _ := testServer(t)
	ctx := context.Background()
	if _, err := srv.cfg.Store.CreateSession(ctx, srv.cfg.RepoID, "session one", "", testAuthor); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.cfg.Store.CreateSession(ctx, srv.cfg.RepoID, "session two", "", testAuthor); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body := readBody(t, resp)
	if !strings.Contains(body, "session one") || !strings.Contains(body, "session two") {
		t.Errorf("plain browse page missing session switcher entries: %s", body)
	}
}
