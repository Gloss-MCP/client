package store

import (
	"context"
	"errors"
	"testing"
)

func TestRepositoryCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	created, err := s.CreateRepository(ctx, "my-project", ConnectorGit, `{"remote":"origin"}`)
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	if created.ID == "" {
		t.Fatal("created repository has empty ID")
	}

	got, err := s.GetRepository(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if got.Name != "my-project" || got.ConnectorType != ConnectorGit || got.ConnectorConfig != `{"remote":"origin"}` {
		t.Errorf("got %+v, want name/connector/config round-tripped", got)
	}
	if !got.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, created.CreatedAt)
	}

	second, err := s.CreateRepository(ctx, "second", ConnectorLocal, "")
	if err != nil {
		t.Fatalf("CreateRepository second: %v", err)
	}
	if second.ConnectorConfig != "{}" {
		t.Errorf("empty connector config stored as %q, want {}", second.ConnectorConfig)
	}

	repos, err := s.ListRepositories(ctx)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repos) != 2 || repos[0].ID != created.ID || repos[1].ID != second.ID {
		t.Errorf("ListRepositories returned %d repos in unexpected order", len(repos))
	}
}

func TestGetRepositoryNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetRepository(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestCreateRepositoryRejectsBadConnectorType(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateRepository(context.Background(), "x", "carrier-pigeon", ""); err == nil {
		t.Fatal("CreateRepository with invalid connector_type succeeded, want CHECK violation")
	}
}

func TestEnsureRepository(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	first, err := s.EnsureRepository(ctx, "proj", ConnectorLocal, "")
	if err != nil {
		t.Fatalf("EnsureRepository (create): %v", err)
	}

	// A second call must return the same record, ignoring new
	// attributes — local mode has exactly one repository.
	again, err := s.EnsureRepository(ctx, "renamed", ConnectorGit, "")
	if err != nil {
		t.Fatalf("EnsureRepository (existing): %v", err)
	}
	if again.ID != first.ID || again.Name != "proj" {
		t.Errorf("EnsureRepository returned %+v, want existing record %+v", again, first)
	}

	repos, err := s.ListRepositories(ctx)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repos) != 1 {
		t.Errorf("repository count = %d, want 1", len(repos))
	}
}

func TestRepositorySessionCount(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := testRepo(t, s)

	if n, err := s.RepositorySessionCount(ctx, repo.ID); err != nil || n != 0 {
		t.Fatalf("count = %d, %v; want 0, nil", n, err)
	}
	testSession(t, s, repo.ID)
	testSession(t, s, repo.ID)
	if n, err := s.RepositorySessionCount(ctx, repo.ID); err != nil || n != 2 {
		t.Fatalf("count = %d, %v; want 2, nil", n, err)
	}
}
