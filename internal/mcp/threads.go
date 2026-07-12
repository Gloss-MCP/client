package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gloss-mcp/client/internal/store"
)

func registerThreadTools(srv *mcp.Server, cfg Config) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_thread",
		Description: "Create a new review thread anchored to a position in a tracked file. The file_path must be in the repository's tracked file set.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, p createThreadParams) (*mcp.CallToolResult, any, error) {
		return handleCreateThread(ctx, req, cfg, p)
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_thread",
		Description: "Get a thread by ID, including its full comment chain.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, p getThreadParams) (*mcp.CallToolResult, any, error) {
		return handleGetThread(ctx, cfg, p)
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_threads",
		Description: "List threads in a session, with optional filters for file, directory, file type, anchor status, or author.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, p listThreadsParams) (*mcp.CallToolResult, any, error) {
		return handleListThreads(ctx, cfg, p)
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "resolve_thread",
		Description: "Mark a thread's anchor as resolved.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, p threadIDParams) (*mcp.CallToolResult, any, error) {
		return handleSetAnchorStatus(ctx, cfg, p.ThreadID, store.AnchorResolved)
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "reopen_thread",
		Description: "Reopen a resolved or orphaned thread, setting its anchor status back to active.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, p threadIDParams) (*mcp.CallToolResult, any, error) {
		return handleSetAnchorStatus(ctx, cfg, p.ThreadID, store.AnchorActive)
	})
}

// anchorParams is the polymorphic anchor shape accepted by create_thread.
// The type field selects the variant; unused fields are ignored.
type anchorParams struct {
	Type      string  `json:"type"                 jsonschema:"required,Anchor variant: line, region, time, or region_time"`
	StartLine int     `json:"start_line,omitempty" jsonschema:"Line anchor: first line (1-based, inclusive)"`
	EndLine   int     `json:"end_line,omitempty"   jsonschema:"Line anchor: last line (1-based, inclusive)"`
	X         float64 `json:"x,omitempty"          jsonschema:"Region anchor: X offset as a fraction of width"`
	Y         float64 `json:"y,omitempty"          jsonschema:"Region anchor: Y offset as a fraction of height"`
	Width     float64 `json:"width,omitempty"      jsonschema:"Region anchor: width as a fraction"`
	Height    float64 `json:"height,omitempty"     jsonschema:"Region anchor: height as a fraction"`
	StartTime float64 `json:"start_time,omitempty" jsonschema:"Time/region-time anchor: start in seconds"`
	EndTime   float64 `json:"end_time,omitempty"   jsonschema:"Time/region-time anchor: end in seconds"`
}

type createThreadParams struct {
	SessionID   string       `json:"session_id"             jsonschema:"required,Session to attach the thread to"`
	FilePath    string       `json:"file_path"              jsonschema:"required,Repo-relative path of the file to annotate"`
	Anchor      anchorParams `json:"anchor"                 jsonschema:"required,Position within the file"`
	Body        string       `json:"body"                   jsonschema:"required,Opening comment body (markdown)"`
	AuthorAgent string       `json:"author_agent,omitempty" jsonschema:"Caller identity; defaults to MCP clientInfo.name"`
}

type getThreadParams struct {
	ThreadID string `json:"thread_id" jsonschema:"required,Thread ID"`
}

type listThreadsParams struct {
	SessionID   string `json:"session_id"             jsonschema:"required,Session to list threads from"`
	FilePath    string `json:"file_path,omitempty"    jsonschema:"Exact file path filter"`
	Directory   string `json:"directory,omitempty"    jsonschema:"Directory prefix filter (e.g. src/)"`
	FileType    string `json:"file_type,omitempty"    jsonschema:"File extension filter (e.g. go or .go)"`
	AnchorStatus string `json:"anchor_status,omitempty" jsonschema:"Filter by anchor status: active, orphaned, or resolved"`
	AuthorType  string `json:"author_type,omitempty"  jsonschema:"Filter by root comment author type: human or ai"`
	AuthorAgent string `json:"author_agent,omitempty" jsonschema:"Filter by root comment author_agent value"`
}

type threadIDParams struct {
	ThreadID string `json:"thread_id" jsonschema:"required,Thread ID"`
}

type threadWithComments struct {
	*store.Thread
	Comments []*store.Comment `json:"comments"`
}

