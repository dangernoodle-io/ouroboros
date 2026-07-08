package app

import (
	"database/sql"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dangernoodle.io/ouroboros/internal/backup"
)

const (
	descFilterProjects = "Filter by project names"
	descFilterTypes    = "Filter by types (kb only)"
	descFilterCats     = "Filter by categories (kb only)"
	descLimit          = "Limit, default 10, max 500"
	descVerbose        = "Include notes (default: false)"
	descDomain         = `Required: "kb" or "backlog"`
	descPriorityMin    = "Min priority P0-P6 (backlog only)"
	descPriorityMax    = "Max priority P0-P6 (backlog only)"
	descStatus         = "open or done (backlog only)"
	descComponent      = "Component tag (subproject/plugin) filter (backlog only)"
)

const serverInstructions = `Persist decisions and track work items across conversations.
get/search are reads (required domain: kb|backlog); kb/backlog are writes.

- Search before writing — avoid duplicates; kb upserts by type+project+category+title.
- Default response is summary; verbose=true only when full content/notes are needed.
- Checkpoint after multi-step tasks; persist non-obvious decisions, update or delete stale ones.`

// buildServer creates a new MCP server with all tools registered at startup.
func buildServer(db *sql.DB, bk *backup.Backup, version string) *server.MCPServer {
	s := server.NewMCPServer("ouroboros", version,
		server.WithToolCapabilities(true),
		server.WithInstructions(serverInstructions),
	)

	s.AddTool(mcp.NewTool("get",
		mcp.WithDescription("Fetch entries by ID or filters (required domain: kb|backlog). ids[] returns exact matches; omit ids for a filtered list."),
		mcp.WithString("domain", mcp.Required(), mcp.Description(descDomain)),
		mcp.WithArray("ids", mcp.Description("IDs to fetch (kb: document IDs, backlog: item IDs)")),
		mcp.WithArray("types", mcp.Description(descFilterTypes), mcp.Items(map[string]any{"type": "string"})),
		mcp.WithArray("projects", mcp.Description(descFilterProjects)),
		mcp.WithArray("categories", mcp.Description(descFilterCats), mcp.Items(map[string]any{"type": "string"})),
		mcp.WithString("query", mcp.Description("Full-text search (kb only)")),
		mcp.WithArray("tags", mcp.Description("Filter by tags, all match (kb only)")),
		mcp.WithString("priority_min", mcp.Description(descPriorityMin)),
		mcp.WithString("priority_max", mcp.Description(descPriorityMax)),
		mcp.WithString("status", mcp.Description(descStatus)),
		mcp.WithString("component", mcp.Description(descComponent)),
		mcp.WithNumber("limit", mcp.Description(descLimit)),
		mcp.WithBoolean("verbose", mcp.Description(descVerbose)),
		toolAnnotation(mcp.ToBoolPtr(true), nil, nil),
	), withRecover(handleGet(db)))

	s.AddTool(mcp.NewTool("search",
		mcp.WithDescription("Full-text search over entries (required domain: kb|backlog). domain=kb supports query or a batched queries[]; domain=backlog takes a single query over title/description/notes."),
		mcp.WithString("domain", mcp.Required(), mcp.Description(descDomain)),
		mcp.WithString("query", mcp.Description("Single query")),
		mcp.WithArray("queries", mcp.Description("Batch queries sharing filters, kb only; response is positional [[...], [...]]")),
		mcp.WithArray("types", mcp.Description(descFilterTypes), mcp.Items(map[string]any{"type": "string"})),
		mcp.WithArray("projects", mcp.Description(descFilterProjects)),
		mcp.WithArray("categories", mcp.Description(descFilterCats), mcp.Items(map[string]any{"type": "string"})),
		mcp.WithString("priority_min", mcp.Description(descPriorityMin)),
		mcp.WithString("priority_max", mcp.Description(descPriorityMax)),
		mcp.WithString("status", mcp.Description(descStatus)),
		mcp.WithString("component", mcp.Description(descComponent)),
		mcp.WithNumber("limit", mcp.Description(descLimit)),
		toolAnnotation(mcp.ToBoolPtr(true), nil, nil),
	), withRecover(handleSearch(db)))

	s.AddTool(mcp.NewTool("kb",
		mcp.WithDescription("Create or update knowledge entries via entries[], upserting by type+project+category+title. Reads live under get/search domain=kb."),
		mcp.WithArray("entries", mcp.Required(), mcp.Description("Documents to upsert")),
		toolAnnotation(nil, nil, mcp.ToBoolPtr(true)),
	), withRecover(handleKB(db)))

	s.AddTool(mcp.NewTool("backlog",
		mcp.WithDescription("Create, update, or delete backlog items: entries[] (id present = update, else create) or delete_ids[]. Reads live under get/search domain=backlog."),
		mcp.WithArray("entries", mcp.Description("Items to create/update: {id?}, project, priority, title, description?, notes?, component?, status?")),
		mcp.WithArray("delete_ids", mcp.Description("Item IDs to delete")),
		toolAnnotation(nil, mcp.ToBoolPtr(true), nil),
	), withRecover(handleBacklog(db, bk)))

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
