package delta

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/gloss-mcp/client/internal/connector"
	"github.com/gloss-mcp/client/internal/store"
)

// watchStore is the store surface the watcher needs. Using an interface
// keeps the delta package testable without a real SQLite store.
type watchStore interface {
	ListActiveLineThreadsForPath(ctx context.Context, repoID, path string) ([]*store.Thread, error)
	UpdateThreadAnchor(ctx context.Context, id string, anchor store.Anchor) (*store.Thread, error)
	SetAnchorStatus(ctx context.Context, id string, status store.AnchorStatus) (*store.Thread, error)
	GetFileSnapshot(ctx context.Context, id string) (*store.FileSnapshot, error)
}

// debounceDelay is how long the watcher waits after the last write event
// before processing a file change — editors burst writes, so we coalesce.
const debounceDelay = 150 * time.Millisecond

// Watcher monitors the repo for file changes and remaps or orphans
// affected line-anchored threads.
type Watcher struct {
	root     string
	repoID   string
	connType store.ConnectorType
	st       watchStore

	fw     *fsnotify.Watcher
	mu     sync.Mutex
	timers map[string]*time.Timer // debounce timers keyed by repo-relative path
}

// New creates a Watcher. Call Run in a goroutine to start it. Returns
// (nil, nil) if fsnotify is unavailable — the caller should treat that as
// best-effort and not fatal.
func New(root, repoID string, connType store.ConnectorType, st watchStore) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{
		root:     root,
		repoID:   repoID,
		connType: connType,
		st:       st,
		fw:       fw,
		timers:   make(map[string]*time.Timer),
	}, nil
}

// Run starts the event loop, adding watched directories from the current
// tracked file set and then processing events until ctx is canceled.
// If ready is non-nil it is closed once watches are registered so callers
// can wait before triggering file events (useful in tests).
func (w *Watcher) Run(ctx context.Context, ready chan<- struct{}) error {
	if err := w.watchDirs(); err != nil {
		_ = w.fw.Close()
		return err
	}
	if ready != nil {
		close(ready)
	}

	defer func() {
		w.mu.Lock()
		for _, t := range w.timers {
			t.Stop()
		}
		w.mu.Unlock()
		_ = w.fw.Close()
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case event, ok := <-w.fw.Events:
			if !ok {
				return nil
			}
			w.handleEvent(ctx, event)

		case _, ok := <-w.fw.Errors:
			if !ok {
				return nil
			}
			// fsnotify errors are non-fatal; continue watching.
		}
	}
}

// watchDirs walks the tracked file set and adds each unique directory to
// the fsnotify watcher. fsnotify on Linux watches a directory (not
// recursively), so every directory containing tracked files must be
// registered.
func (w *Watcher) watchDirs() error {
	dirs := map[string]bool{w.root: true}

	paths, err := connector.ListFiles(w.root, w.connType)
	if err != nil {
		return err
	}
	for _, p := range paths {
		abs := filepath.Join(w.root, filepath.FromSlash(p))
		dirs[filepath.Dir(abs)] = true
	}

	for dir := range dirs {
		if err := w.fw.Add(dir); err != nil {
			// Best-effort: a directory may have been deleted between the
			// listing and here; skip it silently.
			continue
		}
	}
	return nil
}

// handleEvent dispatches a single fsnotify event.
func (w *Watcher) handleEvent(ctx context.Context, event fsnotify.Event) {
	absPath := event.Name
	info, statErr := os.Stat(absPath)

	// New directory created: add it to the watcher so files inside it
	// are also tracked.
	if event.Has(fsnotify.Create) && statErr == nil && info.IsDir() {
		_ = w.fw.Add(absPath)
		return
	}

	// Derive the repo-relative slash path.
	rel, err := filepath.Rel(w.root, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return
	}
	relSlash := filepath.ToSlash(rel)

	switch {
	case event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename):
		// Immediate orphan — no debounce needed since the file is gone.
		go w.orphanFile(ctx, relSlash)

	case event.Has(fsnotify.Write) || event.Has(fsnotify.Create):
		if statErr != nil || info.IsDir() {
			return
		}
		w.debounce(ctx, relSlash)
	}
}

// debounce schedules remapFile after debounceDelay, resetting the timer if
// another event arrives for the same path first.
func (w *Watcher) debounce(ctx context.Context, relPath string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if t, ok := w.timers[relPath]; ok {
		t.Stop()
	}
	w.timers[relPath] = time.AfterFunc(debounceDelay, func() {
		w.mu.Lock()
		delete(w.timers, relPath)
		w.mu.Unlock()
		w.remapFile(ctx, relPath)
	})
}

// remapFile reads the current file content and, for each active
// line-anchored thread anchored to that file, attempts to remap the
// anchor. Threads that cannot be remapped are orphaned.
func (w *Watcher) remapFile(ctx context.Context, relPath string) {
	absPath := filepath.Join(w.root, filepath.FromSlash(relPath))
	content, err := os.ReadFile(absPath)
	if err != nil {
		w.orphanFile(ctx, relPath)
		return
	}

	newLines := SplitLines(content)
	newHash := contentHash(content)

	threads, err := w.st.ListActiveLineThreadsForPath(ctx, w.repoID, relPath)
	if err != nil || len(threads) == 0 {
		return
	}

	// Sort threads oldest-first so remap is deterministic.
	sort.Slice(threads, func(i, j int) bool {
		return threads[i].CreatedAt.Before(threads[j].CreatedAt)
	})

	for _, t := range threads {
		la, ok := t.Anchor.(store.LineAnchor)
		if !ok {
			continue
		}

		// Skip if the file content matches the snapshot this thread was
		// anchored to — nothing has changed.
		snap, err := w.st.GetFileSnapshot(ctx, t.FileSnapshotID)
		if err == nil && snap.ContentHash == newHash {
			continue
		}

		// Try fuzzy context match first.
		remapped, ok := Remap(la, newLines)
		if ok {
			_, _ = w.st.UpdateThreadAnchor(ctx, t.ID, remapped)
			continue
		}

		// Fall back to git diff if the snapshot has a commit SHA.
		var gitSHA string
		if snap != nil {
			gitSHA = snap.GitCommitSHA
		}
		if gitSHA != "" {
			remapped, ok = RemapViaGitDiff(ctx, w.root, relPath, gitSHA, la)
			if ok {
				_, _ = w.st.UpdateThreadAnchor(ctx, t.ID, remapped)
				continue
			}
		}

		// Could not remap — orphan.
		_, _ = w.st.SetAnchorStatus(ctx, t.ID, store.AnchorOrphaned)
	}
}

// orphanFile marks all active line-anchored threads for relPath as orphaned.
func (w *Watcher) orphanFile(ctx context.Context, relPath string) {
	threads, err := w.st.ListActiveLineThreadsForPath(ctx, w.repoID, relPath)
	if err != nil {
		return
	}
	for _, t := range threads {
		_, _ = w.st.SetAnchorStatus(ctx, t.ID, store.AnchorOrphaned)
	}
}

// contentHash returns the hex SHA-256 of content — matches the hashing
// used by the connector and server snapshot helpers.
func contentHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
