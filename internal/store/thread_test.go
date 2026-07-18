package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestCreateThreadWithRootComment(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := testRepo(t, s)
	sess := testSession(t, s, repo.ID)
	snap := testSnapshot(t, s, repo.ID, "main.go")

	thread, root, err := s.CreateThread(ctx, CreateThreadParams{
		SessionID:      sess.ID,
		FileSnapshotID: snap.ID,
		Anchor:         testLineAnchor(),
		CreatedBy:      "claude-opus-4",
		Body:           "this loop leaks the ticker",
		AuthorType:     AuthorAI,
		AuthorAgent:    "claude-opus-4",
	})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	if thread.AnchorStatus != AnchorActive {
		t.Errorf("new thread anchor_status = %q, want active", thread.AnchorStatus)
	}

	got, comments, err := s.GetThread(ctx, thread.ID)
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	if got.SessionID != sess.ID || got.FileSnapshotID != snap.ID || got.CreatedBy != "claude-opus-4" {
		t.Errorf("GetThread = %+v, want fields round-tripped", got)
	}
	if len(comments) != 1 {
		t.Fatalf("comment chain length = %d, want 1 (the root)", len(comments))
	}
	if comments[0].ID != root.ID || comments[0].Body != "this loop leaks the ticker" ||
		comments[0].AuthorType != AuthorAI || comments[0].AuthorAgent != "claude-opus-4" ||
		comments[0].ParentCommentID != "" {
		t.Errorf("root comment = %+v, want the one handed to CreateThread", comments[0])
	}
}

func TestCreateThreadAtomicRollback(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := testRepo(t, s)
	sess := testSession(t, s, repo.ID)
	snap := testSnapshot(t, s, repo.ID, "main.go")

	// An invalid author_type fails the comment insert AFTER the thread
	// insert; the whole transaction must roll back.
	_, _, err := s.CreateThread(ctx, CreateThreadParams{
		SessionID:      sess.ID,
		FileSnapshotID: snap.ID,
		Anchor:         testLineAnchor(),
		CreatedBy:      "x",
		Body:           "body",
		AuthorType:     "cyborg",
	})
	if err == nil {
		t.Fatal("CreateThread with invalid author_type succeeded, want error")
	}

	threads, err := s.ListThreads(ctx, sess.ID, ListThreadsFilter{})
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(threads) != 0 {
		t.Errorf("thread row survived rolled-back CreateThread: %d threads", len(threads))
	}
}

func TestCreateThreadRequiresAnchor(t *testing.T) {
	s := newTestStore(t)
	_, _, err := s.CreateThread(context.Background(), CreateThreadParams{
		SessionID: "s", FileSnapshotID: "f", Body: "b", AuthorType: AuthorHuman,
	})
	if err == nil {
		t.Fatal("CreateThread without anchor succeeded, want error")
	}
}

func TestAnchorVariantsRoundTripThroughStore(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := testRepo(t, s)
	sess := testSession(t, s, repo.ID)

	tests := []struct {
		path   string
		anchor Anchor
	}{
		{"main.go", LineAnchor{StartLine: 5, EndLine: 9, ContextBefore: []string{"// setup", "func run() {"}, ContextAfter: []string{"}"}}},
		{"diagram.png", RegionAnchor{X: 12.5, Y: 40, Width: 25, Height: 10.75}},
		{"podcast.mp3", TimeAnchor{StartTime: 61.5, EndTime: 75}},
		{"demo.mp4", RegionTimeAnchor{X: 0, Y: 0, Width: 50, Height: 50, StartTime: 12, EndTime: 14.5}},
	}

	for _, tt := range tests {
		t.Run(string(tt.anchor.AnchorType()), func(t *testing.T) {
			snap := testSnapshot(t, s, repo.ID, tt.path)
			created, _, err := s.CreateThread(ctx, CreateThreadParams{
				SessionID:      sess.ID,
				FileSnapshotID: snap.ID,
				Anchor:         tt.anchor,
				CreatedBy:      "ben",
				Body:           "note",
				AuthorType:     AuthorHuman,
			})
			if err != nil {
				t.Fatalf("CreateThread: %v", err)
			}
			got, _, err := s.GetThread(ctx, created.ID)
			if err != nil {
				t.Fatalf("GetThread: %v", err)
			}
			if !reflect.DeepEqual(got.Anchor, tt.anchor) {
				t.Errorf("anchor round trip = %#v, want %#v", got.Anchor, tt.anchor)
			}
		})
	}
}

