package connector

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
)

var shaRE = regexp.MustCompile(`^[0-9a-f]{40}$`)

// initGitRepo runs `git init` in dir and configures a throwaway commit
// identity, so tests don't depend on the host's global git config.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "fixture@example.com")
	runGit(t, dir, "config", "user.name", "Fixture")
}

// commitAll stages every file in dir and commits with message.
func commitAll(t *testing.T, dir, message string) {
	t.Helper()
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", message)
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestHeadCommitSHA(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, dir, "initial")

	got, err := headCommitSHA(context.Background(), dir)
	if err != nil {
		t.Fatalf("headCommitSHA: %v", err)
	}
	if !shaRE.MatchString(got) {
		t.Errorf("headCommitSHA = %q, want a 40-char hex SHA", got)
	}

	want := runGit(t, dir, "rev-parse", "HEAD")
	if got+"\n" != want {
		t.Errorf("headCommitSHA = %q, want %q", got, want)
	}
}

func TestHeadCommitSHANonGitDir(t *testing.T) {
	dir := t.TempDir()
	if _, err := headCommitSHA(context.Background(), dir); err == nil {
		t.Error("headCommitSHA in non-git dir succeeded, want error")
	}
}

func TestHeadCommitSHANoCommitsYet(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	if _, err := headCommitSHA(context.Background(), dir); err == nil {
		t.Error("headCommitSHA with no commits succeeded, want error")
	}
}
