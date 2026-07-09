package connector

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/gloss-mcp/client/internal/store"
)

// snapshot is the shared walk+hash+persist core behind both connector
// types. useGitIgnore controls whether root/.gitignore is loaded in
// addition to root/.glossignore; commitSHA (may be empty) is attached to
// every FileSnapshot created during this run.
func snapshot(ctx context.Context, st *store.Store, repoID, root string, useGitIgnore bool, commitSHA string) (Result, error) {
	result := Result{Root: root}

	m, err := loadIgnoreMatcher(root, useGitIgnore)
	if err != nil {
		return result, fmt.Errorf("connector: load ignore rules: %w", err)
	}

	entries, skipped, walkErrs := walkTree(root, m)
	result.Skipped += skipped
	result.Errors = append(result.Errors, walkErrs...)

	for _, entry := range entries {
		hash, err := hashFile(entry.absPath)
		if err != nil {
			result.Skipped++
			result.Errors = append(result.Errors, fmt.Errorf("hash %s: %w", entry.relPath, err))
			continue
		}
		result.Files++

		_, err = st.FindFileSnapshot(ctx, repoID, entry.relPath, hash)
		switch {
		case err == nil:
			result.Reused++
			continue
		case errors.Is(err, store.ErrNotFound):
			// fall through to create
		default:
			return result, fmt.Errorf("connector: find snapshot for %s: %w", entry.relPath, err)
		}

		if _, err := st.CreateFileSnapshot(ctx, repoID, entry.relPath, hash, commitSHA); err != nil {
			return result, fmt.Errorf("connector: create snapshot for %s: %w", entry.relPath, err)
		}
		result.Created++
	}

	return result, nil
}

// loadIgnoreMatcher concatenates .gitignore rules (only when
// useGitIgnore) before .glossignore rules, both read from root, in that
// fixed order -- so .glossignore can override a .gitignore exclusion via
// "!negate", never the reverse.
func loadIgnoreMatcher(root string, useGitIgnore bool) (*ignoreMatcher, error) {
	var rules []ignoreRule
	if useGitIgnore {
		gitRules, err := loadIgnoreFile(filepath.Join(root, ".gitignore"))
		if err != nil {
			return nil, err
		}
		rules = append(rules, gitRules...)
	}
	glossRules, err := loadIgnoreFile(filepath.Join(root, ".glossignore"))
	if err != nil {
		return nil, err
	}
	rules = append(rules, glossRules...)
	return &ignoreMatcher{rules: rules}, nil
}
