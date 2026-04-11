package main

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerTools(s *server.MCPServer) {
	// kb_log_decision
	s.AddTool(mcp.NewTool("kb_log_decision",
		mcp.WithDescription("Record an architecture or design decision with rationale."),
		mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
		mcp.WithString("summary", mcp.Required(), mcp.Description("Decision summary")),
		mcp.WithString("rationale", mcp.Description("Decision rationale")),
		mcp.WithArray("tags", mcp.Description("Tags to categorize the decision")),
	), withRecover(handleLogDecision))

	// kb_get_decisions
	s.AddTool(mcp.NewTool("kb_get_decisions",
		mcp.WithDescription("Query decisions. Returns summaries only (no rationale) to conserve tokens. Use id to fetch a specific decision with full rationale."),
		mcp.WithNumber("id", mcp.Description("Fetch a specific decision by ID (returns full rationale)")),
		mcp.WithString("project", mcp.Description("Filter by project")),
		mcp.WithArray("tags", mcp.Description("Filter by tags (all must match)")),
		mcp.WithString("query", mcp.Description("Full-text search query")),
		mcp.WithNumber("limit", mcp.Description("Result limit (50-500, default 50)")),
	), withRecover(handleGetDecisions))

	// kb_delete_decision
	s.AddTool(mcp.NewTool("kb_delete_decision",
		mcp.WithDescription("Delete a decision by ID."),
		mcp.WithNumber("id", mcp.Required(), mcp.Description("Decision ID")),
	), withRecover(handleDeleteDecision))

	// kb_set_fact
	s.AddTool(mcp.NewTool("kb_set_fact",
		mcp.WithDescription("Set a project fact. Upserts by project+category+key. Use for hardware specs, API surfaces, dependency choices, env state."),
		mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
		mcp.WithString("category", mcp.Required(), mcp.Description("Fact category")),
		mcp.WithString("key", mcp.Required(), mcp.Description("Fact key")),
		mcp.WithString("value", mcp.Required(), mcp.Description("Fact value")),
	), withRecover(handleSetFact))

	// kb_get_facts
	s.AddTool(mcp.NewTool("kb_get_facts",
		mcp.WithDescription("Query facts. Filter by project, category, key, or use query for full-text search."),
		mcp.WithString("project", mcp.Description("Filter by project")),
		mcp.WithString("category", mcp.Description("Filter by category")),
		mcp.WithString("key", mcp.Description("Filter by key")),
		mcp.WithString("query", mcp.Description("Full-text search query")),
		mcp.WithNumber("limit", mcp.Description("Result limit (50-500, default 50)")),
	), withRecover(handleGetFacts))

	// kb_delete_fact
	s.AddTool(mcp.NewTool("kb_delete_fact",
		mcp.WithDescription("Delete a fact by project, category, and key."),
		mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
		mcp.WithString("category", mcp.Required(), mcp.Description("Fact category")),
		mcp.WithString("key", mcp.Required(), mcp.Description("Fact key")),
	), withRecover(handleDeleteFact))

	// kb_link
	s.AddTool(mcp.NewTool("kb_link",
		mcp.WithDescription("Create a relation between entities. Types: depends_on, implements, uses, replaces, calls, extends, produces, consumes."),
		mcp.WithString("source_project", mcp.Required(), mcp.Description("Source project")),
		mcp.WithString("source", mcp.Required(), mcp.Description("Source entity")),
		mcp.WithString("target_project", mcp.Required(), mcp.Description("Target project")),
		mcp.WithString("target", mcp.Required(), mcp.Description("Target entity")),
		mcp.WithString("relation_type", mcp.Required(), mcp.Description("Type of relation")),
		mcp.WithString("description", mcp.Description("Relation description")),
	), withRecover(handleLink))

	// kb_get_links
	s.AddTool(mcp.NewTool("kb_get_links",
		mcp.WithDescription("Query relations. Filter by project, entity (matches source or target), or relation type."),
		mcp.WithString("project", mcp.Description("Filter by project")),
		mcp.WithString("entity", mcp.Description("Filter by entity (source or target)")),
		mcp.WithString("relation_type", mcp.Description("Filter by relation type")),
		mcp.WithNumber("limit", mcp.Description("Result limit (50-500, default 50)")),
	), withRecover(handleGetLinks))

	// kb_delete_link
	s.AddTool(mcp.NewTool("kb_delete_link",
		mcp.WithDescription("Delete a relation by ID."),
		mcp.WithNumber("id", mcp.Required(), mcp.Description("Relation ID")),
	), withRecover(handleDeleteLink))

	// kb_search
	s.AddTool(mcp.NewTool("kb_search",
		mcp.WithDescription("Full-text search across all decisions and facts."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query")),
		mcp.WithString("project", mcp.Description("Filter by project")),
		mcp.WithNumber("limit", mcp.Description("Result limit (50-500, default 50)")),
	), withRecover(handleSearch))

	// kb_export
	s.AddTool(mcp.NewTool("kb_export",
		mcp.WithDescription("Export knowledge base to markdown. Filter by project or export all."),
		mcp.WithString("project", mcp.Description("Filter by project, or empty for all")),
	), withRecover(handleExport))

	// kb_import
	s.AddTool(mcp.NewTool("kb_import",
		mcp.WithDescription("Import data from JSON. Content is a JSON string with decisions, facts, and relations arrays."),
		mcp.WithString("project", mcp.Required(), mcp.Description("Default project for imported items")),
		mcp.WithString("content", mcp.Required(), mcp.Description("JSON content to import")),
	), withRecover(handleImport))

	// kb_set_note
	s.AddTool(mcp.NewTool("kb_set_note",
		mcp.WithDescription("Create or update a note. Upserts by project+category+title. Use for procedures, guides, conventions, and longer-form documentation."),
		mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
		mcp.WithString("category", mcp.Required(), mcp.Description("Note category (e.g. procedure, guide, convention)")),
		mcp.WithString("title", mcp.Required(), mcp.Description("Note title")),
		mcp.WithString("body", mcp.Required(), mcp.Description("Note body (markdown)")),
		mcp.WithArray("tags", mcp.Description("Tags to categorize the note")),
	), withRecover(handleSetNote))

	// kb_get_notes
	s.AddTool(mcp.NewTool("kb_get_notes",
		mcp.WithDescription("Query notes. Returns titles only (no body) to conserve tokens. Use id to fetch a specific note with full body."),
		mcp.WithNumber("id", mcp.Description("Fetch a specific note by ID (returns full body)")),
		mcp.WithString("project", mcp.Description("Filter by project")),
		mcp.WithString("category", mcp.Description("Filter by category")),
		mcp.WithString("query", mcp.Description("Full-text search query")),
		mcp.WithArray("tags", mcp.Description("Filter by tags (all must match)")),
		mcp.WithNumber("limit", mcp.Description("Result limit (default 50, max 500)")),
	), withRecover(handleGetNotes))

	// kb_delete_note
	s.AddTool(mcp.NewTool("kb_delete_note",
		mcp.WithDescription("Delete a note by ID."),
		mcp.WithNumber("id", mcp.Required(), mcp.Description("Note ID")),
	), withRecover(handleDeleteNote))
}
