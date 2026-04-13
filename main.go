package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/kb"
	"dangernoodle.io/ouroboros/internal/store"
)

var Version = "dev"

var db *sql.DB

func withRecover(handler server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (result *mcp.CallToolResult, err error) {
		defer func() {
			if r := recover(); r != nil {
				result = mcp.NewToolResultError(fmt.Sprintf("internal error: %v", r))
				err = nil
			}
		}()
		return handler(ctx, req)
	}
}

func jsonResult(v interface{}) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" {
			fmt.Println(Version)
			os.Exit(0)
		}
	}

	// CLI query mode: ouroboros query --project <name> [--type <type>] [--limit N]
	if len(os.Args) > 1 && os.Args[1] == "query" {
		runQuery(os.Args[2:])
		return
	}

	// CLI items mode: ouroboros items --project <name> [--status <open|done>] [--limit N]
	if len(os.Args) > 1 && os.Args[1] == "items" {
		runItems(os.Args[2:])
		return
	}

	var err error
	db, err = store.InitDB()
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}

	s := server.NewMCPServer("ouroboros", Version,
		server.WithToolCapabilities(true),
		server.WithInstructions(`Project knowledge base and backlog management — persist decisions across conversations and track work items.

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

Config: Use config tool for key-value settings (no args = list all, key = get, key + value = set).`),
	)

	registerTools(s)

	signal.Ignore(syscall.SIGPIPE)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		db.Close()
		os.Exit(0)
	}()

	if err := server.ServeStdio(s); err != nil {
		log.Fatal(err)
	}
}

// Handler functions

func handlePut(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	docType, err := req.RequireString("type")
	if err != nil {
		return mcp.NewToolResultError("type is required"), nil //nolint:nilerr
	}

	project, err := req.RequireString("project")
	if err != nil {
		return mcp.NewToolResultError("project is required"), nil //nolint:nilerr
	}

	title, err := req.RequireString("title")
	if err != nil {
		return mcp.NewToolResultError("title is required"), nil //nolint:nilerr
	}

	content, _ := req.GetArguments()["content"].(string)
	notes, _ := req.GetArguments()["notes"].(string)
	category, _ := req.GetArguments()["category"].(string)

	// Enforce 500-char hard cap on content
	if len(content) > 500 {
		fmt.Fprintf(os.Stderr, "ouroboros put cap reject: project=%q title=%q len(content)=%d len(notes)=%d\n", project, title, len(content), len(notes))
		return mcp.NewToolResultError(fmt.Sprintf("content exceeds 500 char hard cap (got %d). Move narrative into notes field.", len(content))), nil //nolint:nilerr
	}

	// Parse metadata from JSON string
	var metadata map[string]string
	if metadataStr, ok := req.GetArguments()["metadata"].(string); ok && metadataStr != "" {
		if err := json.Unmarshal([]byte(metadataStr), &metadata); err != nil {
			return mcp.NewToolResultError("invalid metadata JSON"), nil //nolint:nilerr
		}
	}

	// Parse tags from array
	var tags []string
	if rawTags, ok := req.GetArguments()["tags"].([]interface{}); ok {
		for _, t := range rawTags {
			if s, ok := t.(string); ok {
				tags = append(tags, s)
			}
		}
	}

	doc := store.Document{
		Type:     docType,
		Project:  project,
		Category: category,
		Title:    title,
		Content:  content,
		Notes:    notes,
		Metadata: metadata,
		Tags:     tags,
	}

	result, err := store.UpsertDocument(db, doc)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return jsonResult(map[string]interface{}{"id": result.ID, "action": result.Action})
}

func handleGet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// If id provided, return full document
	if idFloat, ok := req.GetArguments()["id"].(float64); ok {
		doc, err := store.GetDocument(db, int64(idFloat))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if doc == nil {
			return mcp.NewToolResultError("document not found"), nil
		}

		// Check verbose flag
		verbose, _ := req.GetArguments()["verbose"].(bool)
		if !verbose {
			doc.Notes = ""
		}

		return jsonResult(doc)
	}

	// Otherwise return summaries by filters
	docType, _ := req.GetArguments()["type"].(string)
	project, _ := req.GetArguments()["project"].(string)
	category, _ := req.GetArguments()["category"].(string)
	query, _ := req.GetArguments()["query"].(string)

	var tags []string
	if rawTags, ok := req.GetArguments()["tags"].([]interface{}); ok {
		for _, t := range rawTags {
			if s, ok := t.(string); ok {
				tags = append(tags, s)
			}
		}
	}

	limit := 0
	if v, ok := req.GetArguments()["limit"].(float64); ok {
		limit = int(v)
	}

	summaries, err := store.QueryDocuments(db, docType, project, category, query, tags, limit)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return jsonResult(summaries)
}

