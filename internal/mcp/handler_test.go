package mcp_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	glossmcp "github.com/gloss-mcp/client/internal/mcp"
	"github.com/gloss-mcp/client/internal/store"
)

// newTestEnv boots a test HTTP server backed by a real SQLite store and a
// temp directory containing a tracked file. It returns the MCP client
// session, the store, and the repo record.
func newTestEnv(t *testing.T) (session *sdkmcp.ClientSession, st *store.Store, repo *store.Repository, root string) {
	t.Helper()

	root = t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}

	st, err := store.Open(filepath.Join(t.TempDir(), "gloss.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	ctx := context.Background()
	repo, err = st.CreateRepository(ctx, "fixture", store.ConnectorLocal, "")
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	handler := glossmcp.NewHandler(glossmcp.Config{
		Store:         st,
		RepoID:        repo.ID,
		Root:          root,
		ConnectorType: store.ConnectorLocal,
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "0"}, nil)
	session, err = client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: srv.URL}, nil)
	if err != nil {
		t.Fatalf("mcp client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	return session, st, repo, root
}

// callTool is a convenience wrapper that calls a tool and returns the
// TextContent text from the first result item.
func callTool(t *testing.T, session *sdkmcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	ctx := context.Background()
	res, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	if res.IsError {
		t.Fatalf("CallTool(%s) returned tool error: %v", name, res.Content)
	}
	if len(res.Content) == 0 {
		t.Fatalf("CallTool(%s): no content in result", name)
	}
	tc, ok := res.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatalf("CallTool(%s): first content item is not TextContent", name)
	}
	return tc.Text
}

// callToolExpectError calls a tool and asserts it returned a tool-level
// error result. It returns the error text from the first content item.
func callToolExpectError(t *testing.T, session *sdkmcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	ctx := context.Background()
	res, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%s): unexpected RPC error: %v", name, err)
	}
	if !res.IsError {
		t.Fatalf("CallTool(%s): expected tool error, got success: %v", name, res.Content)
	}
	if len(res.Content) == 0 {
		return ""
	}
	tc, ok := res.Content[0].(*sdkmcp.TextContent)
	if !ok {
		return ""
	}
	return tc.Text
}

// createSession creates a session via the MCP tool and returns its ID.
func createSession(t *testing.T, session *sdkmcp.ClientSession, repoID, name string) string {
	t.Helper()
	text := callTool(t, session, "create_session", map[string]any{"repo_id": repoID, "name": name})
	var obj map[string]any
	if err := json.Unmarshal([]byte(text), &obj); err != nil {
		t.Fatalf("unmarshal create_session: %v", err)
	}
	id, _ := obj["id"].(string)
	if id == "" {
		t.Fatal("create_session: empty id")
	}
	return id
}

// createThread creates a thread on main.go lines 1–1 and returns its ID.
func createThread(t *testing.T, session *sdkmcp.ClientSession, sessID, body string) string {
	t.Helper()
	text := callTool(t, session, "create_thread", map[string]any{
		"session_id": sessID,
		"file_path":  "main.go",
		"anchor":     map[string]any{"type": "line", "start_line": 1, "end_line": 1},
		"body":       body,
	})
	var obj map[string]any
	if err := json.Unmarshal([]byte(text), &obj); err != nil {
		t.Fatalf("unmarshal create_thread: %v", err)
	}
	id, _ := obj["id"].(string)
	if id == "" {
		t.Fatal("create_thread: empty id")
	}
	return id
}

func TestListToolsExposesAllTools(t *testing.T) {
	session, _, _, _ := newTestEnv(t)
	ctx := context.Background()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	want := []string{
		"create_session", "list_sessions", "get_session",
		"create_thread", "get_thread", "list_threads", "resolve_thread", "reopen_thread",
		"add_comment", "edit_comment", "delete_comment",
		"list_repos", "get_repo",
	}
	got := make(map[string]bool, len(result.Tools))
	for _, tool := range result.Tools {
		got[tool.Name] = true
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("tool %q not in tools/list response", name)
		}
	}
	if len(result.Tools) != len(want) {
		t.Errorf("tools/list: got %d tools, want %d", len(result.Tools), len(want))
	}
}

func TestListRepos(t *testing.T) {
	session, _, repo, _ := newTestEnv(t)

	text := callTool(t, session, "list_repos", nil)

	var repos []map[string]any
	if err := json.Unmarshal([]byte(text), &repos); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("list_repos: got %d repos, want 1", len(repos))
	}
	if repos[0]["id"] != repo.ID {
		t.Errorf("list_repos: id = %s, want %s", repos[0]["id"], repo.ID)
	}
}

