// Package mcp exposes the Gloss MCP server: a streamable HTTP endpoint
// that gives AI clients read/write access to sessions, threads, comments,
// and repositories (docs/mcp-api.md). The handler is mounted by
// internal/server at /mcp on the already-running gloss binary — the
// single owner of the SQLite handle.
package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gloss-mcp/client/internal/connector"
	"github.com/gloss-mcp/client/internal/store"
)

// Config configures the MCP handler.
type Config struct {
	Store         *store.Store
	RepoID        string
	Root          string // absolute path to the served directory
	ConnectorType store.ConnectorType
}

// NewHandler builds and registers all Gloss MCP tools, then returns an
// http.Handler to mount at /mcp. The handler is stateless between
// requests; all state lives in cfg.Store.
func NewHandler(cfg Config) http.Handler {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "gloss",
		Version: "0.1.0",
	}, nil)

	registerSessionTools(srv, cfg)
	registerThreadTools(srv, cfg)
	registerCommentTools(srv, cfg)
	registerRepoTools(srv, cfg)

	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return srv
	}, nil)
}

// agentFrom resolves the author_agent for a write operation. The override
// (from the tool's explicit author_agent parameter) takes precedence; if
// absent, we fall back to the client identity declared in the MCP
// initialize handshake.
func agentFrom(req *mcp.CallToolRequest, override string) string {
	if override != "" {
		return override
	}
	if ip := req.Session.InitializeParams(); ip != nil && ip.ClientInfo != nil {
		return ip.ClientInfo.Name
	}
	return ""
}

// jsonResult serialises v to JSON and wraps it in a TextContent result.
func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
	}, nil, nil
}

// toolErr returns a tool-level error result (IsError = true) for caller
// mistakes such as "not found" or "invalid input". Fatal server errors
// should be returned as the third (error) return value instead.
func toolErr(err error) (*mcp.CallToolResult, any, error) {
	res := &mcp.CallToolResult{}
	res.SetError(err)
	return res, nil, nil
}

// listTrackedFiles returns the sorted tracked-file slice for the root.
func listTrackedFiles(cfg Config) ([]string, error) {
	paths, err := connector.ListFiles(cfg.Root, cfg.ConnectorType)
	if err != nil {
		return nil, fmt.Errorf("mcp: list files: %w", err)
	}
	return paths, nil
}