func TestSetAnchorStatus(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := testRepo(t, s)
	sess := testSession(t, s, repo.ID)
	snap := testSnapshot(t, s, repo.ID, "main.go")
	thread := testThread(t, s, sess.ID, snap.ID)

	resolved, err := s.SetAnchorStatus(ctx, thread.ID, AnchorResolved)
	if err != nil {
		t.Fatalf("SetAnchorStatus resolve: %v", err)
	}
	if resolved.AnchorStatus != AnchorResolved {
		t.Errorf("anchor_status = %q, want resolved", resolved.AnchorStatus)
	}
	if !resolved.UpdatedAt.After(thread.UpdatedAt) {
		t.Errorf("SetAnchorStatus did not bump updated_at")
	}

	reopened, err := s.SetAnchorStatus(ctx, thread.ID, AnchorActive)
	if err != nil {
		t.Fatalf("SetAnchorStatus reopen: %v", err)
	}
	if reopened.AnchorStatus != AnchorActive {
		t.Errorf("anchor_status = %q, want active", reopened.AnchorStatus)
	}

	if _, err := s.SetAnchorStatus(ctx, thread.ID, "lost"); err == nil {
		t.Error("SetAnchorStatus with invalid status succeeded, want CHECK violation")
	}
	if _, err := s.SetAnchorStatus(ctx, "nope", AnchorResolved); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetAnchorStatus on missing thread = %v, want ErrNotFound", err)
	}
}

func TestUpdateThreadAnchor(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := testRepo(t, s)
	sess := testSession(t, s, repo.ID)
	snap := testSnapshot(t, s, repo.ID, "main.go")
	thread := testThread(t, s, sess.ID, snap.ID)

	remapped := LineAnchor{StartLine: 20, EndLine: 22, ContextBefore: []string{"func main() {"}, ContextAfter: []string{"}"}}
	updated, err := s.UpdateThreadAnchor(ctx, thread.ID, remapped)
	if err != nil {
		t.Fatalf("UpdateThreadAnchor: %v", err)
	}
	if !reflect.DeepEqual(updated.Anchor, remapped) {
		t.Errorf("anchor = %#v, want %#v", updated.Anchor, remapped)
	}
	if _, err := s.UpdateThreadAnchor(ctx, "nope", remapped); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateThreadAnchor on missing thread = %v, want ErrNotFound", err)
	}
}

func TestGetThreadNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, _, err := s.GetThread(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// seedListThreadsFixture builds the corpus the filter tests query
// against. It returns the session ID and the created threads keyed by a
// short label.
func seedListThreadsFixture(t *testing.T, s *Store) (string, map[string]*Thread) {
	t.Helper()
	ctx := context.Background()
	repo := testRepo(t, s)
	sess := testSession(t, s, repo.ID)

	create := func(label, path string, anchor Anchor, authorType AuthorType, authorAgent string) *Thread {
		t.Helper()
		snap := testSnapshot(t, s, repo.ID, path)
		thread, _, err := s.CreateThread(ctx, CreateThreadParams{
			SessionID:      sess.ID,
			FileSnapshotID: snap.ID,
			Anchor:         anchor,
			CreatedBy:      "fixture",
			Body:           "root of " + label,
			AuthorType:     authorType,
			AuthorAgent:    authorAgent,
		})
		if err != nil {
			t.Fatalf("CreateThread %s: %v", label, err)
		}
		return thread
	}

	threads := map[string]*Thread{
		"go-root":     create("go-root", "main.go", LineAnchor{StartLine: 1, EndLine: 1}, AuthorHuman, ""),
		"go-src":      create("go-src", "src/util.go", LineAnchor{StartLine: 2, EndLine: 3}, AuthorAI, "claude-opus-4"),
		"md-src-deep": create("md-src-deep", "src/docs/readme.md", LineAnchor{StartLine: 4, EndLine: 4}, AuthorAI, "claude-code-session-xyz"),
		"png-assets":  create("png-assets", "assets/logo.png", RegionAnchor{X: 1, Y: 2, Width: 3, Height: 4}, AuthorHuman, ""),
	}

	// go-src gets orphaned; a human replies to md-src-deep (must NOT
	// make it match author_type=human, which filters on the root
	// comment).
	if _, err := s.SetAnchorStatus(ctx, threads["go-src"].ID, AnchorOrphaned); err != nil {
		t.Fatalf("SetAnchorStatus: %v", err)
	}
	if _, err := s.AddComment(ctx, AddCommentParams{
		ThreadID: threads["md-src-deep"].ID, AuthorType: AuthorHuman, Body: "human reply",
	}); err != nil {
		t.Fatalf("AddComment: %v", err)
	}

	return sess.ID, threads
}

func TestListThreadsFilters(t *testing.T) {
	s := newTestStore(t)
	sessID, threads := seedListThreadsFixture(t, s)

	tests := []struct {
		name   string
		filter ListThreadsFilter
		want   []string // fixture labels
	}{
		{"no filter", ListThreadsFilter{}, []string{"go-root", "go-src", "md-src-deep", "png-assets"}},
		{"file path", ListThreadsFilter{FilePath: "src/util.go"}, []string{"go-src"}},
		{"file path misses subpaths", ListThreadsFilter{FilePath: "main.go"}, []string{"go-root"}},
		{"directory", ListThreadsFilter{Directory: "src"}, []string{"go-src", "md-src-deep"}},
		{"directory trailing slash", ListThreadsFilter{Directory: "src/"}, []string{"go-src", "md-src-deep"}},
		{"nested directory", ListThreadsFilter{Directory: "src/docs"}, []string{"md-src-deep"}},
		{"directory is not a prefix match on file names", ListThreadsFilter{Directory: "main.go"}, nil},
		{"file type", ListThreadsFilter{FileType: "go"}, []string{"go-root", "go-src"}},
		{"file type with dot", ListThreadsFilter{FileType: ".md"}, []string{"md-src-deep"}},
		{"anchor status", ListThreadsFilter{AnchorStatus: AnchorOrphaned}, []string{"go-src"}},
		{"author type human matches root comment only", ListThreadsFilter{AuthorType: AuthorHuman}, []string{"go-root", "png-assets"}},
		{"author type ai", ListThreadsFilter{AuthorType: AuthorAI}, []string{"go-src", "md-src-deep"}},
		{"author agent", ListThreadsFilter{AuthorAgent: "claude-opus-4"}, []string{"go-src"}},
		{"combined", ListThreadsFilter{Directory: "src", FileType: "go", AnchorStatus: AnchorOrphaned, AuthorType: AuthorAI}, []string{"go-src"}},
		{"combined excluding", ListThreadsFilter{Directory: "src", FileType: "go", AuthorType: AuthorHuman}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.ListThreads(context.Background(), sessID, tt.filter)
			if err != nil {
				t.Fatalf("ListThreads: %v", err)
			}
			wantIDs := make(map[string]bool, len(tt.want))
			for _, label := range tt.want {
				wantIDs[threads[label].ID] = true
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d threads, want %d", len(got), len(tt.want))
			}
			for _, th := range got {
				if !wantIDs[th.ID] {
					t.Errorf("unexpected thread %s in results", th.ID)
				}
			}
		})
	}
}

