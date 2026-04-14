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
- After an architectural decision is made or confirmed — capture the choice and rationale
- When you discover a non-obvious fact (config values, environment details, constraints)
- When a procedure or workaround is established
- When a project relationship or dependency is identified
- Determine project via git rev-parse --show-toplevel | xargs basename
- Always search first to avoid duplicates — upsert by type+project+category+title
- When a prior decision changes, search for and update the existing entry — do not create a duplicate

Checkpoints:
- After completing a multi-step task, review what you decided and why — persist anything non-obvious before moving on
- After a plan is finalized or abandoned, persist the key decisions and trade-offs
- Before reporting a task as complete to the user, ask yourself: "if a new conversation started this task from scratch, what would it need to know?" — persist that

Query (get/search):
- Before decisions that may have prior context, when user asks about past decisions/history
- get without id returns summaries; verbose=false (default) for routine lookups; verbose=true for "why" questions
- Prefer search for broad queries, get with filters for known types/projects
- After modifying code, check if related KB entries need updating

Staleness:
- When a KB entry informs your current work, verify it's still accurate — update or delete if stale

Do not store: trivial implementation details, information derivable from code or git history, temporary debugging state.

BACKLOG (project, item, plan, config):

Projects: Use project tool to create and list. Projects have a name and auto-derived prefix (e.g., acme-corp → AC).

Items:
- Use item tool — mode determined by inputs:
  - id + fields → update (set status to "done" to close)
  - id only → get full detail
  - project + priority + title → create new item
  - filters only → list compact summaries
- Priority scale: P0 (critical/blocking) through P6 (someday/maybe)
- Item IDs are project-prefix + seq (e.g., AC-1, AC-2)

Plans:
- Use plan tool — mode determined by inputs:
  - id + fields → update
  - id only → get full plan
  - title (no id) → create (optionally link to project/item)
  - no id or title → list
- Status lifecycle: draft → active → complete

