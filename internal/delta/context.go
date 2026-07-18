// Package delta handles anchor drift: capturing context lines when a
// thread is created, and re-matching those lines after file edits to keep
// threads anchored or to orphan them when their location is gone.
package delta

// ContextLines is the number of surrounding lines captured above and
// below a line anchor. Three lines matches the standard unified-diff
// context window and gives the remapper enough signal to distinguish
// nearby identical spans.
const ContextLines = 3

// ExtractContext returns up to n lines immediately before startLine and
// immediately after endLine (both 1-based, inclusive). The slices are
// clipped at the file boundaries without error — a file that is too short
// to provide n lines of context returns whatever is available.
//
// lines is the file split on newlines; startLine and endLine are
// 1-based line numbers within that slice.
func ExtractContext(lines []string, startLine, endLine, n int) (before, after []string) {
	total := len(lines)
	if total == 0 || n <= 0 {
		return nil, nil
	}

	// before: up to n lines before startLine (1-based → 0-based: startLine-1)
	beforeEnd := startLine - 1 // exclusive upper bound (0-based)
	beforeStart := beforeEnd - n
	if beforeStart < 0 {
		beforeStart = 0
	}
	if beforeEnd > 0 && beforeStart < beforeEnd {
		before = make([]string, beforeEnd-beforeStart)
		copy(before, lines[beforeStart:beforeEnd])
	}

	// after: up to n lines after endLine (1-based → 0-based: endLine)
	afterStart := endLine // 0-based index of first line after the anchor
	afterEnd := afterStart + n
	if afterEnd > total {
		afterEnd = total
	}
	if afterStart < total && afterStart < afterEnd {
		after = make([]string, afterEnd-afterStart)
		copy(after, lines[afterStart:afterEnd])
	}

	return before, after
}
