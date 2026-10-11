package plugins

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"path"
	"strings"
)

// JSON renders .json files in two views: "Raw", the highlighted source
// as written (the annotatable view, so anchors always refer to the bytes
// on disk), and "Formatted", a read-only pretty-printed copy. Invalid
// JSON falls back to the highlighted source with a note.
type JSON struct {
	code Code
}

// NewJSON constructs the json plugin.
func NewJSON() JSON { return JSON{code: NewCode()} }

// Name implements Plugin.
func (JSON) Name() string { return "json" }

// Accepts implements Plugin.
func (JSON) Accepts(p string) bool { return strings.EqualFold(path.Ext(p), ".json") }

// Render implements Plugin.
func (j JSON) Render(f File) ([]View, error) {
	raw, err := j.code.Render(f)
	if err != nil {
		return nil, err
	}
	if isBinary(f.Content) {
		return raw, nil
	}

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, f.Content, "", "  "); err != nil {
		note := template.HTML(`<p class="gloss-note text-xs text-amber-700 mb-2">Not valid JSON (` +
			template.HTMLEscapeString(err.Error()) + `); showing source.</p>`)
		return []View{{HTML: note + raw[0].HTML}}, nil
	}
	formatted := template.HTML(fmt.Sprintf(
		`<pre class="gloss-readonly font-mono text-xs leading-5 whitespace-pre-wrap break-all">%s</pre>`,
		template.HTMLEscapeString(pretty.String())))
	return []View{
		{Name: "Raw", HTML: raw[0].HTML},
		{Name: "Formatted", HTML: formatted},
	}, nil
}
