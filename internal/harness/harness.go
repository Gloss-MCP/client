// Package harness detects which AI agent harnesses (Claude Code, Codex, ...)
// are installed on the machine and tells the user how to connect each one to
// Gloss's MCP endpoint.
//
// Detection is best-effort and purely local: a binary on PATH and/or a
// config directory in $HOME. Whether a harness is already connected is a
// substring check for the endpoint URL in that harness's known config files,
// which stays robust across JSON, TOML and YAML without parsing any of them.
// The table below is data, so a wrong entry is a one-line fix.
package harness

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Env is everything detection needs from the outside world, injectable so
// tests need no real installs.
type Env struct {
	Home     string // user home directory ("" disables all home lookups)
	Root     string // the directory gloss is serving (project-scoped configs)
	LookPath func(name string) (string, error)
	ReadFile func(path string) ([]byte, error)
	DirExist func(path string) bool
}

// SystemEnv is the real environment for a gloss run rooted at root.
func SystemEnv(root string) Env {
	home, _ := os.UserHomeDir()
	return Env{
		Home:     home,
		Root:     root,
		LookPath: exec.LookPath,
		ReadFile: os.ReadFile,
		DirExist: func(p string) bool {
			fi, err := os.Stat(p)
			return err == nil && fi.IsDir()
		},
	}
}

// Harness describes one supported agent harness.
type Harness struct {
	Name     string
	Binaries []string // executable names on PATH
	Dirs     []string // config dirs relative to $HOME
	// RequireBoth demands a binary AND a config dir, for harnesses whose
	// binary name is too generic to trust alone.
	RequireBoth bool
	HomeFiles   []string // config files relative to $HOME
	RootFiles   []string // config files relative to the served directory
	// Connect returns the instructions for connecting to url.
	Connect func(url string) string
}

// URL is the MCP endpoint for a listener address such as "127.0.0.1:4747".
// It uses "localhost" to match what .mcp.json and the docs advertise.
func URL(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		port = addr
	}
	return "http://localhost:" + port + "/mcp"
}

// Installed reports whether h looks installed.
func (h Harness) Installed(e Env) bool {
	bin := false
	for _, b := range h.Binaries {
		if e.LookPath != nil {
			if _, err := e.LookPath(b); err == nil {
				bin = true
				break
			}
		}
	}
	dir := false
	if e.Home != "" && e.DirExist != nil {
		for _, d := range h.Dirs {
			if e.DirExist(filepath.Join(e.Home, d)) {
				dir = true
				break
			}
		}
	}
	if h.RequireBoth {
		return bin && dir
	}
	return bin || dir
}

// Configured reports whether any of h's known config files already mention
// the endpoint url (on either localhost or 127.0.0.1).
func (h Harness) Configured(e Env, url string) bool {
	if e.ReadFile == nil {
		return false
	}
	needles := endpointNeedles(url)
	var paths []string
	if e.Home != "" {
		for _, f := range h.HomeFiles {
			paths = append(paths, filepath.Join(e.Home, f))
		}
	}
	if e.Root != "" {
		for _, f := range h.RootFiles {
			paths = append(paths, filepath.Join(e.Root, f))
		}
	}
	for _, p := range paths {
		b, err := e.ReadFile(p)
		if err != nil {
			continue
		}
		s := string(b)
		for _, n := range needles {
			if strings.Contains(s, n) {
				return true
			}
		}
	}
	return false
}

func endpointNeedles(url string) []string {
	hp := strings.TrimPrefix(url, "http://")
	return []string{hp, strings.Replace(hp, "localhost", "127.0.0.1", 1)}
}

// Pending returns the installed harnesses that are not yet connected to url.
func Pending(e Env, url string) []Harness {
	var out []Harness
	for _, h := range All() {
		if h.Installed(e) && !h.Configured(e, url) {
			out = append(out, h)
		}
	}
	return out
}

// Print writes connect instructions for hs to w. It writes nothing when hs
// is empty.
func Print(w io.Writer, hs []Harness, url string) {
	if len(hs) == 0 {
		return
	}
	fmt.Fprintf(w, "gloss: detected agent harnesses not yet connected to %s\n", url)
	for _, h := range hs {
		fmt.Fprintf(w, "\n  %s\n", h.Name)
		for _, line := range strings.Split(strings.TrimRight(h.Connect(url), "\n"), "\n") {
			if line == "" {
				fmt.Fprintln(w)
				continue
			}
			fmt.Fprintf(w, "    %s\n", line)
		}
	}
	fmt.Fprintln(w)
}

// All is the supported harness table. Order is display order.
func All() []Harness {
	return []Harness{
		{
			Name:      "Claude Code",
			Binaries:  []string{"claude"},
			Dirs:      []string{".claude"},
			HomeFiles: []string{".claude.json"},
			RootFiles: []string{".mcp.json"},
			Connect: func(url string) string {
				return "claude mcp add --transport http --scope user gloss " + url
			},
		},
		{
			Name:      "OpenAI Codex",
			Binaries:  []string{"codex"},
			Dirs:      []string{".codex"},
			HomeFiles: []string{".codex/config.toml"},
			RootFiles: []string{".codex/config.toml"},
			Connect: func(url string) string {
				return "codex mcp add gloss --url " + url
			},
		},
		{
			Name:        "Pi",
			Binaries:    []string{"pi"},
			Dirs:        []string{".pi"},
			RequireBoth: true,
			HomeFiles:   []string{".pi/agent/mcp.json"},
			RootFiles:   []string{".pi/mcp.json"},
			Connect: func(url string) string {
				return "Pi has no built-in MCP support; install the pi-mcp-adapter extension, then\n" +
					"add this to ~/.pi/agent/mcp.json:\n\n" +
					"{\n  \"mcpServers\": {\n    \"gloss\": { \"url\": \"" + url + "\" }\n  }\n}"
			},
		},
		{
			Name:      "Hermes Agent",
			Binaries:  []string{"hermes"},
			Dirs:      []string{".hermes"},
			HomeFiles: []string{".hermes/config.yaml"},
			Connect: func(url string) string {
				return "Add this to ~/.hermes/config.yaml, then run /reload-mcp in a session:\n\n" +
					"mcp_servers:\n  gloss:\n    url: \"" + url + "\""
			},
		},
		{
			Name:      "OpenCode",
			Binaries:  []string{"opencode"},
			Dirs:      []string{".config/opencode"},
			HomeFiles: []string{".config/opencode/opencode.json"},
			RootFiles: []string{"opencode.json"},
			Connect: func(url string) string {
				return "Add this to ~/.config/opencode/opencode.json (or ./opencode.json):\n\n" +
					"{\n  \"mcp\": {\n    \"gloss\": { \"type\": \"remote\", \"url\": \"" + url + "\", \"enabled\": true }\n  }\n}"
			},
		},
		{
			Name:      "Kimi Code",
			Binaries:  []string{"kimi"},
			Dirs:      []string{".kimi"},
			HomeFiles: []string{".kimi/mcp.json"},
			Connect: func(url string) string {
				return "kimi mcp add --transport http gloss " + url
			},
		},
		{
			Name:      "Grok Build",
			Binaries:  []string{"grok"},
			Dirs:      []string{".grok"},
			HomeFiles: []string{".grok/config.toml"},
			RootFiles: []string{".grok/config.toml"},
			Connect: func(url string) string {
				return "grok mcp add --transport http gloss " + url
			},
		},
	}
}