func TestGetRepo(t *testing.T) {
	session, _, repo, _ := newTestEnv(t)

	text := callTool(t, session, "get_repo", map[string]any{"repo_id": repo.ID})

	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["id"] != repo.ID {
		t.Errorf("get_repo: id = %v, want %s", result["id"], repo.ID)
	}
	if result["session_count"].(float64) != 0 {
		t.Errorf("get_repo: session_count = %v, want 0", result["session_count"])
	}

	// unknown repo returns tool error
	callToolExpectError(t, session, "get_repo", map[string]any{"repo_id": "no-such-repo"})
}

func TestSessionRoundTrip(t *testing.T) {
	session, _, repo, _ := newTestEnv(t)

	sessID := createSession(t, session, repo.ID, "review-1")

	// list returns the new session
	listText := callTool(t, session, "list_sessions", map[string]any{"repo_id": repo.ID})
	var sessions []map[string]any
	if err := json.Unmarshal([]byte(listText), &sessions); err != nil {
		t.Fatalf("unmarshal list_sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0]["id"] != sessID {
		t.Errorf("list_sessions: got %v, want 1 entry with id %s", sessions, sessID)
	}

	// get returns session + stats
	getText := callTool(t, session, "get_session", map[string]any{"session_id": sessID})
	var result map[string]any
	if err := json.Unmarshal([]byte(getText), &result); err != nil {
		t.Fatalf("unmarshal get_session: %v", err)
	}
	if result["id"] != sessID {
		t.Errorf("get_session: id = %v, want %s", result["id"], sessID)
	}
	if result["stats"] == nil {
		t.Error("get_session: missing stats field")
	}

	// unknown session returns tool error
	callToolExpectError(t, session, "get_session", map[string]any{"session_id": "no-such-sess"})
}

func TestSessionMissingFields(t *testing.T) {
	session, _, _, _ := newTestEnv(t)
	callToolExpectError(t, session, "create_session", map[string]any{"name": "x"})    // missing repo_id
	callToolExpectError(t, session, "create_session", map[string]any{"repo_id": "r"}) // missing name
	callToolExpectError(t, session, "list_sessions", map[string]any{})                // missing repo_id
	callToolExpectError(t, session, "get_session", map[string]any{})                  // missing session_id
}

func TestThreadRoundTrip(t *testing.T) {
	session, _, repo, _ := newTestEnv(t)

	sessID := createSession(t, session, repo.ID, "review")

	// create thread
	text := callTool(t, session, "create_thread", map[string]any{
		"session_id":   sessID,
		"file_path":    "main.go",
		"anchor":       map[string]any{"type": "line", "start_line": 1, "end_line": 2},
		"body":         "this function could be clearer",
		"author_agent": "claude-test",
	})
	var thread map[string]any
	if err := json.Unmarshal([]byte(text), &thread); err != nil {
		t.Fatalf("unmarshal create_thread: %v", err)
	}
	threadID, _ := thread["id"].(string)
	if threadID == "" {
		t.Fatal("create_thread: empty id")
	}
	if thread["anchor_status"] != "active" {
		t.Errorf("create_thread: anchor_status = %v, want active", thread["anchor_status"])
	}

	// get_thread returns thread + comments
	getText := callTool(t, session, "get_thread", map[string]any{"thread_id": threadID})
	var threadFull map[string]any
	if err := json.Unmarshal([]byte(getText), &threadFull); err != nil {
		t.Fatalf("unmarshal get_thread: %v", err)
	}
	comments, _ := threadFull["comments"].([]any)
	if len(comments) != 1 {
		t.Fatalf("get_thread: want 1 root comment, got %d", len(comments))
	}
	root := comments[0].(map[string]any)
	if root["body"] != "this function could be clearer" {
		t.Errorf("root comment body = %v", root["body"])
	}
	if root["author_type"] != "ai" {
		t.Errorf("root comment author_type = %v, want ai", root["author_type"])
	}
	if root["author_agent"] != "claude-test" {
		t.Errorf("root comment author_agent = %v, want claude-test", root["author_agent"])
	}

	// unknown thread returns tool error
	callToolExpectError(t, session, "get_thread", map[string]any{"thread_id": "no-such"})
}

func TestCreateThreadUnknownFile(t *testing.T) {
	session, _, repo, _ := newTestEnv(t)
	sessID := createSession(t, session, repo.ID, "s")

	callToolExpectError(t, session, "create_thread", map[string]any{
		"session_id": sessID,
		"file_path":  "does-not-exist.go",
		"anchor":     map[string]any{"type": "line", "start_line": 1, "end_line": 1},
		"body":       "x",
	})
}

func TestCreateThreadInvalidAnchor(t *testing.T) {
	session, _, repo, _ := newTestEnv(t)
	sessID := createSession(t, session, repo.ID, "s")

	// end_line before start_line
	callToolExpectError(t, session, "create_thread", map[string]any{
		"session_id": sessID,
		"file_path":  "main.go",
		"anchor":     map[string]any{"type": "line", "start_line": 5, "end_line": 3},
		"body":       "x",
	})

	// unknown anchor type
	callToolExpectError(t, session, "create_thread", map[string]any{
		"session_id": sessID,
		"file_path":  "main.go",
		"anchor":     map[string]any{"type": "bogus"},
		"body":       "x",
	})
}

func TestAddCommentAttribution(t *testing.T) {
	session, _, repo, _ := newTestEnv(t)
	sessID := createSession(t, session, repo.ID, "s")
	threadID := createThread(t, session, sessID, "initial")

	// add_comment with explicit author_agent override
	text := callTool(t, session, "add_comment", map[string]any{
		"thread_id":    threadID,
		"body":         "my reply",
		"author_agent": "claude-opus-4",
	})
	var comment map[string]any
	if err := json.Unmarshal([]byte(text), &comment); err != nil {
		t.Fatalf("unmarshal add_comment: %v", err)
	}
	if comment["author_type"] != "ai" {
		t.Errorf("author_type = %v, want ai", comment["author_type"])
	}
	if comment["author_agent"] != "claude-opus-4" {
		t.Errorf("author_agent = %v, want claude-opus-4", comment["author_agent"])
	}
	if comment["thread_id"] != threadID {
		t.Errorf("thread_id = %v, want %s", comment["thread_id"], threadID)
	}
}

func TestListThreadsFilter(t *testing.T) {
	session, _, repo, _ := newTestEnv(t)
	sessID := createSession(t, session, repo.ID, "s")
	threadID := createThread(t, session, sessID, "initial")

	// active filter returns the thread
	listText := callTool(t, session, "list_threads", map[string]any{
		"session_id":    sessID,
		"anchor_status": "active",
	})
	var threads []map[string]any
	if err := json.Unmarshal([]byte(listText), &threads); err != nil {
		t.Fatalf("unmarshal list_threads: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("list_threads(active): want 1, got %d", len(threads))
	}

	// resolve and verify
	callTool(t, session, "resolve_thread", map[string]any{"thread_id": threadID})

	if err := json.Unmarshal([]byte(callTool(t, session, "list_threads", map[string]any{
		"session_id": sessID, "anchor_status": "active",
	})), &threads); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(threads) != 0 {
		t.Errorf("list_threads(active) after resolve: want 0, got %d", len(threads))
	}

	if err := json.Unmarshal([]byte(callTool(t, session, "list_threads", map[string]any{
		"session_id": sessID, "anchor_status": "resolved",
	})), &threads); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(threads) != 1 {
		t.Errorf("list_threads(resolved): want 1, got %d", len(threads))
	}

	// reopen and verify
	callTool(t, session, "reopen_thread", map[string]any{"thread_id": threadID})

	if err := json.Unmarshal([]byte(callTool(t, session, "list_threads", map[string]any{
		"session_id": sessID, "anchor_status": "active",
	})), &threads); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(threads) != 1 {
		t.Errorf("list_threads(active) after reopen: want 1, got %d", len(threads))
	}
}

func TestEditAndDeleteComment(t *testing.T) {
	session, _, repo, _ := newTestEnv(t)
	sessID := createSession(t, session, repo.ID, "s")
	threadID := createThread(t, session, sessID, "initial comment")

	// get root comment ID
	var threadFull map[string]any
	if err := json.Unmarshal([]byte(callTool(t, session, "get_thread", map[string]any{"thread_id": threadID})), &threadFull); err != nil {
		t.Fatalf("unmarshal get_thread: %v", err)
	}
	commentID := threadFull["comments"].([]any)[0].(map[string]any)["id"].(string)

	// edit
	var edited map[string]any
	if err := json.Unmarshal([]byte(callTool(t, session, "edit_comment", map[string]any{
		"comment_id": commentID,
		"body":       "updated body",
	})), &edited); err != nil {
		t.Fatalf("unmarshal edit_comment: %v", err)
	}
	if edited["body"] != "updated body" {
		t.Errorf("edit_comment: body = %v, want 'updated body'", edited["body"])
	}

	// delete
	var deleted map[string]any
	if err := json.Unmarshal([]byte(callTool(t, session, "delete_comment", map[string]any{
		"comment_id": commentID,
	})), &deleted); err != nil {
		t.Fatalf("unmarshal delete_comment: %v", err)
	}
	if deleted["status"] != "deleted" {
		t.Errorf("delete_comment: status = %v, want 'deleted'", deleted["status"])
	}

	// double-delete is idempotent
	callTool(t, session, "delete_comment", map[string]any{"comment_id": commentID})

	// editing a deleted comment is a tool error
	callToolExpectError(t, session, "edit_comment", map[string]any{
		"comment_id": commentID,
		"body":       "can't edit deleted",
	})
}
