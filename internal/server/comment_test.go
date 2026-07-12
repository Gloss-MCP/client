package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAddCommentTopLevelReply(t *testing.T) {
	srv, root := testServer(t)
	ctx := context.Background()
	sess, err := srv.cfg.Store.CreateSession(ctx, srv.cfg.RepoID, "review", "", testAuthor)
	if err != nil {
		t.Fatal(err)
	}
	thread := createTestThread(t, srv, root, sess.ID, "README.md", 1, 1, "root comment")

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	fileURL := ts.URL + "/s/" + sess.ID + "/files/README.md"
	resp, err := http.PostForm(ts.URL+"/s/"+sess.ID+"/threads/"+thread.ID+"/comments", url.Values{
		"body": {"here is a reply"}, "return_to": {fileURL},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	body := readBody(t, resp)
	if !strings.Contains(body, "root comment") || !strings.Contains(body, "here is a reply") {
		t.Errorf("file view missing root comment or reply: %s", body)
	}

	comments, err := srv.cfg.Store.ListComments(ctx, thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 2 {
		t.Fatalf("len(comments) = %d, want 2 (root + reply)", len(comments))
	}
	reply := comments[1]
	if reply.AuthorType != "human" || reply.AuthorAgent != testAuthor {
		t.Errorf("reply attribution = (%q, %q), want (human, %q)", reply.AuthorType, reply.AuthorAgent, testAuthor)
	}
}

func TestAddNestedReply(t *testing.T) {
	srv, root := testServer(t)
	ctx := context.Background()
	sess, err := srv.cfg.Store.CreateSession(ctx, srv.cfg.RepoID, "review", "", testAuthor)
	if err != nil {
		t.Fatal(err)
	}
	thread := createTestThread(t, srv, root, sess.ID, "README.md", 1, 1, "root comment")

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	rootComments, err := srv.cfg.Store.ListComments(ctx, thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	rootCommentID := rootComments[0].ID

	fileURL := ts.URL + "/s/" + sess.ID + "/files/README.md"
	resp, err := http.PostForm(ts.URL+"/s/"+sess.ID+"/threads/"+thread.ID+"/comments", url.Values{
		"body": {"nested reply"}, "parent_comment_id": {rootCommentID}, "return_to": {fileURL},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	comments, err := srv.cfg.Store.ListComments(ctx, thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 2 {
		t.Fatalf("len(comments) = %d, want 2", len(comments))
	}
	if comments[1].ParentCommentID != rootCommentID {
		t.Errorf("ParentCommentID = %q, want %q", comments[1].ParentCommentID, rootCommentID)
	}

	body := readBody(t, resp)
	if !strings.Contains(body, "nested reply") {
		t.Errorf("file view missing nested reply: %s", body)
	}
}

func TestAddCommentRequiresBody(t *testing.T) {
	srv, root := testServer(t)
	ctx := context.Background()
	sess, err := srv.cfg.Store.CreateSession(ctx, srv.cfg.RepoID, "review", "", testAuthor)
	if err != nil {
		t.Fatal(err)
	}
	thread := createTestThread(t, srv, root, sess.ID, "README.md", 1, 1, "root comment")

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.PostForm(ts.URL+"/s/"+sess.ID+"/threads/"+thread.ID+"/comments", url.Values{"body": {"  "}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a blank body", resp.StatusCode)
	}
}

func TestAddCommentUnknownThreadNotFound(t *testing.T) {
	srv, _ := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.PostForm(ts.URL+"/s/does-not-exist/threads/does-not-exist/comments", url.Values{"body": {"x"}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}
