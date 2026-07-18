package delta

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gloss-mcp/client/internal/store"
)

// fakeStore is an in-memory watchStore implementation for watcher tests.
type fakeStore struct {
	threads   map[string]*store.Thread       // by ID
	snapshots map[string]*store.FileSnapshot // by ID
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		threads:   make(map[string]*store.Thread),
		snapshots: make(map[string]*store.FileSnapshot),
	}
}

func (f *fakeStore) addThread(t *store.Thread)         { f.threads[t.ID] = t }
func (f *fakeStore) addSnapshot(s *store.FileSnapshot) { f.snapshots[s.ID] = s }

func (f *fakeStore) ListActiveLineThreadsForPath(_ context.Context, _, path string) ([]*store.Thread, error) {
	var out []*store.Thread
	for _, t := range f.threads {
		if t.AnchorStatus != store.AnchorActive {
			continue
		}
		if _, ok := t.Anchor.(store.LineAnchor); !ok {
			continue
		}
		if snap, ok := f.snapshots[t.FileSnapshotID]; ok && snap.Path == path {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeStore) UpdateThreadAnchor(_ context.Context, id string, anchor store.Anchor) (*store.Thread, error) {
	t := f.threads[id]
	t.Anchor = anchor
	return t, nil
}

func (f *fakeStore) SetAnchorStatus(_ context.Context, id string, status store.AnchorStatus) (*store.Thread, error) {
	t := f.threads[id]
	t.AnchorStatus = status
	return t, nil
}

func (f *fakeStore) GetFileSnapshot(_ context.Context, id string) (*store.FileSnapshot, error) {
	s, ok := f.snapshots[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return s, nil
}

func TestWatcherRemapsOnFileWrite(t *testing.T) {
	dir := t.TempDir()

	// Write the initial file content.
	initialContent := "line1\nline2\nline3\nanchor\nline5\nline6\nline7\n"
	path := filepath.Join(dir, "sample.go")
	if err := os.WriteFile(path, []byte(initialContent), 0o644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}

	fs := newFakeStore()

	snap := &store.FileSnapshot{
		ID:          "snap1",
		RepoID:      "repo1",
		Path:        "sample.go",
		ContentHash: contentHash([]byte(initialContent)),
	}
	fs.addSnapshot(snap)

	thread := &store.Thread{
		ID:             "t1",
		FileSnapshotID: "snap1",
		AnchorStatus:   store.AnchorActive,
		Anchor: store.LineAnchor{
			StartLine:     4,
			EndLine:       4,
			ContextBefore: []string{"line1", "line2", "line3"},
			ContextAfter:  []string{"line5", "line6", "line7"},
		},
	}
	fs.addThread(thread)

	w, err := New(dir, "repo1", store.ConnectorLocal, fs)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx, ready) }()

	// Wait until watches are registered before writing, or bail if the
	// watcher fails to start.
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("watcher did not become ready in time")
	}

	// Insert 2 lines before the anchor; anchor should shift from line 4 → 6.
	modified := "line1\nline2\nlineNEW_A\nlineNEW_B\nline3\nanchor\nline5\nline6\nline7\n"
	if err := os.WriteFile(path, []byte(modified), 0o644); err != nil {
		t.Fatalf("write modified file: %v", err)
	}

	// Poll until the thread's anchor is updated or timeout.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		la, ok := fs.threads["t1"].Anchor.(store.LineAnchor)
		if ok && la.StartLine == 6 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	cancel()
	<-done

	la, ok := fs.threads["t1"].Anchor.(store.LineAnchor)
	if !ok {
		t.Fatal("anchor is not a LineAnchor after remap")
	}
	if la.StartLine != 6 || la.EndLine != 6 {
		t.Errorf("anchor = %d–%d, want 6–6", la.StartLine, la.EndLine)
	}
	if fs.threads["t1"].AnchorStatus != store.AnchorActive {
		t.Errorf("status = %q, want active", fs.threads["t1"].AnchorStatus)
	}
}

func TestWatcherOrphansOnFileDeletion(t *testing.T) {
	dir := t.TempDir()

	content := "line1\nanchor\nline3\n"
	path := filepath.Join(dir, "gone.go")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	fs := newFakeStore()
	snap := &store.FileSnapshot{ID: "s1", RepoID: "r1", Path: "gone.go", ContentHash: contentHash([]byte(content))}
	fs.addSnapshot(snap)
	thread := &store.Thread{
		ID: "t1", FileSnapshotID: "s1", AnchorStatus: store.AnchorActive,
		Anchor: store.LineAnchor{StartLine: 2, EndLine: 2, ContextBefore: []string{"line1"}, ContextAfter: []string{"line3"}},
	}
	fs.addThread(thread)

	w, err := New(dir, "r1", store.ConnectorLocal, fs)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx, ready) }()

	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("watcher did not become ready in time")
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if fs.threads["t1"].AnchorStatus == store.AnchorOrphaned {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	cancel()
	<-done

	if fs.threads["t1"].AnchorStatus != store.AnchorOrphaned {
		t.Errorf("status = %q, want orphaned", fs.threads["t1"].AnchorStatus)
	}
}
