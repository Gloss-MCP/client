package server

import (
	"embed"
	"encoding/json"
	"html/template"
)

//go:embed templates/*.html
var templateFS embed.FS

// funcMap's toJSON returns a plain string (not template.JS): x-data
// attributes aren't a context html/template recognises as JS, so
// html/template applies ordinary HTML-attribute escaping to it -- which
// is what's wanted, since the browser decodes entities before Alpine
// evaluates the attribute as a JS expression.
var funcMap = template.FuncMap{
	"toJSON": func(v any) (string, error) {
		b, err := json.Marshal(v)
		return string(b), err
	},
	"treeCtx": func(n *treeNode, linkPrefix, hxTarget string) treeCtx {
		return treeCtx{Node: n, LinkPrefix: linkPrefix, HXTarget: hxTarget}
	},
}

var tmpl = template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html"))