func handleDelete(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	idFloat, ok := req.GetArguments()["id"].(float64)
	if !ok {
		return mcp.NewToolResultError("id is required"), nil //nolint:nilerr
	}

	err := store.DeleteDocument(db, int64(idFloat))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return jsonResult(map[string]bool{"ok": true})
}

func handleSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := req.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query is required"), nil //nolint:nilerr
	}

	docType, _ := req.GetArguments()["type"].(string)
	project, _ := req.GetArguments()["project"].(string)

	limit := 0
	if v, ok := req.GetArguments()["limit"].(float64); ok {
		limit = int(v)
	}

	summaries, err := store.SearchDocuments(db, query, docType, project, limit)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return jsonResult(summaries)
}

func handleExport(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project, _ := req.GetArguments()["project"].(string)
	docType, _ := req.GetArguments()["type"].(string)

	markdown, err := kb.ExportMarkdown(db, project, docType)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(markdown), nil
}

func handleImport(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	content, err := req.RequireString("content")
	if err != nil {
		return mcp.NewToolResultError("content is required"), nil //nolint:nilerr
	}

	project, _ := req.GetArguments()["project"].(string)

	err = kb.Import(db, project, content)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return jsonResult(map[string]bool{"ok": true})
}

type queryArgs struct {
	project string
	docType string
	search  string
	limit   int
}

func parseQueryArgs(args []string) queryArgs {
	qa := queryArgs{limit: 10}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project":
			if i+1 < len(args) {
				qa.project = args[i+1]
				i++
			}
		case "--type":
			if i+1 < len(args) {
				qa.docType = args[i+1]
				i++
			}
		case "--search":
			if i+1 < len(args) {
				qa.search = args[i+1]
				i++
			}
		case "--limit":
			if i+1 < len(args) {
				if n, err := fmt.Sscanf(args[i+1], "%d", &qa.limit); err != nil || n != 1 {
					qa.limit = 10
				}
				i++
			}
		}
	}
	return qa
}

// runQuery handles CLI query mode: ouroboros query --project <name> [--type <type>] [--search <query>] [--limit N]
// Outputs JSON array of document summaries to stdout.
func runQuery(args []string) {
	qa := parseQueryArgs(args)

	var err error
	db, err = store.InitDB()
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	var summaries []store.DocumentSummary

	if qa.search != "" {
		summaries, err = store.KeywordSearch(db, qa.search, qa.project, qa.limit)
		if err != nil {
			log.Fatalf("search failed: %v", err)
		}
	} else {
		summaries, err = store.QueryDocuments(db, qa.docType, qa.project, "", "", nil, qa.limit)
		if err != nil {
			log.Fatalf("query failed: %v", err)
		}
	}

	data, err := json.Marshal(summaries)
	if err != nil {
		log.Fatalf("marshal failed: %v", err)
	}

	fmt.Println(string(data))
}

type itemsArgs struct {
	project string
	status  string
	limit   int
}

func parseItemsArgs(args []string) itemsArgs {
	ia := itemsArgs{status: "open", limit: 20}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project":
			if i+1 < len(args) {
				ia.project = args[i+1]
				i++
			}
		case "--status":
			if i+1 < len(args) {
				ia.status = args[i+1]
				i++
			}
		case "--limit":
			if i+1 < len(args) {
				if n, err := fmt.Sscanf(args[i+1], "%d", &ia.limit); err != nil || n != 1 {
					ia.limit = 20
				}
				i++
			}
		}
	}
	return ia
}

// runItems handles CLI items mode: ouroboros items --project <name> [--status <open|done>] [--limit N]
// Outputs JSON array of backlog items to stdout.
func runItems(args []string) {
	ia := parseItemsArgs(args)

	var err error
	db, err = store.InitDB()
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Look up project by name
	project, err := backlog.GetProjectByName(db, ia.project)
	if err != nil {
		// Project not found — output empty array
		fmt.Println("[]")
		return
	}

	// Build filter with project ID and optionally status
	filter := backlog.ItemFilter{ProjectID: &project.ID}
	if ia.status != "" {
		filter.Status = &ia.status
	}

	// List items
	items, err := backlog.ListItems(db, filter)
	if err != nil {
		log.Fatalf("list items failed: %v", err)
	}

	data, err := json.Marshal(items)
	if err != nil {
		log.Fatalf("marshal failed: %v", err)
	}

	fmt.Println(string(data))
}
