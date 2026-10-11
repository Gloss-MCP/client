package plugins

import (
	"strings"
	"testing"
)

func TestJSONAccepts(t *testing.T) {
	j := NewJSON()
	for path, want := range map[string]bool{"a.json": true, "dir/B.JSON": true, "a.jsonl": false, "a.go": false} {
		if got := j.Accepts(path); got != want {
			t.Errorf("Accepts(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestJSONValidHasRawAndFormattedViews(t *testing.T) {
	views, err := NewJSON().Render(File{Path: "a.json", Content: []byte(`{"a":1,"b":[1,2]}`)})
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 2 || views[0].Name != "Raw" || views[1].Name != "Formatted" {
		t.Fatalf("views = %+v, want Raw then Formatted", views)
	}
	if !strings.Contains(string(views[0].HTML), `data-line="1"`) {
		t.Errorf("Raw view must be line-annotatable: %s", views[0].HTML)
	}
	if strings.Contains(string(views[1].HTML), "data-line") {
		t.Errorf("Formatted view must be read-only (no data-line): %s", views[1].HTML)
	}
	if !strings.Contains(string(views[1].HTML), "&#34;a&#34;: 1") {
		t.Errorf("Formatted view not pretty-printed: %s", views[1].HTML)
	}
}

func TestJSONInvalidFallsBackWithNote(t *testing.T) {
	views, err := NewJSON().Render(File{Path: "a.json", Content: []byte("{\"a\": ,}\n")})
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 {
		t.Fatalf("len(views) = %d, want 1", len(views))
	}
	html := string(views[0].HTML)
	if !strings.Contains(html, "Not valid JSON") || !strings.Contains(html, `data-line="1"`) {
		t.Errorf("want note + annotatable source: %s", html)
	}
}

func TestJSONEscapesHostileContent(t *testing.T) {
	views, err := NewJSON().Render(File{Path: "a.json", Content: []byte(`{"x":"<script>alert(1)</script>"}`)})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range views {
		if strings.Contains(string(v.HTML), "<script>") {
			t.Errorf("view %q has unescaped script: %s", v.Name, v.HTML)
		}
	}
}

func TestJSONEmptyAndBinary(t *testing.T) {
	if _, err := NewJSON().Render(File{Path: "a.json"}); err != nil {
		t.Errorf("empty: %v", err)
	}
	views, err := NewJSON().Render(File{Path: "a.json", Content: []byte("{\x00}")})
	if err != nil || len(views) != 1 || !strings.Contains(string(views[0].HTML), "Binary") {
		t.Errorf("binary: err=%v views=%v", err, views)
	}
}
