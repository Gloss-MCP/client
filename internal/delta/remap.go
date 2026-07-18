package delta

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gloss-mcp/client/internal/store"
)

// Remap attempts to find where anchor's lines have moved in newLines.
//
// It scores every candidate position in newLines by counting how many of
// the anchor's stored context lines still match (trimmed). The candidate
// with the highest score wins if it meets the majority threshold
// (> half of all available context lines). On success the returned anchor
// carries freshly extracted context from the new position so future
// remaps have up-to-date signal.
//
// Returns (remapped, true) on success, or (zero, false) when the anchor
// cannot be located with sufficient confidence.
func Remap(anchor store.LineAnchor, newLines []string) (store.LineAnchor, bool) {
	totalCtx := len(anchor.ContextBefore) + len(anchor.ContextAfter)
	if totalCtx == 0 {
		return store.LineAnchor{}, false
	}

	anchorLen := anchor.EndLine - anchor.StartLine + 1
	if anchorLen < 1 {
		anchorLen = 1
	}

	required := (totalCtx + 1) / 2 // ceiling division — majority required

	bestScore, bestPos := -1, -1
	for pos := 0; pos <= len(newLines)-anchorLen; pos++ {
		score := scoreCandidate(anchor, newLines, pos, anchorLen)
		if score > bestScore {
			bestScore, bestPos = score, pos
		}
	}

	if bestScore < required {
		return store.LineAnchor{}, false
	}

	newStart := bestPos + 1 // convert to 1-based
	newEnd := newStart + anchorLen - 1

	// Re-extract context from the new position so the next remap has
	// fresh signal rather than the original (now-stale) context.
	freshBefore, freshAfter := ExtractContext(newLines, newStart, newEnd, ContextLines)

	return store.LineAnchor{
		StartLine:     newStart,
		EndLine:       newEnd,
		ContextBefore: freshBefore,
		ContextAfter:  freshAfter,
	}, true
}

// scoreCandidate returns the number of context lines (before and after)
// that match at the given 0-based candidate position in newLines.
func scoreCandidate(anchor store.LineAnchor, lines []string, pos, anchorLen int) int {
	score := 0

	// ContextBefore: lines immediately above pos
	nb := len(anchor.ContextBefore)
	for j, ctx := range anchor.ContextBefore {
		lineIdx := pos - nb + j
		if lineIdx >= 0 && lineIdx < len(lines) && normalize(lines[lineIdx]) == normalize(ctx) {
			score++
		}
	}

	// ContextAfter: lines immediately below pos+anchorLen
	afterStart := pos + anchorLen
	for j, ctx := range anchor.ContextAfter {
		lineIdx := afterStart + j
		if lineIdx < len(lines) && normalize(lines[lineIdx]) == normalize(ctx) {
			score++
		}
	}

	return score
}

// normalize strips leading and trailing whitespace for a whitespace-
// insensitive comparison — handles indent-only edits without false orphans.
func normalize(s string) string { return strings.TrimSpace(s) }

// hunkRE matches unified diff hunk headers: @@ -old,oldLen +new,newLen @@
var hunkRE = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// RemapViaGitDiff is a fallback that translates anchor line numbers using
// the unified diff between oldSHA and the current HEAD for path. It is
// called after fuzzy remap fails and is only attempted when both oldSHA
// and HEAD can be resolved.
//
// Returns (remapped, true) on success, (zero, false) on any failure
// (git unavailable, no diff, anchor already in a deleted hunk).
func RemapViaGitDiff(ctx context.Context, root, path, oldSHA string, anchor store.LineAnchor) (store.LineAnchor, bool) {
	if oldSHA == "" {
		return store.LineAnchor{}, false
	}

	headSHA, err := headCommitSHA(ctx, root)
	if err != nil || headSHA == "" || headSHA == oldSHA {
		return store.LineAnchor{}, false
	}

	relPath := filepath.ToSlash(path)
	cmd := exec.CommandContext(ctx, "git", "diff", oldSHA, "HEAD", "--", relPath)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return store.LineAnchor{}, false
	}

	offset, deleted := diffOffset(out, anchor.StartLine)
	if deleted {
		return store.LineAnchor{}, false
	}

	newStart := anchor.StartLine + offset
	newEnd := anchor.EndLine + offset
	if newStart < 1 {
		return store.LineAnchor{}, false
	}

	return store.LineAnchor{
		StartLine:     newStart,
		EndLine:       newEnd,
		ContextBefore: anchor.ContextBefore,
		ContextAfter:  anchor.ContextAfter,
	}, true
}

// diffOffset parses a unified diff and returns the cumulative line-number
// offset that applies to oldLine in the new file, and whether oldLine
// falls inside a deleted section.
//
// For anchors inside a hunk it walks the hunk body line by line to count
// how many insertions precede oldLine, giving a precise mapping. Anchors
// outside any hunk accumulate the net additions/deletions of preceding
// hunks.
func diffOffset(diff []byte, oldLine int) (offset int, deleted bool) {
	scanner := bufio.NewScanner(bytes.NewReader(diff))
	cumOffset := 0

	var (
		inHunk   bool
		oldCur   int // current old-file line pointer inside the hunk
		newCur   int // current new-file line pointer inside the hunk
		oldStart int
		newStart int
		oldEnd   int
	)

	flushHunk := func() {
		if inHunk {
			// Hunk ended above oldLine — accumulate its net delta.
			cumOffset += (newCur - newStart) - (oldCur - oldStart)
			inHunk = false
		}
	}

	for scanner.Scan() {
		line := scanner.Text()

		if m := hunkRE.FindStringSubmatch(line); m != nil {
			flushHunk()

			oldStart, _ = strconv.Atoi(m[1])
			oldLen := 1
			if m[2] != "" {
				oldLen, _ = strconv.Atoi(m[2])
			}
			newStart, _ = strconv.Atoi(m[3])

			oldEnd = oldStart + oldLen - 1
			oldCur = oldStart
			newCur = newStart
			inHunk = true

			if oldStart > oldLine {
				// This hunk and all following are entirely below oldLine.
				inHunk = false
				break
			}
			continue
		}

		if !inHunk {
			continue
		}

		// Walk hunk body.
		switch {
		case strings.HasPrefix(line, "+"):
			// Insertion: no old-file line consumed, new-file line added.
			newCur++
		case strings.HasPrefix(line, "-"):
			// Deletion: old-file line removed.
			if oldCur == oldLine {
				return 0, true // anchor line was deleted
			}
			oldCur++
		default:
			// Context line: present in both old and new.
			if oldCur == oldLine {
				// Found the anchor's exact line — compute its new position.
				return cumOffset + (newCur - oldCur), false
			}
			oldCur++
			newCur++
		}

		if oldCur > oldEnd && oldLine > oldEnd {
			// Finished the hunk and anchor is beyond it.
			cumOffset += (newCur - newStart) - (oldCur - oldStart)
			inHunk = false
		}
	}

	if inHunk && oldLine <= oldEnd {
		// Anchor was inside the last hunk but we ran out of body lines —
		// treat as ambiguous and return the hunk's start offset.
		return cumOffset + (newStart - oldStart), false
	}

	return cumOffset, false
}

// headCommitSHA returns the current HEAD commit SHA by shelling out to
// git. Returns ("", err) if git is unavailable or the repo has no commits.
func headCommitSHA(ctx context.Context, root string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// SplitLines splits content into lines, stripping the trailing newline if
// present. It is the canonical way to prepare file bytes for Remap.
func SplitLines(content []byte) []string {
	s := string(content)
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
