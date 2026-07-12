package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gloss-mcp/client/internal/store"
)

func registerSessionTools(srv *mcp.Server, cfg Config) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_session",
		Description: "Create a new review session in the given repository.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, p createSessionParams) (*mcp.CallToolResult, any, error) {
		return handleCreateSession(ctx, cfg, p)
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_sessions",
		Description: "List sessions for a repository, optionally filtered by status (open, resolved, archived).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, p listSessionsParams) (*mcp.CallToolResult, any, error) {
		return handleListSessions(ctx, cfg, p)
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_session",
		Description: "Get a session by ID, including thread and comment statistics.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, p getSessionParams) (*mcp.CallToolResult, any, error) {
		return handleGetSession(ctx, cfg, p)
	})
}

type createSessionParams struct {
	RepoID      string `json:"repo_id"              jsonschema:"required,Repository ID to create the session in"`
	Name        string `json:"name"                 jsonschema:"required,Human-readable session name"`
	Description string `json:"description,omitempty" jsonschema:"Optional description"`
}

type listSessionsParams struct {
	RepoID string `json:"repo_id"          jsonschema:"required,Repository ID"`
	Status string `json:"status,omitempty" jsonschema:"Filter by status: open, resolved, or archived"`
}

type getSessionParams struct {
	SessionID string `json:"session_id" jsonschema:"required,Session ID"`
}

func handleCreateSession(ctx context.Context, cfg Config, p createSessionParams) (*mcp.CallToolResult, any, error) {
	if p.RepoID == "" || p.Name == "" {
		return toolErr(fmt.Errorf("repo_id and name are required"))
	}
	sess, err := cfg.Store.CreateSession(ctx, p.RepoID, p.Name, p.Description, "")
	if err != nil {
		return nil, nil, fmt.Errorf("create session: %w", err)
	}
	return jsonResult(sess)
}

func handleListSessions(ctx context.Context, cfg Config, p listSessionsParams) (*mcp.CallToolResult, any, error) {
	if p.RepoID == "" {
		return toolErr(fmt.Errorf("repo_id is required"))
	}
	sessions, err := cfg.Store.ListSessions(ctx, p.RepoID, store.SessionStatus(p.Status))
	if err != nil {
		return nil, nil, fmt.Errorf("list sessions: %w", err)
	}
	if sessions == nil {
		sessions = []*store.Session{}
	}
	return jsonResult(sessions)
}

type sessionWithStats struct {
	*store.Session
	Stats *store.SessionStats `json:"stats"`
}

func handleGetSession(ctx context.Context, cfg Config, p getSessionParams) (*mcp.CallToolResult, any, error) {
	if p.SessionID == "" {
		return toolErr(fmt.Errorf("session_id is required"))
	}
	sess, err := cfg.Store.GetSession(ctx, p.SessionID)
	if errors.Is(err, store.ErrNotFound) {
		return toolErr(fmt.Errorf("session not found: %s", p.SessionID))
	}
	if err != nil {
		return nil, nil, fmt.Errorf("get session: %w", err)
	}
	stats, err := cfg.Store.GetSessionStats(ctx, p.SessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("get session stats: %w", err)
	}
	return jsonResult(sessionWithStats{Session: sess, Stats: stats})
}
