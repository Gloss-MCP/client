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

	"github.com/gloss-mcp/client/internal/connector"
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

	result, err := initStore(abs)
	if err != nil {
		fmt.Fprintf(stderr, "gloss: %v\n", err)
		return 1
	}
	fmt.Fprintf(stderr, "gloss: indexed %d files (%d new, %d reused, %d skipped) in %s\n",
		result.Files, result.Created, result.Reused, result.Skipped, abs)

	// Server mode lands in milestone 4 (web server shell); until then the
	// skeleton initialises the store, snapshots tracked files, and
	// validates its input.
	fmt.Fprintf(stderr, "gloss: server mode is not yet implemented (would serve %s)\n", abs)
	return 1
}

// initStore creates <dir>/.gloss/gloss.db (and .gloss/ itself), runs
// migrations, and snapshots dir's tracked files via the connector
// matching the repository's connector type.
func initStore(dir string) (connector.Result, error) {
	glossDir := filepath.Join(dir, ".gloss")
	if err := os.MkdirAll(glossDir, 0o755); err != nil {
		return connector.Result{}, err
	}
	st, err := store.Open(filepath.Join(glossDir, "gloss.db"))
	if err != nil {
		return connector.Result{}, err
	}

	ctx := context.Background()
	repo, err := st.EnsureRepository(ctx, filepath.Base(dir), connector.Detect(dir), "")
	if err != nil {
		_ = st.Close()
		return connector.Result{}, err
	}

	result, err := connector.New(dir, repo.ConnectorType).Snapshot(ctx, st, repo.ID)
	if err != nil {
		_ = st.Close()
		return connector.Result{}, err
	}

	if err := st.Close(); err != nil {
		return connector.Result{}, err
	}
	return result, nil
}
