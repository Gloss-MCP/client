package delta

import (
	"bytes"
	"context"
	"encoding/csv"
	"strings"

	"github.com/gloss-mcp/client/internal/store"
)

// RemapCell relocates a cell anchor in newContent. The row is found with
// the same fuzzy context match line anchors use (Remap); the column is
// then re-resolved by header text, so inserting or reordering columns does
// not silently move a thread onto a different column's data. When the
// anchored column's header no longer exists the anchor cannot be placed
// and ok is false (the caller orphans the thread).
func RemapCell(anchor store.CellAnchor, newContent []byte) (store.CellAnchor, bool) {
	lines := SplitLines(newContent)
	moved, ok := Remap(cellLines(anchor), lines)
	if !ok {
		// Adding or removing a column rewrites every line, defeating
		// exact matching; retry comparing rows by their fields.
		moved, ok = remapWith(cellLines(anchor), lines, sameRecord)
	}
	if !ok {
		return store.CellAnchor{}, false
	}
	return ResolveCellColumn(anchor, moved, newContent)
}

// sameRecord reports whether two csv lines are the same row modulo
// columns being added or removed: one line's fields are all present in
// the other's. Blank lines only match blank lines.
func sameRecord(a, b string) bool {
	fa, fb := recordFields(a), recordFields(b)
	if len(fa) == 0 || len(fb) == 0 {
		return len(fa) == len(fb)
	}
	return fieldsSubset(fa, fb) || fieldsSubset(fb, fa)
}

func recordFields(line string) map[string]int {
	r := csv.NewReader(strings.NewReader(line))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	rec, err := r.Read()
	if err != nil {
		return nil
	}
	m := make(map[string]int, len(rec))
	for _, f := range rec {
		m[strings.TrimSpace(f)]++
	}
	return m
}

// fieldsSubset reports whether every field (with multiplicity) of sub
// appears in sup.
func fieldsSubset(sub, sup map[string]int) bool {
	for f, n := range sub {
		if sup[f] < n {
			return false
		}
	}
	return true
}

// RemapCellViaGitDiff is the git-diff fallback for cell anchors: it
// translates the row with RemapViaGitDiff, then re-resolves the column.
func RemapCellViaGitDiff(ctx context.Context, root, path, oldSHA string, anchor store.CellAnchor, newContent []byte) (store.CellAnchor, bool) {
	moved, ok := RemapViaGitDiff(ctx, root, path, oldSHA, cellLines(anchor))
	if !ok {
		return store.CellAnchor{}, false
	}
	return ResolveCellColumn(anchor, moved, newContent)
}

// ResolveCellColumn combines a row that has been relocated (moved) with
// the original anchor's column, re-resolving the column against the
// header row of content. It returns the updated anchor, or false when the
// column's header is gone.
func ResolveCellColumn(orig store.CellAnchor, moved store.LineAnchor, content []byte) (store.CellAnchor, bool) {
	col := orig.Col
	headers := CSVHeader(content)
	if orig.Header != "" {
		switch {
		case col >= 1 && col <= len(headers) && headers[col-1] == orig.Header:
			// Column unchanged.
		default:
			col = 0
			for i, h := range headers {
				if h == orig.Header {
					col = i + 1
					break
				}
			}
			if col == 0 {
				return store.CellAnchor{}, false
			}
		}
	}
	return store.CellAnchor{
		StartLine:     moved.StartLine,
		EndLine:       moved.EndLine,
		Col:           col,
		Header:        orig.Header,
		ContextBefore: moved.ContextBefore,
		ContextAfter:  moved.ContextAfter,
	}, true
}

// CSVHeader returns the fields of the first record of CSV content (the
// header row), tolerating a UTF-8 BOM and ragged or badly-quoted input.
// It returns nil when there is no parseable first record.
func CSVHeader(content []byte) []string {
	content = bytes.TrimPrefix(content, []byte("\xef\xbb\xbf"))
	r := csv.NewReader(bytes.NewReader(content))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	rec, err := r.Read()
	if err != nil {
		return nil
	}
	for i := range rec {
		rec[i] = strings.TrimSpace(rec[i])
	}
	return rec
}

// HeaderFor returns the header text of the 1-based column col, or "" if
// the file has no such header.
func HeaderFor(content []byte, col int) string {
	headers := CSVHeader(content)
	if col < 1 || col > len(headers) {
		return ""
	}
	return headers[col-1]
}

func cellLines(a store.CellAnchor) store.LineAnchor {
	return store.LineAnchor{
		StartLine:     a.StartLine,
		EndLine:       a.EndLine,
		ContextBefore: a.ContextBefore,
		ContextAfter:  a.ContextAfter,
	}
}
