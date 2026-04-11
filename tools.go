package main

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

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
}
