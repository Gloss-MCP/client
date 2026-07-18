package delta

import (
	"reflect"
	"testing"
)

func TestExtractContext(t *testing.T) {
	lines := []string{
		"a", // 1
		"b", // 2
		"c", // 3
		"d", // 4
		"e", // 5
		"f", // 6
		"g", // 7
	}

	tests := []struct {
		name       string
		start      int
		end        int
		n          int
		wantBefore []string
		wantAfter  []string
	}{
		{
			name:  "anchor in middle, full context",
			start: 4, end: 4, n: 3,
			wantBefore: []string{"a", "b", "c"},
			wantAfter:  []string{"e", "f", "g"},
		},
		{
			name:  "anchor at top, clipped before",
			start: 1, end: 1, n: 3,
			wantBefore: nil,
			wantAfter:  []string{"b", "c", "d"},
		},
		{
			name:  "anchor at bottom, clipped after",
			start: 7, end: 7, n: 3,
			wantBefore: []string{"d", "e", "f"},
			wantAfter:  nil,
		},
		{
			name:  "anchor near top, partial before",
			start: 2, end: 2, n: 3,
			wantBefore: []string{"a"},
			wantAfter:  []string{"c", "d", "e"},
		},
		{
			name:  "multi-line anchor",
			start: 3, end: 5, n: 2,
			wantBefore: []string{"a", "b"},
			wantAfter:  []string{"f", "g"},
		},
		{
			name:  "n=0, no context",
			start: 4, end: 4, n: 0,
			wantBefore: nil,
			wantAfter:  nil,
		},
		{
			name:  "empty file",
			start: 1, end: 1, n: 3,
			wantBefore: nil,
			wantAfter:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := lines
			if tt.name == "empty file" {
				src = nil
			}
			got1, got2 := ExtractContext(src, tt.start, tt.end, tt.n)
			if !reflect.DeepEqual(got1, tt.wantBefore) {
				t.Errorf("before = %v, want %v", got1, tt.wantBefore)
			}
			if !reflect.DeepEqual(got2, tt.wantAfter) {
				t.Errorf("after = %v, want %v", got2, tt.wantAfter)
			}
		})
	}
}
