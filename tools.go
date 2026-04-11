package main

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dangernoodle.io/ouroboros/internal/backup"
)

var bk *backup.Backup

func registerTools(s *server.MCPServer) {
	s.AddTool(mcp.NewTool("put",
		mcp.WithDescription("Create or update a document. Upserts by type+project+category+title."),
		mcp.WithString("type", mcp.Required(), mcp.Description("Document type (e.g. decision, fact, note, relation)")),
		mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
		mcp.WithString("title", mcp.Required(), mcp.Description("Document title (summary for decisions, key for facts)")),
		mcp.WithString("content", mcp.Description("Document content (rationale, value, body, description)")),
		mcp.WithString("category", mcp.Description("Document category")),
		mcp.WithString("metadata", mcp.Description("JSON string of key-value metadata")),
		mcp.WithArray("tags", mcp.Description("Tags array")),
	), withRecover(handlePut))

	s.AddTool(mcp.NewTool("get",
		mcp.WithDescription("Get documents. By id returns full content. Without id returns summaries (no content) to conserve tokens."),
		mcp.WithNumber("id", mcp.Description("Document ID for full detail")),
		mcp.WithString("type", mcp.Description("Filter by type")),
		mcp.WithString("project", mcp.Description("Filter by project")),
		mcp.WithString("category", mcp.Description("Filter by category")),
		mcp.WithString("query", mcp.Description("Full-text search")),
		mcp.WithArray("tags", mcp.Description("Filter by tags (all must match)")),
		mcp.WithNumber("limit", mcp.Description("Result limit (default 50, max 500)")),
	), withRecover(handleGet))

	s.AddTool(mcp.NewTool("delete",
		mcp.WithDescription("Delete a document by ID."),
		mcp.WithNumber("id", mcp.Required(), mcp.Description("Document ID")),
	), withRecover(handleDelete))

	s.AddTool(mcp.NewTool("search",
		mcp.WithDescription("Full-text search across all documents. Returns summaries."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query")),
		mcp.WithString("type", mcp.Description("Filter by type")),
		mcp.WithString("project", mcp.Description("Filter by project")),
		mcp.WithNumber("limit", mcp.Description("Result limit (default 50, max 500)")),
	), withRecover(handleSearch))

	s.AddTool(mcp.NewTool("export",
		mcp.WithDescription("Export knowledge base to markdown."),
		mcp.WithString("project", mcp.Description("Filter by project")),
		mcp.WithString("type", mcp.Description("Filter by type")),
	), withRecover(handleExport))

	s.AddTool(mcp.NewTool("import",
		mcp.WithDescription("Import documents from JSON."),
		mcp.WithString("content", mcp.Required(), mcp.Description("JSON content to import")),
		mcp.WithString("project", mcp.Description("Default project for items without one")),
	), withRecover(handleImport))

	s.AddTool(mcp.NewTool("project",
		mcp.WithDescription("Create a project or list all. With name: creates project (prefix auto-derived). Without: lists all projects."),
		mcp.WithString("name", mcp.Description("Project name (create mode)")),
	), withRecover(handleProject(db, bk)))

	s.AddTool(mcp.NewTool("item",
		mcp.WithDescription("Manage backlog items. With id + fields: update. With id only: get full detail. With project + priority + title: create. With filters only: list compact summaries. Set status to 'done' to close an item."),
		mcp.WithString("id", mcp.Description("Item ID (e.g., AC-1) for get/update")),
		mcp.WithString("project", mcp.Description("Project name (create or filter)")),
		mcp.WithString("priority", mcp.Description("Priority P0-P6 (create or update)")),
		mcp.WithString("title", mcp.Description("Item title (create or update)")),
		mcp.WithString("description", mcp.Description("Item description (create or update)")),
		mcp.WithString("status", mcp.Description("Item status: open or done (update or filter)")),
		mcp.WithString("priority_min", mcp.Description("Minimum priority filter (e.g., P0)")),
		mcp.WithString("priority_max", mcp.Description("Maximum priority filter (e.g., P2)")),
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