func TestListActiveLineThreadsForPath(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := testRepo(t, s)
	sess := testSession(t, s, repo.ID)

	snapGo := testSnapshot(t, s, repo.ID, "main.go")
	snapPng := testSnapshot(t, s, repo.ID, "logo.png")

	// Active line thread on main.go — should be returned.
	lineActive := testThread(t, s, sess.ID, snapGo.ID)

	// Orphaned line thread on main.go — must NOT be returned.
	lineOrphaned := testThread(t, s, sess.ID, snapGo.ID)
	if _, err := s.SetAnchorStatus(ctx, lineOrphaned.ID, AnchorOrphaned); err != nil {
		t.Fatalf("SetAnchorStatus: %v", err)
	}

	// Active region (non-line) thread on main.go — must NOT be returned.
	regionThread, _, err := s.CreateThread(ctx, CreateThreadParams{
		SessionID:      sess.ID,
		FileSnapshotID: snapGo.ID,
		Anchor:         RegionAnchor{X: 10, Y: 10, Width: 50, Height: 50},
		CreatedBy:      "ben",
		Body:           "region",
		AuthorType:     AuthorHuman,
	})
	if err != nil {
		t.Fatalf("CreateThread region: %v", err)
	}
	_ = regionThread

	// Active line thread on a different file — must NOT be returned.
	otherSnap := testSnapshot(t, s, repo.ID, "other.go")
	testThread(t, s, sess.ID, otherSnap.ID)

	// Active line thread on a png — verifies path filter works.
	testThread(t, s, sess.ID, snapPng.ID)

	got, err := s.ListActiveLineThreadsForPath(ctx, repo.ID, "main.go")
	if err != nil {
		t.Fatalf("ListActiveLineThreadsForPath: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d threads, want 1", len(got))
	}
	if got[0].ID != lineActive.ID {
		t.Errorf("got thread %s, want %s", got[0].ID, lineActive.ID)
	}

	// Cross-repo isolation: same path in a different repo must return nothing.
	repo2, err := s.CreateRepository(ctx, "other-repo", ConnectorLocal, "")
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	cross, err := s.ListActiveLineThreadsForPath(ctx, repo2.ID, "main.go")
	if err != nil {
		t.Fatalf("ListActiveLineThreadsForPath cross-repo: %v", err)
	}
	if len(cross) != 0 {
		t.Errorf("cross-repo returned %d threads, want 0", len(cross))
	}
}

func TestListThreadsScopedToSession(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := testRepo(t, s)
	sessA := testSession(t, s, repo.ID)
	sessB := testSession(t, s, repo.ID)
	snap := testSnapshot(t, s, repo.ID, "main.go")
	testThread(t, s, sessA.ID, snap.ID)

	inB, err := s.ListThreads(ctx, sessB.ID, ListThreadsFilter{})
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(inB) != 0 {
		t.Errorf("session B sees %d threads from session A", len(inB))
	}
}
