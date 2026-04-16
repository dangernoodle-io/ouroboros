package app

import (
	"database/sql"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dangernoodle.io/ouroboros/internal/backup"
)

const serverInstructions = `Project knowledge base and backlog management — persist decisions across conversations and track work items.

All tools below are MCP tools — call them directly, not via CLI. The ouroboros binary only supports "query" and "items" subcommands for hook integration; it has no CLI for project/item/plan/config management.

KNOWLEDGE BASE (put, get, delete, search, export, import):

Store immediately (put):
- Architectural decisions with rationale
- Non-obvious facts (config values, environment details, constraints)
- Established procedures or workarounds
- Project relationships or dependencies
- Always search first to avoid duplicates; upsert by type+project+category+title
- Update existing entries when decisions change — do not duplicate

Query (get/search):
- Before decisions with prior context; get/search return summaries by default
- verbose=true for detailed context; verbose=false (default) for routine lookups
- Prefer search for broad queries, get with filters for known types/projects

Checkpoints: After multi-step tasks, finalized plans, or before reporting completion — persist non-obvious decisions.

Staleness: Verify KB entries match current work; update or delete if stale.

Do not store: trivial details, derivable information, or temporary state.

BACKLOG (project, item, plan, config):

Projects: Create with name; auto-derived prefix (e.g., acme-corp → AC).

Items:
- Batch mode: ids=[] fetch, entries=[{...}] create/update, filters list
- id present = update; id absent = create (needs project+priority+title)
- Priority: P0 (critical) through P6 (someday). IDs: prefix+seq (e.g., AC-1)

Plans:
- Batch mode: ids=[] fetch, entries=[{...}] create/update, filters list
- id present = update; id absent = create (needs title)
- Status: draft → active → complete

Config: No args = list all; key = get; key+value = set.`

func buildServer(db *sql.DB, bk *backup.Backup, version string) *server.MCPServer {
	s := server.NewMCPServer("ouroboros", version,
		server.WithToolCapabilities(true),
		server.WithInstructions(serverInstructions),
	)

	registerTools(s, db, bk)
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

func registerTools(s *server.MCPServer, db *sql.DB, bk *backup.Backup) {
	s.AddTool(mcp.NewTool("put",
		mcp.WithDescription("Create/update KB documents (batch). Each: type, project, title, content, notes?, category?, tags?, metadata?"),
		mcp.WithArray("entries", mcp.Required(), mcp.Description("Documents to upsert")),
		toolAnnotation(nil, nil, mcp.ToBoolPtr(true)),
	), withRecover(handlePut(db)))

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

	s.AddTool(mcp.NewTool("delete",
		mcp.WithDescription("Delete a document."),
		mcp.WithNumber("id", mcp.Required(), mcp.Description("Document ID")),
		toolAnnotation(nil, mcp.ToBoolPtr(true), mcp.ToBoolPtr(true)),
	), withRecover(handleDelete(db)))

	s.AddTool(mcp.NewTool("search",
		mcp.WithDescription("Keyword search (FTS5). Single query or queries[] batch. Multi-word = AND."),
		mcp.WithString("query", mcp.Description("Single query")),
		mcp.WithArray("queries", mcp.Description("Batch queries sharing filters; response is positional [[...], [...]]")),
		mcp.WithString("type", mcp.Description("Filter by type")),
		mcp.WithArray("projects", mcp.Description("Filter by project names")),
		mcp.WithNumber("limit", mcp.Description("Limit per query, default 10, max 500")),
		toolAnnotation(mcp.ToBoolPtr(true), nil, nil),
	), withRecover(handleSearch(db)))

	s.AddTool(mcp.NewTool("export",
		mcp.WithDescription("Export KB to markdown."),
		mcp.WithArray("projects", mcp.Description("Filter by project names")),
		mcp.WithString("type", mcp.Description("Filter by type")),
		toolAnnotation(mcp.ToBoolPtr(true), nil, nil),
	), withRecover(handleExport(db)))

	s.AddTool(mcp.NewTool("import",
		mcp.WithDescription("Import documents from JSON."),
		mcp.WithString("content", mcp.Required(), mcp.Description("JSON to import")),
		mcp.WithString("project", mcp.Description("Default project if not specified")),
		toolAnnotation(nil, nil, nil),
	), withRecover(handleImport(db)))

	s.AddTool(mcp.NewTool("project",
		mcp.WithDescription("Create project (with name) or list all (no args)."),
		mcp.WithString("name", mcp.Description("Project name")),
		toolAnnotation(nil, nil, nil),
	), withRecover(handleProject(db, bk)))

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

	s.AddTool(mcp.NewTool("plan",
		mcp.WithDescription("Manage plans: ids fetch, entries create/update, or filters list."),
		mcp.WithArray("ids", mcp.Description("Plan IDs to fetch")),
		mcp.WithArray("entries", mcp.Description("Plans to create/update: {id?}, title, content?, status?, project?, item_id?")),
		mcp.WithArray("projects", mcp.Description("Filter by project names")),
		mcp.WithString("status", mcp.Description("draft, active, or complete")),
		toolAnnotation(nil, nil, nil),
	), withRecover(handlePlan(db, bk)))

	s.AddTool(mcp.NewTool("config",
		mcp.WithDescription("Get/set config: no args=list, key=get, key+value=set."),
		mcp.WithString("key", mcp.Description("Config key")),
		mcp.WithString("value", mcp.Description("Config value")),
		toolAnnotation(nil, nil, mcp.ToBoolPtr(true)),
	), withRecover(handleConfig(db)))
}
