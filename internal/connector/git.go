package connector

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// headCommitSHA shells out to `git rev-parse HEAD` in dir. Shelling out
// keeps the build CGO-free and dependency-light (vs. a git-in-Go
// library); CI and any dev machine already has git installed.
//
// Failure (dir is not a git repository, no commits yet, or git is not
// on PATH) is returned as an error. Callers treat this as best-effort:
// an empty commit SHA is a valid, expected state for a fresh repo.
func headCommitSHA(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git rev-parse HEAD in %s: %w: %s", dir, err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}
