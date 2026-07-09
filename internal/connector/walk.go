package connector

import (
	"io/fs"
	"path/filepath"
)

// fileEntry is one tracked file found by walkTree.
type fileEntry struct {
	relPath string // slash-separated, relative to root
	absPath string
}

// walkTree walks root depth-first, applying m (nil-safe) as an ignore
// filter. It always prunes directories named ".git" and ".gloss" --
// housekeeping, not user-configurable, regardless of ignore-file
// contents -- and never follows symlinks (they are counted in skipped,
// never hashed). Per-entry errors (permission denied, etc.) are
// collected rather than aborting the walk; a directory that errors is
// skipped in its entirety.
func walkTree(root string, m *ignoreMatcher) (entries []fileEntry, skipped int, errs []error) {
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if path == root {
			if err != nil {
				errs = append(errs, err)
				return err
			}
			return nil
		}

		if err != nil {
			errs = append(errs, err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		if d.IsDir() && (d.Name() == ".git" || d.Name() == ".gloss") {
			return fs.SkipDir
		}

		if d.Type()&fs.ModeSymlink != 0 {
			skipped++
			return nil
		}

		relPath, relErr := filepath.Rel(root, path)
		if relErr != nil {
			errs = append(errs, relErr)
			return nil
		}
		relPath = filepath.ToSlash(relPath)

		if m.match(relPath, d.IsDir()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			return nil
		}

		entries = append(entries, fileEntry{relPath: relPath, absPath: path})
		return nil
	})
	if walkErr != nil {
		errs = append(errs, walkErr)
	}
	return entries, skipped, errs
}
