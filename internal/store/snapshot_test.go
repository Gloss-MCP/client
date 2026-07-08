package store

import (
	"context"
	"errors"
	"testing"
)

func TestFileSnapshotCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := testRepo(t, s)

	created, err := s.CreateFileSnapshot(ctx, repo.ID, "src/main.go", "abc123", "deadbeef")
	if err != nil {
		t.Fatalf("CreateFileSnapshot: %v", err)
	}

	got, err := s.GetFileSnapshot(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetFileSnapshot: %v", err)
	}
	if got.Path != "src/main.go" || got.ContentHash != "abc123" || got.GitCommitSHA != "deadbeef" {
		t.Errorf("GetFileSnapshot = %+v, want fields round-tripped", got)
	}
	if !got.CapturedAt.Equal(created.CapturedAt) {
		t.Errorf("CapturedAt = %v, want %v", got.CapturedAt, created.CapturedAt)
	}
}

func TestFileSnapshotEmptyGitSHA(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := testRepo(t, s)

	created, err := s.CreateFileSnapshot(ctx, repo.ID, "notes.md", "h1", "")
	if err != nil {
		t.Fatalf("CreateFileSnapshot: %v", err)
	}
	got, err := s.GetFileSnapshot(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetFileSnapshot: %v", err)
	}
	if got.GitCommitSHA != "" {
		t.Errorf("GitCommitSHA = %q, want empty (stored as NULL)", got.GitCommitSHA)
	}
}

func TestFindFileSnapshot(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := testRepo(t, s)

	old, err := s.CreateFileSnapshot(ctx, repo.ID, "a.go", "v1", "")
	if err != nil {
		t.Fatalf("CreateFileSnapshot: %v", err)
	}
	newer, err := s.CreateFileSnapshot(ctx, repo.ID, "a.go", "v1", "")
	if err != nil {
		t.Fatalf("CreateFileSnapshot: %v", err)
	}

	found, err := s.FindFileSnapshot(ctx, repo.ID, "a.go", "v1")
	if err != nil {
		t.Fatalf("FindFileSnapshot: %v", err)
	}
	if found.ID != newer.ID && found.ID != old.ID {
		t.Errorf("FindFileSnapshot returned unrelated snapshot %s", found.ID)
	}

	if _, err := s.FindFileSnapshot(ctx, repo.ID, "a.go", "v2"); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindFileSnapshot for unknown hash = %v, want ErrNotFound", err)
	}
	if _, err := s.FindFileSnapshot(ctx, repo.ID, "b.go", "v1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindFileSnapshot for unknown path = %v, want ErrNotFound", err)
	}
}

func TestListFileSnapshots(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := testRepo(t, s)

	testSnapshot(t, s, repo.ID, "a.go")
	testSnapshot(t, s, repo.ID, "b.go")
	testSnapshot(t, s, repo.ID, "a.go")

	all, err := s.ListFileSnapshots(ctx, repo.ID, "")
	if err != nil {
		t.Fatalf("ListFileSnapshots all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("all snapshots = %d, want 3", len(all))
	}

	onlyA, err := s.ListFileSnapshots(ctx, repo.ID, "a.go")
	if err != nil {
		t.Fatalf("ListFileSnapshots a.go: %v", err)
	}
	if len(onlyA) != 2 {
		t.Errorf("a.go snapshots = %d, want 2", len(onlyA))
	}
	for _, snap := range onlyA {
		if snap.Path != "a.go" {
			t.Errorf("filtered snapshot has path %q", snap.Path)
		}
	}
}

func TestGetFileSnapshotNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetFileSnapshot(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
