// Package site holds the static docs site. The test checks it can be
// published as-is: every page has a title and description, and every
// relative link resolves to a file that exists.
package site

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	linkRE  = regexp.MustCompile(`(?:href|src)="([^"]+)"`)
	titleRE = regexp.MustCompile(`<title>[^<]+</title>`)
	descRE  = regexp.MustCompile(`<meta name="description" content="[^"]+">`)
)

// generated lists build outputs that are gitignored, so links to them are
// allowed to dangle in a fresh checkout.
var generated = map[string]bool{"css/site.css": true}

func TestPages(t *testing.T) {
	pages, err := filepath.Glob("*.html")
	if err != nil || len(pages) == 0 {
		t.Fatalf("no pages found: %v", err)
	}
	for _, page := range pages {
		b, err := os.ReadFile(page)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		if !titleRE.MatchString(src) {
			t.Errorf("%s: missing <title>", page)
		}
		if !descRE.MatchString(src) {
			t.Errorf("%s: missing meta description", page)
		}
		for _, m := range linkRE.FindAllStringSubmatch(src, -1) {
			ref := m[1]
			if strings.Contains(ref, "://") || strings.HasPrefix(ref, "#") || strings.HasPrefix(ref, "mailto:") {
				continue
			}
			target, _, _ := strings.Cut(ref, "#")
			if generated[target] {
				continue
			}
			if _, err := os.Stat(target); err != nil {
				t.Errorf("%s: link %q does not resolve", page, ref)
			}
		}
	}
}
