package harness

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testURL = "http://localhost:4747/mcp"

// fakeEnv builds an Env over an in-memory world.
func fakeEnv(bins []string, dirs []string, files map[string]string) Env {
	has := func(list []string, s string) bool {
		for _, x := range list {
			if x == s {
				return true
			}
		}
		return false
	}
	return Env{
		Home: "/home/u",
		Root: "/proj",
		LookPath: func(n string) (string, error) {
			if has(bins, n) {
				return "/usr/bin/" + n, nil
			}
			return "", errors.New("not found")
		},
		ReadFile: func(p string) ([]byte, error) {
			if s, ok := files[p]; ok {
				return []byte(s), nil
			}
			return nil, os.ErrNotExist
		},
		DirExist: func(p string) bool { return has(dirs, p) },
	}
}

func names(hs []Harness) []string {
	var out []string
	for _, h := range hs {
		out = append(out, h.Name)
	}
	return out
}

func TestURL(t *testing.T) {
	for addr, want := range map[string]string{
		"127.0.0.1:4747": testURL,
		"[::]:4747":      testURL,
		"4747":           testURL,
	} {
		if got := URL(addr); got != want {
			t.Errorf("URL(%q) = %q, want %q", addr, got, want)
		}
	}
}

func TestPending(t *testing.T) {
	tests := []struct {
		name  string
		bins  []string
		dirs  []string
		files map[string]string
		want  []string
	}{
		{name: "nothing installed"},
		{name: "binary only", bins: []string{"claude"}, want: []string{"Claude Code"}},
		{name: "dir only", dirs: []string{"/home/u/.codex"}, want: []string{"OpenAI Codex"}},
		{
			name: "pi binary alone is too generic",
			bins: []string{"pi"},
		},
		{
			name: "pi needs binary and dir",
			bins: []string{"pi"},
			dirs: []string{"/home/u/.pi"},
			want: []string{"Pi"},
		},
		{
			name: "several, in table order",
			bins: []string{"grok", "claude", "opencode"},
			want: []string{"Claude Code", "OpenCode", "Grok Build"},
		},
		{
			name:  "configured via project .mcp.json",
			bins:  []string{"claude"},
			files: map[string]string{"/proj/.mcp.json": `{"url":"http://localhost:4747/mcp"}`},
		},
		{
			name:  "configured with 127.0.0.1",
			bins:  []string{"codex"},
			files: map[string]string{"/home/u/.codex/config.toml": `url = "http://127.0.0.1:4747/mcp"`},
		},
		{
			name:  "other port does not count as configured",
			bins:  []string{"claude"},
			files: map[string]string{"/home/u/.claude.json": `{"url":"http://localhost:9999/mcp"}`},
			want:  []string{"Claude Code"},
		},
		{
			name:  "configured harness hidden, others kept",
			bins:  []string{"kimi", "grok"},
			files: map[string]string{"/home/u/.kimi/mcp.json": `localhost:4747/mcp`},
			want:  []string{"Grok Build"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := names(Pending(fakeEnv(tt.bins, tt.dirs, tt.files), testURL))
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("Pending = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSystemEnvDetectsRealFiles(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", t.TempDir()) // no binaries

	if err := os.MkdirAll(filepath.Join(home, ".hermes"), 0o755); err != nil {
		t.Fatal(err)
	}
	e := SystemEnv(root)
	if got := names(Pending(e, testURL)); strings.Join(got, ",") != "Hermes Agent" {
		t.Fatalf("Pending = %v, want [Hermes Agent]", got)
	}

	cfg := filepath.Join(home, ".hermes", "config.yaml")
	if err := os.WriteFile(cfg, []byte("mcp_servers:\n  gloss:\n    url: \""+testURL+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Pending(e, testURL); len(got) != 0 {
		t.Fatalf("Pending after configuring = %v, want none", names(got))
	}
}

func TestPrint(t *testing.T) {
	var buf bytes.Buffer
	Print(&buf, nil, testURL)
	if buf.Len() != 0 {
		t.Fatalf("empty list printed %q", buf.String())
	}

	Print(&buf, All(), testURL)
	out := buf.String()
	for _, want := range []string{
		"claude mcp add --transport http --scope user gloss " + testURL,
		"codex mcp add gloss --url " + testURL,
		"pi-mcp-adapter",
		"/reload-mcp",
		`"type": "remote"`,
		"kimi mcp add --transport http gloss " + testURL,
		"grok mcp add --transport http gloss " + testURL,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestAllHaveRequiredFields(t *testing.T) {
	seen := map[string]bool{}
	for _, h := range All() {
		if h.Name == "" || len(h.Binaries) == 0 || len(h.Dirs) == 0 || h.Connect == nil {
			t.Errorf("incomplete harness: %+v", h)
		}
		if seen[h.Name] {
			t.Errorf("duplicate harness %q", h.Name)
		}
		seen[h.Name] = true
		if !strings.Contains(h.Connect(testURL), testURL) {
			t.Errorf("%s instructions omit the URL", h.Name)
		}
	}
	if len(seen) != 7 {
		t.Errorf("expected 7 harnesses, got %d", len(seen))
	}
}
