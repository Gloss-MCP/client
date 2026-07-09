package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
			name:       "server mode placeholder",
			args:       []string{dir},
			wantCode:   1,
			wantStderr: "server mode is not yet implemented",
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
			code := run(tt.args, &stdout, &stderr)

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

// TestRunInitialisesStore covers the milestone-2 exit criterion:
// `gloss .` creates .gloss/gloss.db in the target directory.
func TestRunInitialisesStore(t *testing.T) {
	dir := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := run([]string{dir}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stderr: %q)", code, stderr.String())
	}

	dbPath := filepath.Join(dir, ".gloss", "gloss.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf(".gloss/gloss.db not created: %v", err)
	}

	// Running again against an existing store must succeed (idempotent
	// open + ensure).
	stderr.Reset()
	if code := run([]string{dir}, &stdout, &stderr); code != 1 {
		t.Fatalf("second run exit code = %d, want 1 (stderr: %q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "server mode is not yet implemented") {
		t.Errorf("stderr = %q, want the server-mode placeholder", stderr.String())
	}
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

	var stdout, stderr bytes.Buffer
	if code := run([]string{dir}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit code = %d, want 1 (stderr: %q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "indexed 2 files") {
		t.Errorf("stderr = %q, want it to report 2 indexed files", stderr.String())
	}

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

	var stdout, stderr bytes.Buffer
	if code := run([]string{dir}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit code = %d, want 1 (stderr: %q)", code, stderr.String())
	}

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

func TestRunFileNotDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "plain.txt")
	if err := os.WriteFile(file, []byte("not a directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{file}, &stdout, &stderr)

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "is not a directory") {
		t.Errorf("stderr = %q, want it to contain %q", stderr.String(), "is not a directory")
	}
}
