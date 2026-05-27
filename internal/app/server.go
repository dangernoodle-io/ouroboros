package app

import (
	"database/sql"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dangernoodle.io/ouroboros/internal/backup"
)

const serverInstructions = `Persist decisions and track work items across conversations.
- Search before put/create — avoid duplicates; upsert by type+project+category+title.
- Default response is summary; verbose=true only when full content/notes are needed.
- Checkpoint after multi-step tasks; persist non-obvious decisions.
- Update or delete stale entries.`

// buildServer creates a new MCP server with all tools registered at startup.
func buildServer(db *sql.DB, bk *backup.Backup, version string) *server.MCPServer {
	s := server.NewMCPServer("ouroboros", version,
		server.WithToolCapabilities(true),
		server.WithInstructions(serverInstructions),
	)

	s.AddTool(mcp.NewTool("get",
		mcp.WithDescription("Get documents: ids array for fetch, or filters for list."),
		mcp.WithArray("ids", mcp.Description("Document IDs (batch fetch)")),
		mcp.WithString("type", mcp.Description("Filter by type")),
		mcp.WithArray("projects", mcp.Description("Filter by project names")),
		mcp.WithString("category", mcp.Description("Filter by category")),
		mcp.WithString("query", mcp.Description("Full-text search")),
		mcp.WithArray("tags", mcp.Description("Filter by tags (all match)")),
		mcp.WithNumber("limit", mcp.Description("Limit, default 10, max 500")),
		mcp.WithBoolean("verbose", mcp.Description("Include notes (default: false)")),
		toolAnnotation(mcp.ToBoolPtr(true), nil, nil),
	), withRecover(handleGet(db)))

	s.AddTool(mcp.NewTool("search",
		mcp.WithDescription("Keyword search (FTS5). Single query or queries[] batch. Multi-word = AND."),
		mcp.WithString("query", mcp.Description("Single query")),
		mcp.WithArray("queries", mcp.Description("Batch queries sharing filters; response is positional [[...], [...]]")),
		mcp.WithString("type", mcp.Description("Filter by type")),
		mcp.WithArray("projects", mcp.Description("Filter by project names")),
		mcp.WithNumber("limit", mcp.Description("Limit per query, default 10, max 500")),
		toolAnnotation(mcp.ToBoolPtr(true), nil, nil),
	), withRecover(handleSearch(db)))

	s.AddTool(mcp.NewTool("put",
		mcp.WithDescription("Create/update KB documents (batch). Each: type, project, title, content, notes?, category?, tags?, metadata?"),
		mcp.WithArray("entries", mcp.Required(), mcp.Description("Documents to upsert")),
		toolAnnotation(nil, nil, mcp.ToBoolPtr(true)),
	), withRecover(handlePut(db)))

	s.AddTool(mcp.NewTool("item",
		mcp.WithDescription("Manage backlog items: ids fetch, entries create/update, or filters list."),
		mcp.WithArray("ids", mcp.Description("Item IDs to fetch")),
		mcp.WithArray("entries", mcp.Description("Items to create/update: {id?}, project, priority, title, description?, notes?, component?, status?")),
		mcp.WithArray("delete_ids", mcp.Description("Item IDs to delete")),
		mcp.WithArray("projects", mcp.Description("Filter by project names")),
		mcp.WithString("priority_min", mcp.Description("Min priority (P0–P6)")),
		mcp.WithString("priority_max", mcp.Description("Max priority (P0–P6)")),
		mcp.WithString("status", mcp.Description("open or done")),
		mcp.WithString("component", mcp.Description("Component tag (subproject/plugin); filter or set")),
		mcp.WithBoolean("verbose", mcp.Description("Include notes (default: false)")),
		toolAnnotation(nil, mcp.ToBoolPtr(true), nil),
	), withRecover(handleItem(db, bk)))

	return s
}

// toolAnnotation constructs a mcp.WithToolAnnotation option with only the
// specified hint fields set (others remain nil to drop from JSON via omitempty).
func toolAnnotation(readOnly, destructive, idempotent *bool) mcp.ToolOption {
	return mcp.WithToolAnnotation(mcp.ToolAnnotation{
		ReadOnlyHint:    readOnly,
		DestructiveHint: destructive,
		IdempotentHint:  idempotent,
		OpenWorldHint:   nil, // always nil: local SQLite, no external calls
	})
}
