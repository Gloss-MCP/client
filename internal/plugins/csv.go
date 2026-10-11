package plugins

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"html/template"
	"io"
	"path"
	"strings"
)

// CSV renders .csv files in two views: "Table", whose rows and cells are
// annotatable, and "Source" (highlighted text, line-annotatable).
//
// Each Table row carries data-line / data-line-end -- the physical line
// span of its record, since a quoted field may contain newlines -- and
// each cell carries data-col (1-based). A selection on a cell anchors
// with a CellAnchor; a selection on the row alone with a LineAnchor.
type CSV struct {
	code Code
}

// NewCSV constructs the csv plugin.
func NewCSV() CSV { return CSV{code: NewCode()} }

// Name implements Plugin.
func (CSV) Name() string { return "csv" }

// Accepts implements Plugin.
func (CSV) Accepts(p string) bool { return strings.EqualFold(path.Ext(p), ".csv") }

type csvCell struct {
	Col  int
	Text string
}

type csvRow struct {
	Start, End int
	Header     bool
	Cells      []csvCell
}

var csvTemplate = template.Must(template.New("csv").Parse(`<div class="gloss-csv-wrap overflow-x-auto"><table class="gloss-csv font-mono text-xs">
{{- range .}}
<tr class="gloss-csv-row{{if .Header}} gloss-csv-header{{end}}" data-line="{{.Start}}" data-line-end="{{.End}}"><td class="gloss-line-no select-none text-right pr-3 text-gray-400">{{.Start}}</td>
{{- $h := .Header}}{{range .Cells}}{{if $h}}<th class="gloss-csv-cell" data-col="{{.Col}}">{{.Text}}</th>{{else}}<td class="gloss-csv-cell" data-col="{{.Col}}">{{.Text}}</td>{{end}}{{end}}</tr>
{{- end}}
</table></div>`))

// Render implements Plugin. Binary content, or content encoding/csv
// cannot parse, falls back to the plain line view with a note.
func (c CSV) Render(f File) ([]View, error) {
	source, err := c.code.Render(f)
	if err != nil {
		return nil, err
	}
	if isBinary(f.Content) {
		return source, nil
	}

	rows, err := parseCSVRows(f.Content)
	if err != nil {
		note := template.HTML(`<p class="gloss-note text-xs text-amber-700 mb-2">Could not parse as CSV (` +
			template.HTMLEscapeString(err.Error()) + `); showing source.</p>`)
		return []View{{HTML: note + source[0].HTML}}, nil
	}

	var buf bytes.Buffer
	if err := csvTemplate.Execute(&buf, rows); err != nil {
		return nil, fmt.Errorf("plugins: render csv %s: %w", f.Path, err)
	}
	return []View{
		{Name: "Table", HTML: template.HTML(buf.String())},
		{Name: "Source", HTML: source[0].HTML},
	}, nil
}

// parseCSVRows parses content into rows with their physical line spans.
// Ragged rows and bare quotes are tolerated.
func parseCSVRows(content []byte) ([]csvRow, error) {
	content = bytes.TrimPrefix(content, []byte("\xef\xbb\xbf"))
	lm := newLineMap(content)

	r := csv.NewReader(bytes.NewReader(content))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	var rows []csvRow
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		line, _ := r.FieldPos(0)
		end := lm.lineOf(max(int(r.InputOffset())-1, 0))
		if end < line {
			end = line
		}
		row := csvRow{Start: line, End: end, Header: len(rows) == 0}
		for i, field := range rec {
			row.Cells = append(row.Cells, csvCell{Col: i + 1, Text: field})
		}
		rows = append(rows, row)
	}
	return rows, nil
}
