// Command gloss is the Gloss client: run `gloss .` against a directory to
// boot a localhost web UI and MCP server for async annotation and review
// of AI-generated content.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gloss-mcp/client/internal/store"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gloss", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: gloss [flags] [directory]")
		fs.PrintDefaults()
	}

	showVersion := fs.Bool("version", false, "print version and exit")
	cloud := fs.Bool("cloud", false, "run as a proxy agent for Gloss Cloud (not yet available)")
	token := fs.String("token", "", "Gloss Cloud API token (not yet available)")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showVersion {
		fmt.Fprintf(stdout, "gloss %s\n", version)
		return 0
	}

	if *cloud || *token != "" {
		fmt.Fprintln(stderr, "gloss: proxy mode (--cloud) is not yet available")
		return 1
	}

	if fs.NArg() > 1 {
		fmt.Fprintln(stderr, "gloss: expected at most one directory argument")
		fs.Usage()
		return 2
	}

	dir := "."
	if fs.NArg() == 1 {
		dir = fs.Arg(0)
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintf(stderr, "gloss: %v\n", err)
		return 1
	}
	info, err := os.Stat(abs)
	if err != nil {
		fmt.Fprintf(stderr, "gloss: %v\n", err)
		return 1
	}
	if !info.IsDir() {
		fmt.Fprintf(stderr, "gloss: %s is not a directory\n", abs)
		return 1
	}

	if err := initStore(abs); err != nil {
		fmt.Fprintf(stderr, "gloss: %v\n", err)
		return 1
	}

	// Server mode lands in milestone 4 (web server shell); until then the
	// skeleton initialises the store and validates its input.
	fmt.Fprintf(stderr, "gloss: server mode is not yet implemented (would serve %s)\n", abs)
	return 1
}

// initStore creates <dir>/.gloss/gloss.db (and .gloss/ itself) and runs
// migrations, so the review store exists before server mode lands.
func initStore(dir string) error {
	glossDir := filepath.Join(dir, ".gloss")
	if err := os.MkdirAll(glossDir, 0o755); err != nil {
		return err
	}
	st, err := store.Open(filepath.Join(glossDir, "gloss.db"))
	if err != nil {
		return err
	}

	// Milestone 3 (connectors) takes over git awareness; until then a
	// .git directory is enough to record the connector type honestly.
	connectorType := store.ConnectorLocal
	if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil && info.IsDir() {
		connectorType = store.ConnectorGit
	}
	if _, err := st.EnsureRepository(context.Background(), filepath.Base(dir), connectorType, ""); err != nil {
		_ = st.Close()
		return err
	}
	return st.Close()
}