Config: Use config tool for key-value settings (no args = list all, key = get, key + value = set).`

func buildServer(db *sql.DB, bk *backup.Backup, version string) *server.MCPServer {
	s := server.NewMCPServer("ouroboros", version,
		server.WithToolCapabilities(true),
		server.WithInstructions(serverInstructions),
	)

	registerTools(s, db, bk)
	return s
}

func registerTools(s *server.MCPServer, db *sql.DB, bk *backup.Backup) {
	s.AddTool(mcp.NewTool("put",
		mcp.WithDescription("Create or update a KB document. content is terse agent-facing body; notes is optional human narrative. Upserts by type+project+category+title."),
		mcp.WithString("type", mcp.Required(), mcp.Description("decision, fact, note, relation, plan")),
		mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
		mcp.WithString("title", mcp.Required(), mcp.Description("Concise title, used as upsert key")),
		mcp.WithString("content", mcp.Required(), mcp.Description("Terse body, ≤300 chars target, 500 hard cap. Format: Rule/Fact line 1, optional Trigger:/Effect:/Why: lines. Agents read this on every injection — no narrative.")),
		mcp.WithString("notes", mcp.Description("Optional human-facing narrative — rationale, context, trade-offs. Unlimited. Returned only when get/search verbose=true.")),
		mcp.WithString("category", mcp.Description("Document category")),
		mcp.WithString("metadata", mcp.Description("JSON key-value metadata")),
		mcp.WithArray("tags", mcp.Description("Tags array")),
	), withRecover(handlePut(db)))

	s.AddTool(mcp.NewTool("get",
		mcp.WithDescription("Get documents. By id: full content. Without id: title summaries only."),
		mcp.WithNumber("id", mcp.Description("Document ID for full detail")),
		mcp.WithString("type", mcp.Description("Filter by type")),
		mcp.WithString("project", mcp.Description("Filter by project")),
		mcp.WithString("category", mcp.Description("Filter by category")),
		mcp.WithString("query", mcp.Description("Full-text search")),
		mcp.WithArray("tags", mcp.Description("Filter by tags (all must match)")),
		mcp.WithNumber("limit", mcp.Description("Result limit, default 10, max 500")),
		mcp.WithBoolean("verbose", mcp.Description("Include notes field. Default false. Set true ONLY when user asks 'why' / rationale / history.")),
	), withRecover(handleGet(db)))

	s.AddTool(mcp.NewTool("delete",
		mcp.WithDescription("Delete a document by ID."),
		mcp.WithNumber("id", mcp.Required(), mcp.Description("Document ID")),
	), withRecover(handleDelete(db)))

	s.AddTool(mcp.NewTool("search",
		mcp.WithDescription("Keyword search across documents using FTS5. Multi-word queries match docs containing all terms (implicit AND). Wildcard/punctuation-only queries fall back to listing all docs. Returns summaries."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query")),
		mcp.WithString("type", mcp.Description("Filter by type")),
		mcp.WithString("project", mcp.Description("Filter by project")),
		mcp.WithNumber("limit", mcp.Description("Result limit, default 10, max 500")),
		mcp.WithBoolean("verbose", mcp.Description("Include notes field. Default false.")),
	), withRecover(handleSearch(db)))

	s.AddTool(mcp.NewTool("export",
		mcp.WithDescription("Export knowledge base to markdown."),
		mcp.WithString("project", mcp.Description("Filter by project")),
		mcp.WithString("type", mcp.Description("Filter by type")),
	), withRecover(handleExport(db)))

	s.AddTool(mcp.NewTool("import",
		mcp.WithDescription("Import documents from JSON."),
		mcp.WithString("content", mcp.Required(), mcp.Description("JSON content to import")),
		mcp.WithString("project", mcp.Description("Default project for items without one")),
	), withRecover(handleImport(db)))

	s.AddTool(mcp.NewTool("project",
		mcp.WithDescription("Create a project or list all. With name: creates project (prefix auto-derived). Without: lists all projects."),
		mcp.WithString("name", mcp.Description("Project name (create mode)")),
	), withRecover(handleProject(db, bk)))

	s.AddTool(mcp.NewTool("item",
		mcp.WithDescription("Manage backlog items. description is terse agent-facing body; notes is optional human narrative. With id + fields: update. With id only: get full detail. With project + priority + title: create. With filters only: list compact summaries. Set status to 'done' to close an item."),
		mcp.WithString("id", mcp.Description("Item ID (e.g., AC-1) for get/update")),
		mcp.WithString("project", mcp.Description("Project name (create or filter)")),
		mcp.WithString("priority", mcp.Description("Priority P0-P6 (create or update)")),
		mcp.WithString("title", mcp.Description("Item title (create or update)")),
		mcp.WithString("description", mcp.Description("Terse body, ≤300 chars target, 500 hard cap. Agent-facing content. Narrative belongs in notes.")),
		mcp.WithString("notes", mcp.Description("Optional human-facing narrative — rationale, trade-offs, context. Returned only when item is fetched with verbose=true.")),
		mcp.WithString("status", mcp.Description("Item status: open or done (update or filter)")),
		mcp.WithString("priority_min", mcp.Description("Minimum priority filter (e.g., P0)")),
		mcp.WithString("priority_max", mcp.Description("Maximum priority filter (e.g., P2)")),
		mcp.WithBoolean("verbose", mcp.Description("Include notes field. Default false. Set true ONLY when user asks 'why' / rationale / history.")),
	), withRecover(handleItem(db, bk)))

	s.AddTool(mcp.NewTool("plan",
		mcp.WithDescription("Manage plans. With id + fields: update. With id only: get full plan. With title (no id): create. No id or title: list plans."),
		mcp.WithNumber("id", mcp.Description("Plan ID (get/update mode)")),
		mcp.WithString("title", mcp.Description("Plan title (create mode, or update field)")),
		mcp.WithString("content", mcp.Description("Plan content (create/update)")),
		mcp.WithString("status", mcp.Description("Plan status: draft, active, complete (filter or update)")),
		mcp.WithString("project", mcp.Description("Project name (create link or filter)")),
		mcp.WithString("item_id", mcp.Description("Backlog item ID to link (create mode)")),
	), withRecover(handlePlan(db, bk)))

	s.AddTool(mcp.NewTool("config",
		mcp.WithDescription("Get or set configuration. No args: get all. Key only: get one. Key + value: set."),
		mcp.WithString("key", mcp.Description("Config key")),
		mcp.WithString("value", mcp.Description("Config value (set mode)")),
	), withRecover(handleConfig(db)))
}