func handleCreateThread(ctx context.Context, req *mcp.CallToolRequest, cfg Config, p createThreadParams) (*mcp.CallToolResult, any, error) {
	if p.SessionID == "" || p.FilePath == "" || p.Body == "" {
		return toolErr(fmt.Errorf("session_id, file_path, and body are required"))
	}

	paths, err := listTrackedFiles(cfg)
	if err != nil {
		return nil, nil, err
	}
	if !containsPath(paths, p.FilePath) {
		return toolErr(fmt.Errorf("file not in tracked set: %s", p.FilePath))
	}

	anchor, err := parseAnchor(p.Anchor)
	if err != nil {
		return toolErr(err)
	}

	content, err := os.ReadFile(filepath.Join(cfg.Root, filepath.FromSlash(p.FilePath)))
	if err != nil {
		return nil, nil, fmt.Errorf("read file %s: %w", p.FilePath, err)
	}
	snap, err := resolveSnapshot(ctx, cfg, p.FilePath, content)
	if err != nil {
		return nil, nil, err
	}

	thread, _, err := cfg.Store.CreateThread(ctx, store.CreateThreadParams{
		SessionID:      p.SessionID,
		FileSnapshotID: snap.ID,
		Anchor:         anchor,
		CreatedBy:      agentFrom(req, p.AuthorAgent),
		Body:           p.Body,
		AuthorType:     store.AuthorAI,
		AuthorAgent:    agentFrom(req, p.AuthorAgent),
	})
	if errors.Is(err, store.ErrNotFound) {
		return toolErr(fmt.Errorf("session not found: %s", p.SessionID))
	}
	if err != nil {
		return nil, nil, fmt.Errorf("create thread: %w", err)
	}
	return jsonResult(thread)
}

func handleGetThread(ctx context.Context, cfg Config, p getThreadParams) (*mcp.CallToolResult, any, error) {
	if p.ThreadID == "" {
		return toolErr(fmt.Errorf("thread_id is required"))
	}
	thread, comments, err := cfg.Store.GetThread(ctx, p.ThreadID)
	if errors.Is(err, store.ErrNotFound) {
		return toolErr(fmt.Errorf("thread not found: %s", p.ThreadID))
	}
	if err != nil {
		return nil, nil, fmt.Errorf("get thread: %w", err)
	}
	if comments == nil {
		comments = []*store.Comment{}
	}
	return jsonResult(threadWithComments{Thread: thread, Comments: comments})
}

func handleListThreads(ctx context.Context, cfg Config, p listThreadsParams) (*mcp.CallToolResult, any, error) {
	if p.SessionID == "" {
		return toolErr(fmt.Errorf("session_id is required"))
	}
	threads, err := cfg.Store.ListThreads(ctx, p.SessionID, store.ListThreadsFilter{
		FilePath:     p.FilePath,
		Directory:    p.Directory,
		FileType:     p.FileType,
		AnchorStatus: store.AnchorStatus(p.AnchorStatus),
		AuthorType:   store.AuthorType(p.AuthorType),
		AuthorAgent:  p.AuthorAgent,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("list threads: %w", err)
	}
	if threads == nil {
		threads = []*store.Thread{}
	}
	return jsonResult(threads)
}

func handleSetAnchorStatus(ctx context.Context, cfg Config, threadID string, status store.AnchorStatus) (*mcp.CallToolResult, any, error) {
	if threadID == "" {
		return toolErr(fmt.Errorf("thread_id is required"))
	}
	thread, err := cfg.Store.SetAnchorStatus(ctx, threadID, status)
	if errors.Is(err, store.ErrNotFound) {
		return toolErr(fmt.Errorf("thread not found: %s", threadID))
	}
	if err != nil {
		return nil, nil, fmt.Errorf("set anchor status: %w", err)
	}
	return jsonResult(thread)
}

// parseAnchor converts the tool's anchorParams to a store.Anchor sum type.
func parseAnchor(p anchorParams) (store.Anchor, error) {
	switch p.Type {
	case "line":
		if p.StartLine < 1 || p.EndLine < p.StartLine {
			return nil, fmt.Errorf("line anchor requires start_line >= 1 and end_line >= start_line")
		}
		return store.LineAnchor{StartLine: p.StartLine, EndLine: p.EndLine}, nil
	case "region":
		return store.RegionAnchor{X: p.X, Y: p.Y, Width: p.Width, Height: p.Height}, nil
	case "time":
		if p.EndTime < p.StartTime {
			return nil, fmt.Errorf("time anchor requires end_time >= start_time")
		}
		return store.TimeAnchor{StartTime: p.StartTime, EndTime: p.EndTime}, nil
	case "region_time":
		return store.RegionTimeAnchor{
			X: p.X, Y: p.Y, Width: p.Width, Height: p.Height,
			StartTime: p.StartTime, EndTime: p.EndTime,
		}, nil
	default:
		return nil, fmt.Errorf("unknown anchor type %q; must be line, region, time, or region_time", p.Type)
	}
}

// resolveSnapshot finds the current snapshot for path+content or creates
// one. Mirrors server.resolveFileSnapshot without exporting it.
func resolveSnapshot(ctx context.Context, cfg Config, path string, content []byte) (*store.FileSnapshot, error) {
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])
	snap, err := cfg.Store.FindFileSnapshot(ctx, cfg.RepoID, path, hash)
	if errors.Is(err, store.ErrNotFound) {
		return cfg.Store.CreateFileSnapshot(ctx, cfg.RepoID, path, hash, "")
	}
	return snap, err
}

// containsPath reports whether path is present in the sorted slice.
func containsPath(paths []string, path string) bool {
	i := sort.SearchStrings(paths, path)
	return i < len(paths) && paths[i] == path
}
