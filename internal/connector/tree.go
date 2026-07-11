package connector

import (
	"fmt"
	"sort"

	"github.com/gloss-mcp/client/internal/store"
)

// ListFiles returns the sorted, slash-separated relative paths of root's
// tracked files for connectorType -- the same set (and ignore rules) that
// Snapshot uses, so callers such as the file-browser UI show exactly the
// files that can be annotated. Per-entry walk errors (permission denied,
// etc.) are non-fatal, matching Snapshot's tolerance: affected files/dirs
// are simply absent from the result.
func ListFiles(root string, connectorType store.ConnectorType) ([]string, error) {
	m, err := loadIgnoreMatcher(root, connectorType == store.ConnectorGit)
	if err != nil {
		return nil, fmt.Errorf("connector: load ignore rules: %w", err)
	}

	entries, _, _ := walkTree(root, m)

	paths := make([]string, len(entries))
	for i, e := range entries {
		paths[i] = e.relPath
	}
	sort.Strings(paths)
	return paths, nil
}
