package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gloss-mcp/client/internal/store"
)

func registerRepoTools(srv *mcp.Server, cfg Config) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_repos",
		Description: "List all repositories. In local mode there is always exactly one.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return handleListRepos(ctx, cfg)
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_repo",
		Description: "Get a repository by ID, including its session count.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, p getRepoParams) (*mcp.CallToolResult, any, error) {
		return handleGetRepo(ctx, cfg, p)
	})
}

type getRepoParams struct {
	RepoID string `json:"repo_id" jsonschema:"required,Repository ID"`
}

type repoWithCount struct {
	*store.Repository
	SessionCount int `json:"session_count"`
}

func handleListRepos(ctx context.Context, cfg Config) (*mcp.CallToolResult, any, error) {
	repos, err := cfg.Store.ListRepositories(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list repos: %w", err)
	}
	if repos == nil {
		repos = []*store.Repository{}
	}
	return jsonResult(repos)
}

func handleGetRepo(ctx context.Context, cfg Config, p getRepoParams) (*mcp.CallToolResult, any, error) {
	if p.RepoID == "" {
		return toolErr(fmt.Errorf("repo_id is required"))
	}
	repo, err := cfg.Store.GetRepository(ctx, p.RepoID)
	if errors.Is(err, store.ErrNotFound) {
		return toolErr(fmt.Errorf("repository not found: %s", p.RepoID))
	}
	if err != nil {
		return nil, nil, fmt.Errorf("get repo: %w", err)
	}
	n, err := cfg.Store.RepositorySessionCount(ctx, p.RepoID)
	if err != nil {
		return nil, nil, fmt.Errorf("get repo session count: %w", err)
	}
	return jsonResult(repoWithCount{Repository: repo, SessionCount: n})
}
