// Package site holds the static docs site. The test checks it can be
// published as-is: every page has a title and description, and every
// relative link resolves to a file that exists and is staged by pages.yml.
package site

import (
	"os"
	"os/exec"
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
	// A link that resolves in the checkout still 404s in production unless
	// the deploy workflow copies it into the published artifact.
	wf, err := os.ReadFile("../.github/workflows/pages.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(wf)
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
			if dir, _, ok := strings.Cut(target, "/"); ok && !strings.Contains(workflow, "site/"+dir) {
				t.Errorf("%s: %q is not staged by pages.yml (no site/%s)", page, ref, dir)
			}
		}
	}
}

// TestInstallScriptIsPublishable guards what pages.yml copies: the installer
// must exist, start with a shebang and be parseable by sh.
func TestInstallScriptIsPublishable(t *testing.T) {
	b, err := os.ReadFile("install.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b), "#!/bin/sh\n") {
		t.Error("install.sh must start with #!/bin/sh")
	}
	if sh, err := exec.LookPath("sh"); err == nil {
		if out, err := exec.Command(sh, "-n", "install.sh").CombinedOutput(); err != nil {
			t.Errorf("sh -n install.sh: %v\n%s", err, out)
		}
	}
}
