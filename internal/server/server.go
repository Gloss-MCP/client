// Package server is the local Gloss web server: a read-only file-browser
// UI (HTMX + Alpine.js + Tailwind, server-rendered via html/template)
// backed by a plugins.Registry that renders file content.
package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gloss-mcp/client/internal/plugins"
	"github.com/gloss-mcp/client/internal/store"
)

// Config configures a Server.
type Config struct {
	Root          string // absolute path to the directory being served
	RepoName      string
	ConnectorType store.ConnectorType
	Port          int // 0 = OS-assigned
	Registry      *plugins.Registry
}

// Server serves the read-only file-browser UI over HTTP.
type Server struct {
	cfg Config
	mux *http.ServeMux
}

// New builds a Server from cfg. Call Handler for tests, or
// ListenAndServe to actually run it.
func New(cfg Config) *Server {
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	s.routes()
	return s
}

// Handler returns the server's http.Handler.
func (s *Server) Handler() http.Handler { return s.mux }

// ListenAndServe binds 127.0.0.1:cfg.Port (0 = OS-assigned; this is a
// local-only tool, never binds beyond loopback), calls onReady with the
// bound address once listening starts, then serves until ctx is
// canceled, at which point it shuts down gracefully.
func (s *Server) ListenAndServe(ctx context.Context, onReady func(addr string)) error {
	addr := fmt.Sprintf("127.0.0.1:%d", s.cfg.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("server: listen on %s: %w", addr, err)
	}

	if onReady != nil {
		onReady(ln.Addr().String())
	}

	httpServer := &http.Server{Handler: s.mux}
	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.Serve(ln) }()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
}
