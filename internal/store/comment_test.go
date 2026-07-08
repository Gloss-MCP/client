package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// commentFixture is the common thread setup for comment tests.
func commentFixture(t *testing.T, s *Store) *Thread {
	t.Helper()
	repo := testRepo(t, s)
	sess := testSession(t, s, repo.ID)
	snap := testSnapshot(t, s, repo.ID, "main.go")
	return testThread(t, s, sess.ID, snap.ID)
}

func TestAddCommentNesting(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	thread := commentFixture(t, s)

	reply, err := s.AddComment(ctx, AddCommentParams{
		ThreadID: thread.ID, AuthorType: AuthorAI, AuthorAgent: "claude-opus-4", Body: "top-level reply",
	})
	if err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	if reply.ParentCommentID != "" {
		t.Errorf("top-level reply has parent %q", reply.ParentCommentID)
	}

	nested, err := s.AddComment(ctx, AddCommentParams{
		ThreadID: thread.ID, ParentCommentID: reply.ID, AuthorType: AuthorHuman, Body: "nested reply",
	})
	if err != nil {
		t.Fatalf("AddComment nested: %v", err)
	}

	got, err := s.GetComment(ctx, nested.ID)
	if err != nil {
		t.Fatalf("GetComment: %v", err)
	}
	if got.ParentCommentID != reply.ID || got.AuthorType != AuthorHuman || got.Body != "nested reply" {
		t.Errorf("GetComment = %+v, want nested fields round-tripped", got)
	}
	if got.AuthorAgent != "" {
		t.Errorf("AuthorAgent = %q, want empty (stored as NULL)", got.AuthorAgent)
	}

	// Root + reply + nested, oldest first.
	comments, err := s.ListComments(ctx, thread.ID)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(comments) != 3 {
		t.Fatalf("comment count = %d, want 3", len(comments))
	}
	if comments[1].ID != reply.ID || comments[2].ID != nested.ID {
		t.Errorf("comments out of order: %s, %s, %s", comments[0].ID, comments[1].ID, comments[2].ID)
	}
}

func TestAddCommentValidation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	threadA := commentFixture(t, s)
	threadB := commentFixture(t, s)

	if _, err := s.AddComment(ctx, AddCommentParams{
		ThreadID: "nope", AuthorType: AuthorHuman, Body: "x",
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("AddComment to missing thread = %v, want ErrNotFound", err)
	}

	if _, err := s.AddComment(ctx, AddCommentParams{
		ThreadID: threadA.ID, ParentCommentID: "nope", AuthorType: AuthorHuman, Body: "x",
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("AddComment with missing parent = %v, want ErrNotFound", err)
	}

	rootB, err := s.ListComments(ctx, threadB.ID)
	if err != nil || len(rootB) != 1 {
		t.Fatalf("ListComments threadB: %v (%d)", err, len(rootB))
	}
	_, err = s.AddComment(ctx, AddCommentParams{
		ThreadID: threadA.ID, ParentCommentID: rootB[0].ID, AuthorType: AuthorHuman, Body: "x",
	})
	if err == nil || !strings.Contains(err.Error(), "belongs to thread") {
		t.Errorf("AddComment with cross-thread parent = %v, want thread-mismatch error", err)
	}
}

func TestUpdateCommentBody(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	thread := commentFixture(t, s)
	comment, err := s.AddComment(ctx, AddCommentParams{
		ThreadID: thread.ID, AuthorType: AuthorHuman, Body: "first draft",
	})
	if err != nil {
		t.Fatalf("AddComment: %v", err)
	}

	updated, err := s.UpdateCommentBody(ctx, comment.ID, "second draft")
	if err != nil {
		t.Fatalf("UpdateCommentBody: %v", err)
	}
	if updated.Body != "second draft" {
		t.Errorf("body = %q, want %q", updated.Body, "second draft")
	}
	if !updated.UpdatedAt.After(comment.UpdatedAt) {
		t.Errorf("UpdateCommentBody did not bump updated_at")
	}
	if !updated.CreatedAt.Equal(comment.CreatedAt) {
		t.Errorf("UpdateCommentBody changed created_at")
	}

	if _, err := s.UpdateCommentBody(ctx, "nope", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateCommentBody on missing comment = %v, want ErrNotFound", err)
	}
}

func TestSoftDeleteComment(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	thread := commentFixture(t, s)
	comment, err := s.AddComment(ctx, AddCommentParams{
		ThreadID: thread.ID, AuthorType: AuthorHuman, Body: "delete me",
	})
	if err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	nested, err := s.AddComment(ctx, AddCommentParams{
		ThreadID: thread.ID, ParentCommentID: comment.ID, AuthorType: AuthorAI, Body: "child",
	})
	if err != nil {
		t.Fatalf("AddComment nested: %v", err)
	}

	if err := s.SoftDeleteComment(ctx, comment.ID); err != nil {
		t.Fatalf("SoftDeleteComment: %v", err)
	}

	got, err := s.GetComment(ctx, comment.ID)
	if err != nil {
		t.Fatalf("GetComment after soft delete: %v", err)
	}
	if got.DeletedAt == nil {
		t.Fatal("DeletedAt not set after soft delete")
	}

	// The row survives: the nested reply stays attached and the chain
	// keeps its shape.
	comments, err := s.ListComments(ctx, thread.ID)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(comments) != 3 {
		t.Errorf("comment count after soft delete = %d, want 3", len(comments))
	}
	child, err := s.GetComment(ctx, nested.ID)
	if err != nil {
		t.Fatalf("GetComment child: %v", err)
	}
	if child.ParentCommentID != comment.ID {
		t.Errorf("child lost its parent after soft delete")
	}

	// Idempotent: deleting again succeeds and keeps the original
	// deleted_at.
	if err := s.SoftDeleteComment(ctx, comment.ID); err != nil {
		t.Fatalf("second SoftDeleteComment: %v", err)
	}
	again, err := s.GetComment(ctx, comment.ID)
	if err != nil {
		t.Fatalf("GetComment: %v", err)
	}
	if !again.DeletedAt.Equal(*got.DeletedAt) {
		t.Errorf("repeat delete moved deleted_at from %v to %v", got.DeletedAt, again.DeletedAt)
	}

	// Editing a deleted comment is rejected.
	if _, err := s.UpdateCommentBody(ctx, comment.ID, "resurrect"); !errors.Is(err, ErrCommentDeleted) {
		t.Errorf("UpdateCommentBody on deleted comment = %v, want ErrCommentDeleted", err)
	}

	if err := s.SoftDeleteComment(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("SoftDeleteComment on missing comment = %v, want ErrNotFound", err)
	}
}
