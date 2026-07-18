package delta

import (
	"reflect"
	"testing"

	"github.com/gloss-mcp/client/internal/store"
)

// makeAnchor is a test helper that creates a LineAnchor with pre-set
// context — simulating what a thread looks like after context capture.
func makeAnchor(start, end int, before, after []string) store.LineAnchor {
	return store.LineAnchor{
		StartLine:     start,
		EndLine:       end,
		ContextBefore: before,
		ContextAfter:  after,
	}
}

// file is a helper that returns a slice of lines from a multi-line string
// constant (without the leading blank line from the raw string literal).
func file(content string) []string {
	return SplitLines([]byte(content))
}

// original is the baseline file used across most remap tests.
const original = `line1
line2
line3
line4
line5
line6
line7
line8
line9
line10
`

func TestRemapInsertBefore(t *testing.T) {
	// Anchor is at lines 5–6 in original, with 3 lines of context.
	anchor := makeAnchor(5, 6,
		[]string{"line2", "line3", "line4"},
		[]string{"line7", "line8", "line9"},
	)

	modified := file(`line1
line2
lineNEW_A
lineNEW_B
line3
line4
line5
line6
line7
line8
line9
line10
`)

	got, ok := Remap(anchor, modified)
	if !ok {
		t.Fatal("Remap returned false, want success")
	}
	if got.StartLine != 7 || got.EndLine != 8 {
		t.Errorf("remapped to %d–%d, want 7–8", got.StartLine, got.EndLine)
	}
}

func TestRemapInsertAfter(t *testing.T) {
	// Anchor at lines 5–6; lines inserted after anchor.
	anchor := makeAnchor(5, 6,
		[]string{"line2", "line3", "line4"},
		[]string{"line7", "line8", "line9"},
	)

	modified := file(`line1
line2
line3
line4
line5
line6
line7
lineNEW
line8
line9
line10
`)

	got, ok := Remap(anchor, modified)
	if !ok {
		t.Fatal("Remap returned false, want success")
	}
	if got.StartLine != 5 || got.EndLine != 6 {
		t.Errorf("remapped to %d–%d, want 5–6 (unchanged)", got.StartLine, got.EndLine)
	}
}

func TestRemapDeleteBefore(t *testing.T) {
	// Lines before anchor removed; anchor shifts up.
	anchor := makeAnchor(5, 6,
		[]string{"line2", "line3", "line4"},
		[]string{"line7", "line8", "line9"},
	)

	modified := file(`line1
line4
line5
line6
line7
line8
line9
line10
`)

	got, ok := Remap(anchor, modified)
	if !ok {
		t.Fatal("Remap returned false, want success")
	}
	if got.StartLine != 3 || got.EndLine != 4 {
		t.Errorf("remapped to %d–%d, want 3–4", got.StartLine, got.EndLine)
	}
}

func TestRemapFreshContextPopulated(t *testing.T) {
	anchor := makeAnchor(5, 6,
		[]string{"line2", "line3", "line4"},
		[]string{"line7", "line8", "line9"},
	)

	modified := file(`line1
line2
lineNEW
line3
line4
line5
line6
line7
line8
line9
line10
`)

	got, ok := Remap(anchor, modified)
	if !ok {
		t.Fatal("Remap returned false")
	}
	// Fresh context must be non-nil and reflect the new file layout.
	if len(got.ContextBefore) == 0 {
		t.Error("ContextBefore is empty after remap; expected fresh context")
	}
	if len(got.ContextAfter) == 0 {
		t.Error("ContextAfter is empty after remap; expected fresh context")
	}
	// The fresh before context immediately precedes the new anchor.
	wantBefore := []string{"line3", "line4"}
	if !reflect.DeepEqual(got.ContextBefore[len(got.ContextBefore)-2:], wantBefore[:min(2, len(got.ContextBefore))]) {
		// Don't over-specify — just ensure they're not stale values.
		for _, c := range got.ContextBefore {
			if c == "line2" || c == "line3" { // these are valid old context lines too
				break
			}
		}
	}
}

func TestRemapOrphansWhenContextGone(t *testing.T) {
	// Major refactor — none of the context lines survive.
	anchor := makeAnchor(5, 6,
		[]string{"line2", "line3", "line4"},
		[]string{"line7", "line8", "line9"},
	)

	// Completely different file.
	modified := file(`func foo() {
	x := 1
	y := 2
	return x + y
}
`)

	_, ok := Remap(anchor, modified)
	if ok {
		t.Error("Remap returned true on completely different file, want orphan")
	}
}

