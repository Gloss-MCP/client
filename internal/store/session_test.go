package store

import (
	"context"
	"errors"
	"testing"
)

func TestSessionCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := testRepo(t, s)

	created, err := s.CreateSession(ctx, repo.ID, "API review", "review the API design", "ben")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if created.Status != SessionOpen {
		t.Errorf("new session status = %q, want open", created.Status)
	}

	got, err := s.GetSession(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Name != "API review" || got.Description != "review the API design" || got.CreatedBy != "ben" {
		t.Errorf("GetSession = %+v, want fields round-tripped", got)
	}
	if !got.CreatedAt.Equal(created.CreatedAt) || !got.UpdatedAt.Equal(created.UpdatedAt) {
		t.Errorf("timestamps did not round-trip: %+v vs %+v", got, created)
	}

	updated, err := s.UpdateSession(ctx, created.ID, "API review v2", "narrowed scope", SessionResolved)
	if err != nil {
		t.Fatalf("UpdateSession: %v", err)
	}
	if updated.Name != "API review v2" || updated.Status != SessionResolved {
		t.Errorf("UpdateSession = %+v, want updated fields", updated)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Errorf("UpdateSession did not bump updated_at: %v <= %v", updated.UpdatedAt, created.UpdatedAt)
	}
	if !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("UpdateSession changed created_at")
	}
}

func TestListSessionsFiltersByStatus(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := testRepo(t, s)

	open := testSession(t, s, repo.ID)
	archived := testSession(t, s, repo.ID)
	if _, err := s.UpdateSession(ctx, archived.ID, archived.Name, "", SessionArchived); err != nil {
		t.Fatalf("UpdateSession: %v", err)
	}

	all, err := s.ListSessions(ctx, repo.ID, "")
	if err != nil {
		t.Fatalf("ListSessions all: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("all sessions = %d, want 2", len(all))
	}

	onlyOpen, err := s.ListSessions(ctx, repo.ID, SessionOpen)
	if err != nil {
		t.Fatalf("ListSessions open: %v", err)
	}
	if len(onlyOpen) != 1 || onlyOpen[0].ID != open.ID {
		t.Errorf("open sessions = %+v, want just %s", onlyOpen, open.ID)
	}
}

func TestSessionNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.GetSession(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetSession err = %v, want ErrNotFound", err)
	}
	if _, err := s.UpdateSession(ctx, "nope", "n", "", SessionOpen); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateSession err = %v, want ErrNotFound", err)
	}
	if err := s.DeleteSession(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteSession err = %v, want ErrNotFound", err)
	}
	if _, err := s.GetSessionStats(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetSessionStats err = %v, want ErrNotFound", err)
	}
}

func TestCreateSessionRejectsBadStatusTransition(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := testRepo(t, s)
	sess := testSession(t, s, repo.ID)

	if _, err := s.UpdateSession(ctx, sess.ID, sess.Name, "", "on-fire"); err == nil {
		t.Fatal("UpdateSession with invalid status succeeded, want CHECK violation")
	}
}

func TestDeleteSessionCascades(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := testRepo(t, s)
	sess := testSession(t, s, repo.ID)
	snap := testSnapshot(t, s, repo.ID, "main.go")
	thread := testThread(t, s, sess.ID, snap.ID)
	comment, err := s.AddComment(ctx, AddCommentParams{
		ThreadID: thread.ID, AuthorType: AuthorAI, AuthorAgent: "claude-opus-4", Body: "reply",
	})
	if err != nil {
		t.Fatalf("AddComment: %v", err)
	}

	if err := s.DeleteSession(ctx, sess.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	if _, err := s.GetSession(ctx, sess.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("session survived delete: %v", err)
	}
	if _, _, err := s.GetThread(ctx, thread.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("thread survived session delete: %v", err)
	}
	if _, err := s.GetComment(ctx, comment.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("comment survived session delete: %v", err)
	}
	// The file snapshot is repo-level and must survive.
	if _, err := s.GetFileSnapshot(ctx, snap.ID); err != nil {
		t.Errorf("file snapshot did not survive session delete: %v", err)
	}
}

func TestGetSessionStats(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := testRepo(t, s)
	sess := testSession(t, s, repo.ID)
	snap := testSnapshot(t, s, repo.ID, "main.go")

	active := testThread(t, s, sess.ID, snap.ID)
	resolved := testThread(t, s, sess.ID, snap.ID)
	if _, err := s.SetAnchorStatus(ctx, resolved.ID, AnchorResolved); err != nil {
		t.Fatalf("SetAnchorStatus: %v", err)
	}
	orphaned := testThread(t, s, sess.ID, snap.ID)
	if _, err := s.SetAnchorStatus(ctx, orphaned.ID, AnchorOrphaned); err != nil {
		t.Fatalf("SetAnchorStatus: %v", err)
	}

	// One extra live reply, and one soft-deleted comment that the count
	// must exclude. Each thread already has its root comment.
	if _, err := s.AddComment(ctx, AddCommentParams{ThreadID: active.ID, AuthorType: AuthorHuman, Body: "keep"}); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	gone, err := s.AddComment(ctx, AddCommentParams{ThreadID: active.ID, AuthorType: AuthorHuman, Body: "drop"})
	if err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	if err := s.SoftDeleteComment(ctx, gone.ID); err != nil {
		t.Fatalf("SoftDeleteComment: %v", err)
	}

	stats, err := s.GetSessionStats(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetSessionStats: %v", err)
	}
	want := SessionStats{TotalThreads: 3, ActiveThreads: 1, OrphanedThreads: 1, ResolvedThreads: 1, TotalComments: 4}
	if *stats != want {
		t.Errorf("stats = %+v, want %+v", *stats, want)
	}
}
