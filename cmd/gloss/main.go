// Command gloss is the Gloss client: run `gloss .` against a directory to
// boot a localhost web UI and MCP server for async annotation and review
// of AI-generated content.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gloss-mcp/client/internal/connector"
	"github.com/gloss-mcp/client/internal/plugins"
	"github.com/gloss-mcp/client/internal/server"
	"github.com/gloss-mcp/client/internal/store"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

// defaultPort is gloss's fixed default port, chosen for a stable,
// bookmarkable URL across runs. Override with -port (0 for an
// OS-assigned port).
const defaultPort = 4747

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gloss", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: gloss [flags] [directory]")
		fs.PrintDefaults()
	}

	showVersion := fs.Bool("version", false, "print version and exit")
	cloud := fs.Bool("cloud", false, "run as a proxy agent for Gloss Cloud (not yet available)")
	token := fs.String("token", "", "Gloss Cloud API token (not yet available)")
	port := fs.Int("port", defaultPort, "port to serve on (0 for an OS-assigned port)")
	noOpen := fs.Bool("no-open", false, "do not open a browser tab automatically")

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

	st, repo, result, err := openStoreAndSnapshot(abs)
	if err != nil {
		fmt.Fprintf(stderr, "gloss: %v\n", err)
		return 1
	}
	defer func() { _ = st.Close() }()

	fmt.Fprintf(stderr, "gloss: indexed %d files (%d new, %d reused, %d skipped) in %s\n",
		result.Files, result.Created, result.Reused, result.Skipped, abs)

	srv := server.New(server.Config{
		Root:          abs,
		RepoName:      repo.Name,
		ConnectorType: repo.ConnectorType,
		Port:          *port,
		Registry:      plugins.NewRegistry(plugins.NewPlaintext()),
	})

	onReady := func(addr string) {
		url := "http://" + addr
		fmt.Fprintf(stderr, "gloss: serving %s at %s\n", abs, url)
		if !*noOpen {
			if err := server.OpenBrowser(url); err != nil {
				fmt.Fprintf(stderr, "gloss: could not open browser: %v\n", err)
			}
		}
	}

	if err := srv.ListenAndServe(ctx, onReady); err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			fmt.Fprintf(stderr, "gloss: port %d is already in use; pick another with -port\n", *port)
		} else {
			fmt.Fprintf(stderr, "gloss: %v\n", err)
		}
		return 1
	}
	return 0
}

// openStoreAndSnapshot creates <dir>/.gloss/gloss.db (and .gloss/ itself),
// runs migrations, and snapshots dir's tracked files via the connector
// matching the repository's connector type. Unlike a one-shot init, the
// returned *store.Store is left open -- the server holds it for its
// lifetime; callers close it once the server has stopped.
func openStoreAndSnapshot(dir string) (*store.Store, *store.Repository, connector.Result, error) {
	glossDir := filepath.Join(dir, ".gloss")
	if err := os.MkdirAll(glossDir, 0o755); err != nil {
		return nil, nil, connector.Result{}, err
	}
	st, err := store.Open(filepath.Join(glossDir, "gloss.db"))
	if err != nil {
		return nil, nil, connector.Result{}, err
	}

	ctx := context.Background()
	repo, err := st.EnsureRepository(ctx, filepath.Base(dir), connector.Detect(dir), "")
	if err != nil {
		_ = st.Close()
		return nil, nil, connector.Result{}, err
	}

	result, err := connector.New(dir, repo.ConnectorType).Snapshot(ctx, st, repo.ID)
	if err != nil {
		_ = st.Close()
		return nil, nil, connector.Result{}, err
	}

	return st, repo, result, nil
}
