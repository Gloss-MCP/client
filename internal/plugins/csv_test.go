package plugins

import (
	"regexp"
	"strings"
	"testing"
)

var csvRowRe = regexp.MustCompile(`<tr class="gloss-csv-row[^"]*" data-line="(\d+)" data-line-end="(\d+)">`)

func csvTable(t *testing.T, src string) string {
	t.Helper()
	views, err := NewCSV().Render(File{Path: "a.csv", Content: []byte(src)})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return string(views[0].HTML)
}

func TestCSVAccepts(t *testing.T) {
	c := NewCSV()
	for path, want := range map[string]bool{"a.csv": true, "d/B.CSV": true, "a.tsv": false, "a.txt": false} {
		if got := c.Accepts(path); got != want {
			t.Errorf("Accepts(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestCSVViewsAndCells(t *testing.T) {
	views, err := NewCSV().Render(File{Path: "a.csv", Content: []byte("id,name\n1,apple\n")})
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 2 || views[0].Name != "Table" || views[1].Name != "Source" {
		t.Fatalf("views = %+v", views)
	}
	html := string(views[0].HTML)
	for _, want := range []string{`<th class="gloss-csv-cell" data-col="2">name</th>`, `<td class="gloss-csv-cell" data-col="2">apple</td>`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in %s", want, html)
		}
	}
	if !strings.Contains(string(views[1].HTML), `data-line="2"`) {
		t.Errorf("Source view not line-annotatable: %s", views[1].HTML)
	}
}

func TestCSVRowLineSpans(t *testing.T) {
	tests := []struct {
		name, src string
		want      [][2]string
	}{
		{"simple", "a,b\n1,2\n3,4\n", [][2]string{{"1", "1"}, {"2", "2"}, {"3", "3"}}},
		{"no trailing newline", "a,b\n1,2", [][2]string{{"1", "1"}, {"2", "2"}}},
		{"quoted newline spans lines", "a,b\n1,\"x\ny\nz\"\n3,4\n", [][2]string{{"1", "1"}, {"2", "4"}, {"5", "5"}}},
		{"blank lines skipped keep numbering", "a,b\n\n1,2\n", [][2]string{{"1", "1"}, {"3", "3"}}},
		{"crlf", "a,b\r\n1,2\r\n", [][2]string{{"1", "1"}, {"2", "2"}}},
		{"bom", "\xef\xbb\xbfa,b\n1,2\n", [][2]string{{"1", "1"}, {"2", "2"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := csvRowRe.FindAllStringSubmatch(csvTable(t, tt.src), -1)
			if len(ms) != len(tt.want) {
				t.Fatalf("rows = %d, want %d", len(ms), len(tt.want))
			}
			for i, m := range ms {
				if m[1] != tt.want[i][0] || m[2] != tt.want[i][1] {
					t.Errorf("row %d span = %s-%s, want %s-%s", i, m[1], m[2], tt.want[i][0], tt.want[i][1])
				}
			}
		})
	}
}

func TestCSVRaggedRows(t *testing.T) {
	html := csvTable(t, "a,b,c\n1\n1,2,3,4\n")
	if !strings.Contains(html, `data-col="4"`) {
		t.Errorf("long row lost its 4th cell: %s", html)
	}
}

func TestCSVHeaderRowIsFirstRecord(t *testing.T) {
	html := csvTable(t, "h1,h2\nv1,v2\n")
	if strings.Count(html, "<th ") != 2 || strings.Count(html, "<td ") != 4 { // 2 line-no tds + 2 value tds
		t.Errorf("header/body cell tags wrong: %s", html)
	}
}

func TestCSVEscapesHostileContent(t *testing.T) {
	html := csvTable(t, "a\n\"<script>alert(1)</script>\"\n")
	if strings.Contains(html, "<script>") {
		t.Errorf("unescaped script: %s", html)
	}
}

func TestCSVEmptyBinaryAndUnparseable(t *testing.T) {
	if views, err := NewCSV().Render(File{Path: "a.csv"}); err != nil || len(views) == 0 {
		t.Errorf("empty: %v %v", err, views)
	}
	views, err := NewCSV().Render(File{Path: "a.csv", Content: []byte("a\x00b")})
	if err != nil || len(views) != 1 || !strings.Contains(string(views[0].HTML), "Binary") {
		t.Errorf("binary: err=%v views=%v", err, views)
	}
}
