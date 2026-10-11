package delta

import (
	"testing"

	"github.com/gloss-mcp/client/internal/store"
)

const csvOriginal = `id,name,price,qty
1,apple,0.50,10
2,banana,0.25,20
3,cherry,4.00,5
4,date,3.00,7
5,elder,9.99,1
`

// cellAt builds a cell anchor on a single-line record of content, the way
// the server does at thread creation.
func cellAt(content string, line, col int) store.CellAnchor {
	b := []byte(content)
	before, after := ExtractContext(SplitLines(b), line, line, ContextLines)
	return store.CellAnchor{
		StartLine: line, EndLine: line, Col: col, Header: HeaderFor(b, col),
		ContextBefore: before, ContextAfter: after,
	}
}

func TestCSVHeader(t *testing.T) {
	tests := []struct {
		name, in string
		want     []string
	}{
		{"plain", "a,b,c\n1,2,3\n", []string{"a", "b", "c"}},
		{"bom", "\xef\xbb\xbfa,b\n", []string{"a", "b"}},
		{"quoted", "\"a,1\",b\n", []string{"a,1", "b"}},
		{"spaces trimmed", " a , b \n", []string{"a", "b"}},
		{"bad quotes tolerated", "a,\"b\nx\n", nil},
		{"empty", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CSVHeader([]byte(tt.in))
			if tt.want != nil && (len(got) != len(tt.want) || got[0] != tt.want[0]) {
				t.Errorf("CSVHeader = %q, want %q", got, tt.want)
			}
		})
	}
	if HeaderFor([]byte(csvOriginal), 3) != "price" || HeaderFor([]byte(csvOriginal), 9) != "" || HeaderFor([]byte(csvOriginal), 0) != "" {
		t.Error("HeaderFor wrong")
	}
}

func TestRemapCellRowInsertedAbove(t *testing.T) {
	a := cellAt(csvOriginal, 4, 3) // cherry's price
	updated := "id,name,price,qty\n0,zero,0.00,0\n1,apple,0.50,10\n2,banana,0.25,20\n3,cherry,4.00,5\n4,date,3.00,7\n5,elder,9.99,1\n"
	got, ok := RemapCell(a, []byte(updated))
	if !ok {
		t.Fatal("remap failed")
	}
	if got.StartLine != 5 || got.EndLine != 5 || got.Col != 3 || got.Header != "price" {
		t.Errorf("got %+v, want line 5 col 3 header price", got)
	}
}

func TestRemapCellFollowsColumnByHeader(t *testing.T) {
	a := cellAt(csvOriginal, 3, 3) // banana's price, col 3
	// A "sku" column is inserted before price: price is now col 4.
	updated := "id,name,sku,price,qty\n1,apple,a1,0.50,10\n2,banana,b2,0.25,20\n3,cherry,c3,4.00,5\n4,date,d4,3.00,7\n5,elder,e5,9.99,1\n"
	got, ok := RemapCell(a, []byte(updated))
	if !ok {
		t.Fatal("remap failed")
	}
	if got.Col != 4 || got.Header != "price" || got.StartLine != 3 {
		t.Errorf("got %+v, want col 4 (price) on line 3", got)
	}
}

func TestRemapCellOrphansWhenHeaderGone(t *testing.T) {
	a := cellAt(csvOriginal, 3, 3)
	updated := "id,name,qty\n1,apple,10\n2,banana,20\n3,cherry,5\n4,date,7\n5,elder,1\n"
	if got, ok := RemapCell(a, []byte(updated)); ok {
		t.Errorf("remap succeeded (%+v) but the price column was removed", got)
	}
}

func TestRemapCellOrphansWhenRowGone(t *testing.T) {
	a := cellAt(csvOriginal, 3, 2)
	if got, ok := RemapCell(a, []byte("x,y\n9,9\n")); ok {
		t.Errorf("remap succeeded (%+v) for unrelated content", got)
	}
}

func TestRemapCellWithoutHeaderKeepsColumn(t *testing.T) {
	a := cellAt(csvOriginal, 3, 2)
	a.Header = ""
	updated := "id,name,price,qty\n0,z,0,0\n" + csvOriginal[len("id,name,price,qty\n"):]
	got, ok := RemapCell(a, []byte(updated))
	if !ok || got.Col != 2 || got.StartLine != 4 {
		t.Errorf("got %+v ok=%v, want col 2 line 4", got, ok)
	}
}

func TestRemapCellHeaderRowAnchor(t *testing.T) {
	a := cellAt(csvOriginal, 1, 2)
	got, ok := RemapCell(a, []byte(csvOriginal+"6,fig,1,1\n"))
	if !ok || got.StartLine != 1 || got.Col != 2 || got.Header != "name" {
		t.Errorf("got %+v ok=%v", got, ok)
	}
}

func TestRemapCellFollowsColumnWhenEarlierColumnRemoved(t *testing.T) {
	a := cellAt(csvOriginal, 4, 3) // cherry's price
	updated := "name,price,qty\napple,0.50,10\nbanana,0.25,20\ncherry,4.00,5\ndate,3.00,7\nelder,9.99,1\n"
	got, ok := RemapCell(a, []byte(updated))
	if !ok || got.Col != 2 || got.Header != "price" || got.StartLine != 4 {
		t.Errorf("got %+v ok=%v, want col 2 (price) on line 4", got, ok)
	}
}

func TestSameRecord(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want bool
	}{
		{"1,apple,0.5", "1,apple,x,0.5", true},
		{"1,apple,x,0.5", "1,apple,0.5", true},
		{"1,apple,0.5", "2,pear,0.7", false},
		{"", "", true},
		{"", "1,2", false},
		{"a,a", "a,b", false},
	} {
		if got := sameRecord(tt.a, tt.b); got != tt.want {
			t.Errorf("sameRecord(%q,%q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}
