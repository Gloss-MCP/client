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

func (f *fakeStore) ListActiveThreadsForPath(_ context.Context, _, path string) ([]*store.Thread, error) {
	var out []*store.Thread
	for _, t := range f.threads {
		if t.AnchorStatus != store.AnchorActive {
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

func TestWatcherRemapsCellAnchorWhenColumnInserted(t *testing.T) {
	dir := t.TempDir()
	initial := csvOriginal
	path := filepath.Join(dir, "prices.csv")
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	fs := newFakeStore()
	fs.addSnapshot(&store.FileSnapshot{ID: "snap1", RepoID: "repo1", Path: "prices.csv", ContentHash: contentHash([]byte(initial))})
	fs.addThread(&store.Thread{
		ID: "t1", FileSnapshotID: "snap1", AnchorStatus: store.AnchorActive,
		Anchor: cellAt(initial, 3, 3), // banana's price
	})

	w, err := New(dir, "repo1", store.ConnectorLocal, fs)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx, ready) }()
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("watcher did not become ready")
	}

	// Insert a sku column before price and a new row at the top.
	modified := "id,name,sku,price,qty\n0,zero,z0,0.00,0\n1,apple,a1,0.50,10\n2,banana,b2,0.25,20\n3,cherry,c3,4.00,5\n4,date,d4,3.00,7\n5,elder,e5,9.99,1\n"
	if err := os.WriteFile(path, []byte(modified), 0o644); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ca, ok := fs.threads["t1"].Anchor.(store.CellAnchor); ok && ca.StartLine == 4 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	<-done

	ca, ok := fs.threads["t1"].Anchor.(store.CellAnchor)
	if !ok {
		t.Fatal("anchor is not a CellAnchor after remap")
	}
	if ca.StartLine != 4 || ca.Col != 4 || ca.Header != "price" {
		t.Errorf("anchor = %+v, want line 4 col 4 (price)", ca)
	}
	if fs.threads["t1"].AnchorStatus != store.AnchorActive {
		t.Errorf("status = %s, want active", fs.threads["t1"].AnchorStatus)
	}
}

func TestWatcherOrphansCellAnchorWhenColumnRemoved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prices.csv")
	if err := os.WriteFile(path, []byte(csvOriginal), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := newFakeStore()
	fs.addSnapshot(&store.FileSnapshot{ID: "snap1", RepoID: "repo1", Path: "prices.csv", ContentHash: contentHash([]byte(csvOriginal))})
	fs.addThread(&store.Thread{
		ID: "t1", FileSnapshotID: "snap1", AnchorStatus: store.AnchorActive,
		Anchor: cellAt(csvOriginal, 3, 3),
	})

	w, err := New(dir, "repo1", store.ConnectorLocal, fs)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx, ready) }()
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("watcher did not become ready")
	}

	if err := os.WriteFile(path, []byte("id,name,qty\n1,apple,10\n2,banana,20\n3,cherry,5\n4,date,7\n5,elder,1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && fs.threads["t1"].AnchorStatus != store.AnchorOrphaned {
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	<-done
	if fs.threads["t1"].AnchorStatus != store.AnchorOrphaned {
		t.Errorf("status = %s, want orphaned (price column removed)", fs.threads["t1"].AnchorStatus)
	}
}

// TestWatcherOrphansMediaAnchorsOnContentChange: region/time anchors
// can't be re-located in a rewritten binary file, so any content change
// orphans them, while a write that leaves the content as anchored (same
// hash) keeps them active.
func TestWatcherOrphansMediaAnchorsOnContentChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shot.png")
	original := []byte("\x89PNG-original")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	fs := newFakeStore()
	fs.addSnapshot(&store.FileSnapshot{ID: "s1", RepoID: "r1", Path: "shot.png", ContentHash: contentHash(original)})
	fs.addThread(&store.Thread{ID: "region", FileSnapshotID: "s1", AnchorStatus: store.AnchorActive,
		Anchor: store.RegionAnchor{X: 0.1, Y: 0.1, Width: 0.2, Height: 0.2}})
	fs.addThread(&store.Thread{ID: "time", FileSnapshotID: "s1", AnchorStatus: store.AnchorActive,
		Anchor: store.TimeAnchor{StartTime: 1, EndTime: 2}})
	fs.addThread(&store.Thread{ID: "rt", FileSnapshotID: "s1", AnchorStatus: store.AnchorActive,
		Anchor: store.RegionTimeAnchor{Width: 0.5, Height: 0.5, StartTime: 1, EndTime: 2}})

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

	// Rewriting identical bytes changes nothing.
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	time.Sleep(debounceDelay + 300*time.Millisecond)
	for id, th := range fs.threads {
		if th.AnchorStatus != store.AnchorActive {
			t.Fatalf("thread %s = %q after identical rewrite, want active", id, th.AnchorStatus)
		}
	}

	if err := os.WriteFile(path, []byte("\x89PNG-edited"), 0o644); err != nil {
		t.Fatalf("edit: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if fs.threads["region"].AnchorStatus == store.AnchorOrphaned &&
			fs.threads["time"].AnchorStatus == store.AnchorOrphaned &&
			fs.threads["rt"].AnchorStatus == store.AnchorOrphaned {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	<-done

	for id, th := range fs.threads {
		if th.AnchorStatus != store.AnchorOrphaned {
			t.Errorf("thread %s = %q, want orphaned", id, th.AnchorStatus)
		}
	}
}