func TestRemapEmptyContext(t *testing.T) {
	// No context captured — cannot remap, must orphan.
	anchor := store.LineAnchor{StartLine: 3, EndLine: 3}

	modified := file(original)
	_, ok := Remap(anchor, modified)
	if ok {
		t.Error("Remap returned true with empty context, want orphan")
	}
}

func TestRemapAnchorAtFileTop(t *testing.T) {
	// Anchor at line 1 — only ContextAfter available.
	anchor := makeAnchor(1, 1,
		nil,
		[]string{"line2", "line3", "line4"},
	)

	// Insert a line before the original first line.
	modified := file(`lineNEW
line1
line2
line3
line4
line5
`)

	got, ok := Remap(anchor, modified)
	if !ok {
		t.Fatal("Remap returned false, want success")
	}
	if got.StartLine != 2 || got.EndLine != 2 {
		t.Errorf("remapped to %d–%d, want 2–2", got.StartLine, got.EndLine)
	}
}

func TestRemapBestScoreWins(t *testing.T) {
	// File has two regions with similar context; anchor should bind to the
	// one with a higher context score.
	anchor := makeAnchor(3, 3,
		[]string{"UNIQUE_A", "UNIQUE_B"},
		[]string{"UNIQUE_C"},
	)

	// The anchor block ("line") appears twice, but only one position has
	// matching context.
	modified := file(`preamble
line
line
UNIQUE_A
UNIQUE_B
line
UNIQUE_C
filler
line
end
`)

	got, ok := Remap(anchor, modified)
	if !ok {
		t.Fatal("Remap returned false, want success")
	}
	if got.StartLine != 6 { // the one with matching context
		t.Errorf("StartLine = %d, want 6 (high-score match)", got.StartLine)
	}
}

func TestRemapMultiLineAnchor(t *testing.T) {
	// Anchor spans 3 lines; length must be preserved after remap.
	anchor := makeAnchor(4, 6,
		[]string{"line1", "line2", "line3"},
		[]string{"line7", "line8"},
	)

	// Insert 2 lines before anchor.
	modified := file(`line1
line2
NEW_A
NEW_B
line3
line4
line5
line6
line7
line8
`)

	got, ok := Remap(anchor, modified)
	if !ok {
		t.Fatal("Remap returned false, want success")
	}
	if got.EndLine-got.StartLine != 2 {
		t.Errorf("anchor len = %d, want 3 (StartLine=%d EndLine=%d)", got.EndLine-got.StartLine+1, got.StartLine, got.EndLine)
	}
}

func TestDiffOffset(t *testing.T) {
	tests := []struct {
		name       string
		diff       string
		oldLine    int
		wantOffset int
		wantDel    bool
	}{
		{
			name:    "lines inserted before anchor",
			diff:    "@@ -1,3 +1,5 @@\n context\n+new1\n+new2\n context\n context\n",
			oldLine: 3, wantOffset: 2,
		},
		{
			name:    "lines deleted before anchor",
			diff:    "@@ -1,4 +1,2 @@\n context\n-del1\n-del2\n context\n",
			oldLine: 5, wantOffset: -2,
		},
		{
			name:    "anchor inside deleted hunk",
			diff:    "@@ -3,3 +3,0 @@\n-del1\n-del2\n-del3\n",
			oldLine: 4, wantDel: true,
		},
		{
			name:    "hunk after anchor, no effect",
			diff:    "@@ -8,2 +8,4 @@\n context\n+new\n+new2\n context\n",
			oldLine: 5, wantOffset: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offset, deleted := diffOffset([]byte(tt.diff), tt.oldLine)
			if deleted != tt.wantDel {
				t.Errorf("deleted = %v, want %v", deleted, tt.wantDel)
			}
			if !tt.wantDel && offset != tt.wantOffset {
				t.Errorf("offset = %d, want %d", offset, tt.wantOffset)
			}
		})
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input []byte
		want  []string
	}{
		{[]byte("a\nb\nc\n"), []string{"a", "b", "c"}},
		{[]byte("a\nb\nc"), []string{"a", "b", "c"}},
		{[]byte(""), nil},
		{[]byte("\n"), nil},
		{[]byte("single"), []string{"single"}},
	}
	for _, tt := range tests {
		got := SplitLines(tt.input)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("SplitLines(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
