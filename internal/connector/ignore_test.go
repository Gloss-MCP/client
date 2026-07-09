package connector

import (
	"os"
	"testing"
)

func TestParseIgnoreLinesSkipsCommentsAndBlanks(t *testing.T) {
	rules := parseIgnoreLines([]string{"", "# comment", "  ", "*.log"})
	if len(rules) != 1 {
		t.Fatalf("rules = %d, want 1 (blank/comment lines skipped)", len(rules))
	}
}

func TestIgnoreMatcherMatch(t *testing.T) {
	tests := []struct {
		name    string
		lines   []string
		path    string
		isDir   bool
		ignored bool
	}{
		{"plain basename match", []string{"vendor"}, "vendor", true, true},
		{"unanchored matches at depth", []string{"vendor"}, "src/vendor", true, true},
		{"dir-only rule ignores file with same name", []string{"vendor/"}, "vendor", false, false},
		{"dir-only rule matches directory", []string{"vendor/"}, "vendor", true, true},
		{"root-anchored does not match nested", []string{"/vendor"}, "src/vendor", true, false},
		{"root-anchored matches root", []string{"/vendor"}, "vendor", true, true},
		{"mid-pattern slash anchors to root", []string{"src/vendor"}, "src/vendor", true, true},
		{"mid-pattern slash does not match elsewhere", []string{"src/vendor"}, "other/src/vendor", true, false},
		{"star wildcard", []string{"*.local"}, "secrets.local", false, true},
		{"star does not cross slash", []string{"*.local"}, "dir/secrets.local", false, true}, // unanchored: matches segment
		{"question mark", []string{"a?c"}, "abc", false, true},
		{"double star crosses slash", []string{"**/build"}, "a/b/build", true, true},
		{"negation re-includes", []string{"*.local", "!important.local"}, "important.local", false, false},
		{"negation does not affect other files", []string{"*.local", "!important.local"}, "secrets.local", false, true},
		{"later rule overrides earlier", []string{"!keep.md", "keep.md"}, "keep.md", false, true},
		{"no rules matches nothing", nil, "anything", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &ignoreMatcher{rules: parseIgnoreLines(tt.lines)}
			got := m.match(tt.path, tt.isDir)
			if got != tt.ignored {
				t.Errorf("match(%q, isDir=%v) = %v, want %v", tt.path, tt.isDir, got, tt.ignored)
			}
		})
	}
}

func TestNilMatcherMatchesNothing(t *testing.T) {
	var m *ignoreMatcher
	if m.match("anything", false) {
		t.Error("nil matcher should never ignore")
	}
}

func TestLoadIgnoreFileMissingIsNotError(t *testing.T) {
	rules, err := loadIgnoreFile("/nonexistent/path/.gitignore")
	if err != nil {
		t.Fatalf("loadIgnoreFile missing file: %v", err)
	}
	if rules != nil {
		t.Errorf("rules = %v, want nil for missing file", rules)
	}
}

func TestLoadIgnoreFileParsesContent(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/.gitignore"
	if err := os.WriteFile(path, []byte("vendor/\n*.local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rules, err := loadIgnoreFile(path)
	if err != nil {
		t.Fatalf("loadIgnoreFile: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("rules = %d, want 2", len(rules))
	}
}
