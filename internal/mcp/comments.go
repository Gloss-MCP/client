package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gloss-mcp/client/internal/store"
)

func registerCommentTools(srv *mcp.Server, cfg Config) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "add_comment",
		Description: "Add a comment to a thread. AI-authored; author_type is always \"ai\".",
	}, func(ctx context.Context, req *mcp.CallToolRequest, p addCommentParams) (*mcp.CallToolResult, any, error) {
		return handleAddComment(ctx, req, cfg, p)
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "edit_comment",
		Description: "Replace the body of an existing comment.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, p editCommentParams) (*mcp.CallToolResult, any, error) {
		return handleEditComment(ctx, cfg, p)
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "delete_comment",
		Description: "Soft-delete a comment. The row is retained so replies stay attached.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, p deleteCommentParams) (*mcp.CallToolResult, any, error) {
		return handleDeleteComment(ctx, cfg, p)
	})
}

type addCommentParams struct {
	ThreadID        string `json:"thread_id"                  jsonschema:"required,Thread to add the comment to"`
	Body            string `json:"body"                       jsonschema:"required,Comment body (markdown)"`
	ParentCommentID string `json:"parent_comment_id,omitempty" jsonschema:"ID of the parent comment for nested replies"`
	AuthorAgent     string `json:"author_agent,omitempty"      jsonschema:"Caller identity (e.g. claude-opus-4); defaults to MCP clientInfo.name"`
}

type editCommentParams struct {
	CommentID string `json:"comment_id" jsonschema:"required,Comment to edit"`
	Body      string `json:"body"       jsonschema:"required,New body text (markdown)"`
}

type deleteCommentParams struct {
	CommentID string `json:"comment_id" jsonschema:"required,Comment to soft-delete"`
}

func handleAddComment(ctx context.Context, req *mcp.CallToolRequest, cfg Config, p addCommentParams) (*mcp.CallToolResult, any, error) {
	if p.ThreadID == "" || p.Body == "" {
		return toolErr(fmt.Errorf("thread_id and body are required"))
	}
	c, err := cfg.Store.AddComment(ctx, store.AddCommentParams{
		ThreadID:        p.ThreadID,
		ParentCommentID: p.ParentCommentID,
		AuthorType:      store.AuthorAI,
		AuthorAgent:     agentFrom(req, p.AuthorAgent),
		Body:            p.Body,
	})
	if errors.Is(err, store.ErrNotFound) {
		return toolErr(fmt.Errorf("thread not found: %s", p.ThreadID))
	}
	if err != nil {
		return nil, nil, fmt.Errorf("add comment: %w", err)
	}
	return jsonResult(c)
}

func handleEditComment(ctx context.Context, cfg Config, p editCommentParams) (*mcp.CallToolResult, any, error) {
	if p.CommentID == "" || p.Body == "" {
		return toolErr(fmt.Errorf("comment_id and body are required"))
	}
	c, err := cfg.Store.UpdateCommentBody(ctx, p.CommentID, p.Body)
	if errors.Is(err, store.ErrNotFound) {
		return toolErr(fmt.Errorf("comment not found: %s", p.CommentID))
	}
	if errors.Is(err, store.ErrCommentDeleted) {
		return toolErr(fmt.Errorf("comment is deleted: %s", p.CommentID))
	}
	if err != nil {
		return nil, nil, fmt.Errorf("edit comment: %w", err)
	}
	return jsonResult(c)
}

func handleDeleteComment(ctx context.Context, cfg Config, p deleteCommentParams) (*mcp.CallToolResult, any, error) {
	if p.CommentID == "" {
		return toolErr(fmt.Errorf("comment_id is required"))
	}
	if err := cfg.Store.SoftDeleteComment(ctx, p.CommentID); errors.Is(err, store.ErrNotFound) {
		return toolErr(fmt.Errorf("comment not found: %s", p.CommentID))
	} else if err != nil {
		return nil, nil, fmt.Errorf("delete comment: %w", err)
	}
	return jsonResult(map[string]string{"status": "deleted", "comment_id": p.CommentID})
}
