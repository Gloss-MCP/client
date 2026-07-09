package connector

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

// ignoreRule is one parsed line of a .gitignore/.glossignore file.
//
// Supported syntax: comments (#), blank lines, "!" negation, a trailing
// "/" for directory-only patterns, a leading (or any) "/" for
// root-anchored patterns, and the glob wildcards "*", "**", "?".
//
// Not supported (documented limitation, not silently wrong): character
// classes ("[abc]", "[a-z]"), escaped specials ("\#", "\!"), and nested
// per-directory ignore files — this package only reads a single
// .gitignore and a single .glossignore at the repository root.
type ignoreRule struct {
	negate   bool
	dirOnly  bool
	anchored bool
	re       *regexp.Regexp
}

// ignoreMatcher combines the rules from one or more ignore files, in
// file order, and answers whether a given path is ignored.
type ignoreMatcher struct {
	rules []ignoreRule
}

// loadIgnoreFile reads and parses path. A missing file yields (nil, nil)
// -- "absent" means "no rules", not an error.
func loadIgnoreFile(path string) ([]ignoreRule, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return parseIgnoreLines(lines), nil
}

// parseIgnoreLines parses ignore-file lines into rules, preserving
// order (later rules can override earlier ones during matching).
func parseIgnoreLines(lines []string) []ignoreRule {
	var rules []ignoreRule
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		line = strings.TrimRight(line, " \t")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		negate := strings.HasPrefix(line, "!")
		if negate {
			line = line[1:]
		}

		dirOnly := strings.HasSuffix(line, "/")
		if dirOnly {
			line = strings.TrimSuffix(line, "/")
		}
		if line == "" {
			continue
		}

		anchored := strings.HasPrefix(line, "/") || strings.Contains(line, "/")
		line = strings.TrimPrefix(line, "/")

		rules = append(rules, ignoreRule{
			negate:   negate,
			dirOnly:  dirOnly,
			anchored: anchored,
			re:       compileGlob(line),
		})
	}
	return rules
}

// compileGlob translates a gitignore-style glob into an anchored regexp
// matched against either a full relative path (anchored rules) or a
// single path segment (unanchored rules) — see ignoreMatcher.match.
func compileGlob(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	runes := []rune(pattern)
	for i := 0; i < len(runes); i++ {
		switch {
		case strings.HasPrefix(string(runes[i:]), "**"):
			b.WriteString(".*")
			i++
		case runes[i] == '*':
			b.WriteString("[^/]*")
		case runes[i] == '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(runes[i])))
		}
	}
	b.WriteString("$")

	re, err := regexp.Compile(b.String())
	if err != nil {
		// compileGlob only ever emits valid regexp syntax (all literals
		// are escaped via QuoteMeta); a failure here is a bug in this
		// function, not bad user input.
		panic("connector: invalid generated pattern: " + err.Error())
	}
	return re
}

// match reports whether relPath (slash-separated, relative to the
// repository root) is ignored. isDir must be true when relPath names a
// directory, since dir-only rules only apply there.
func (m *ignoreMatcher) match(relPath string, isDir bool) bool {
	if m == nil {
		return false
	}
	ignored := false
	for _, r := range m.rules {
		if r.dirOnly && !isDir {
			continue
		}
		if r.anchored {
			if r.re.MatchString(relPath) {
				ignored = !r.negate
			}
			continue
		}
		if matchesAnySegment(r.re, relPath) {
			ignored = !r.negate
		}
	}
	return ignored
}

func matchesAnySegment(re *regexp.Regexp, relPath string) bool {
	for _, seg := range strings.Split(relPath, "/") {
		if re.MatchString(seg) {
			return true
		}
	}
	return false
}
