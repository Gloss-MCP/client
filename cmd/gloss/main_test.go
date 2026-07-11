package main

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gloss-mcp/client/internal/store"
)

func TestRun(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "version flag",
			args:       []string{"--version"},
			wantCode:   0,
			wantStdout: "gloss dev\n",
		},
		{
			name:       "cloud flag reserved",
			args:       []string{"--cloud", dir},
			wantCode:   1,
			wantStderr: "proxy mode (--cloud) is not yet available",
		},
		{
			name:       "token flag reserved",
			args:       []string{"--token", "abc123", dir},
			wantCode:   1,
			wantStderr: "proxy mode (--cloud) is not yet available",
		},
		{
			name:       "nonexistent directory",
			args:       []string{"/nonexistent/gloss/path"},
			wantCode:   1,
			wantStderr: "no such file or directory",
		},
		{
			name:       "too many arguments",
			args:       []string{dir, dir},
			wantCode:   2,
			wantStderr: "expected at most one directory argument",
		},
		{
			name:       "unknown flag",
			args:       []string{"--bogus"},
			wantCode:   2,
			wantStderr: "flag provided but not defined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(context.Background(), tt.args, &stdout, &stderr)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d (stderr: %q)", code, tt.wantCode, stderr.String())
			}
			if tt.wantStdout != "" && stdout.String() != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout.String(), tt.wantStdout)
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestRunFileNotDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "plain.txt")
	if err := os.WriteFile(file, []byte("not a directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{file}, &stdout, &stderr)

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "is not a directory") {
		t.Errorf("stderr = %q, want it to contain %q", stderr.String(), "is not a directory")
	}
}

// syncBuffer is a concurrency-safe io.Writer, needed because these tests
// read stderr from a separate goroutine while run's server is live.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

var servingURLRe = regexp.MustCompile(`serving .* at (http://\S+)`)

// runningServer runs `gloss -port 0 -no-open dir` in the background (an
// OS-assigned port, isolating parallel tests) until the returned stop
// func is called, which cancels its context and waits for run to return
// -- guaranteeing the store is closed before the caller inspects the DB
// file directly.
func runningServer(t *testing.T, dir string) (baseURL string, stop func()) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	stderr := &syncBuffer{}
	var stdout bytes.Buffer
	doneCh := make(chan int, 1)

	go func() {
		doneCh <- run(ctx, []string{"-port", "0", "-no-open", dir}, &stdout, stderr)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if m := servingURLRe.FindStringSubmatch(stderr.String()); m != nil {
			return m[1], func() {
				cancel()
				select {
				case code := <-doneCh:
					if code != 0 {
						t.Errorf("run exit code = %d, want 0 after graceful shutdown (stderr: %q)", code, stderr.String())
					}
				case <-time.After(5 * time.Second):
					t.Fatal("run did not return after context cancellation")
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	t.Fatalf("server never became ready; stderr: %q", stderr.String())
	return "", nil
}

// TestRunServesAndInitialisesStore covers this milestone's exit
// criterion end to end: `gloss .` creates .gloss/gloss.db and serves a
// browsable UI over HTTP.
func TestRunServesAndInitialisesStore(t *testing.T) {
	dir := t.TempDir()

	url, stop := runningServer(t, dir)
	resp, err := http.Get(url + "/")
	if err != nil {
		t.Fatalf("GET %s/: %v", url, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	stop()

	dbPath := filepath.Join(dir, ".gloss", "gloss.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf(".gloss/gloss.db not created: %v", err)
	}

	// Running again against an existing store must succeed (idempotent
	// open + ensure).
	_, stop2 := runningServer(t, dir)
	stop2()
}

// TestRunSnapshotsFiles covers the milestone-3 exit criterion for the
// local connector: `gloss .` snapshots tracked files, respecting
// .glossignore, on every run.
func TestRunSnapshotsFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scratch.draft"), []byte("scratch"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".glossignore"), []byte("*.draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stop := runningServer(t, dir)
	stop()

	st, err := store.Open(filepath.Join(dir, ".gloss", "gloss.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer func() { _ = st.Close() }()

	ctx := context.Background()
	repos, err := st.ListRepositories(ctx)
	if err != nil || len(repos) != 1 {
		t.Fatalf("ListRepositories: err=%v repos=%d, want exactly 1", err, len(repos))
	}
	if repos[0].ConnectorType != store.ConnectorLocal {
		t.Errorf("ConnectorType = %q, want %q", repos[0].ConnectorType, store.ConnectorLocal)
	}

	snaps, err := st.ListFileSnapshots(ctx, repos[0].ID, "")
	if err != nil {
		t.Fatalf("ListFileSnapshots: %v", err)
	}
	got := make([]string, len(snaps))
	for i, s := range snaps {
		got[i] = s.Path
	}
	want := []string{"keep.txt", ".glossignore"}
	for _, w := range want {
		if !containsString(got, w) {
			t.Errorf("snapshot paths = %v, want to contain %q", got, w)
		}
	}
	if containsString(got, "scratch.draft") {
		t.Errorf("snapshot paths = %v, want scratch.draft excluded by .glossignore", got)
	}
}

// TestRunSnapshotsGitRepo covers the milestone-3 exit criterion for the
// git connector: `gloss .` detects a git repo, respects .gitignore, and
// records the current commit SHA on each FileSnapshot.
func TestRunSnapshotsGitRepo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secret.log"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, dir, "init", "-q")
	runGitCmd(t, dir, "config", "user.email", "fixture@example.com")
	runGitCmd(t, dir, "config", "user.name", "Fixture")
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "initial")

	_, stop := runningServer(t, dir)
	stop()

	st, err := store.Open(filepath.Join(dir, ".gloss", "gloss.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer func() { _ = st.Close() }()

	ctx := context.Background()
	repos, err := st.ListRepositories(ctx)
	if err != nil || len(repos) != 1 {
		t.Fatalf("ListRepositories: err=%v repos=%d, want exactly 1", err, len(repos))
	}
	if repos[0].ConnectorType != store.ConnectorGit {
		t.Errorf("ConnectorType = %q, want %q", repos[0].ConnectorType, store.ConnectorGit)
	}

	snaps, err := st.ListFileSnapshots(ctx, repos[0].ID, "")
	if err != nil {
		t.Fatalf("ListFileSnapshots: %v", err)
	}
	wantSHA := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))
	for _, s := range snaps {
		if s.Path == "secret.log" {
			t.Errorf("secret.log should be excluded by .gitignore, found snapshot %+v", s)
		}
		if s.GitCommitSHA != wantSHA {
			t.Errorf("snapshot %s GitCommitSHA = %q, want %q", s.Path, s.GitCommitSHA, wantSHA)
		}
	}
}

// TestRunPortInUse covers the -port flag's error path: a fixed port
// that's already taken fails clearly instead of silently picking another.
func TestRunPortInUse(t *testing.T) {
	url, stop := runningServer(t, t.TempDir())
	defer stop()

	port := strings.TrimPrefix(url, "http://127.0.0.1:")

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-port", port, "-no-open", t.TempDir()}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stderr: %q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "already in use") {
		t.Errorf("stderr = %q, want it to mention the port is already in use", stderr.String())
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func runGitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}
