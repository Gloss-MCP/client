package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
