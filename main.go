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

	var err error
	db, err = initDB()
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}

	s := server.NewMCPServer("ouroboros", Version,
		server.WithToolCapabilities(true),
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

func handleLogDecision(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project, err := req.RequireString("project")
	if err != nil {
		return mcp.NewToolResultError("project is required"), nil //nolint:nilerr
	}

	summary, err := req.RequireString("summary")
	if err != nil {
		return mcp.NewToolResultError("summary is required"), nil //nolint:nilerr
	}

	rationale, _ := req.GetArguments()["rationale"].(string)

	var tags []string
	if rawTags, ok := req.GetArguments()["tags"].([]interface{}); ok {
		for _, t := range rawTags {
			if s, ok := t.(string); ok {
				tags = append(tags, s)
			}
		}
	}

	id, err := insertDecision(db, project, summary, rationale, tags)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return jsonResult(map[string]interface{}{"id": id, "ok": true})
}

func handleGetDecisions(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// If id is provided, return full decision with rationale.
	if idFloat, ok := req.GetArguments()["id"].(float64); ok {
		decision, err := getDecision(db, int64(idFloat))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if decision == nil {
			return mcp.NewToolResultError("decision not found"), nil
		}
		return jsonResult(decision)
	}

	// Otherwise return summaries (no rationale).
	project, _ := req.GetArguments()["project"].(string)
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

	decisions, err := queryDecisions(db, project, tags, query, limit)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return jsonResult(decisions)
}

func handleDeleteDecision(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	idFloat, ok := req.GetArguments()["id"].(float64)
	if !ok {
		return mcp.NewToolResultError("id is required"), nil //nolint:nilerr
	}
	id := int64(idFloat)

	err := deleteDecision(db, id)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return jsonResult(map[string]bool{"ok": true})
}

func handleSetFact(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project, err := req.RequireString("project")
	if err != nil {
		return mcp.NewToolResultError("project is required"), nil //nolint:nilerr
	}

	category, err := req.RequireString("category")
	if err != nil {
		return mcp.NewToolResultError("category is required"), nil //nolint:nilerr
	}

	key, err := req.RequireString("key")
	if err != nil {
		return mcp.NewToolResultError("key is required"), nil //nolint:nilerr
	}

	value, err := req.RequireString("value")
	if err != nil {
		return mcp.NewToolResultError("value is required"), nil //nolint:nilerr
	}

	id, err := upsertFact(db, project, category, key, value)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return jsonResult(map[string]interface{}{"id": id, "ok": true})
}

func handleGetFacts(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project, _ := req.GetArguments()["project"].(string)
	category, _ := req.GetArguments()["category"].(string)
	key, _ := req.GetArguments()["key"].(string)
	query, _ := req.GetArguments()["query"].(string)

	limit := 0
	if v, ok := req.GetArguments()["limit"].(float64); ok {
		limit = int(v)
	}

	facts, err := queryFacts(db, project, category, key, query, limit)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return jsonResult(facts)
}

func handleDeleteFact(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project, err := req.RequireString("project")
	if err != nil {
		return mcp.NewToolResultError("project is required"), nil //nolint:nilerr
	}

	category, err := req.RequireString("category")
	if err != nil {
		return mcp.NewToolResultError("category is required"), nil //nolint:nilerr
	}

	key, err := req.RequireString("key")
	if err != nil {
		return mcp.NewToolResultError("key is required"), nil //nolint:nilerr
	}

	err = deleteFact(db, project, category, key)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return jsonResult(map[string]bool{"ok": true})
}

func handleLink(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sourceProject, err := req.RequireString("source_project")
	if err != nil {
		return mcp.NewToolResultError("source_project is required"), nil //nolint:nilerr
	}

	source, err := req.RequireString("source")
	if err != nil {
		return mcp.NewToolResultError("source is required"), nil //nolint:nilerr
	}

	targetProject, err := req.RequireString("target_project")
	if err != nil {
		return mcp.NewToolResultError("target_project is required"), nil //nolint:nilerr
	}

	target, err := req.RequireString("target")
	if err != nil {
		return mcp.NewToolResultError("target is required"), nil //nolint:nilerr
	}

	relationType, err := req.RequireString("relation_type")
	if err != nil {
		return mcp.NewToolResultError("relation_type is required"), nil //nolint:nilerr
	}

	description, _ := req.GetArguments()["description"].(string)

	id, err := insertRelation(db, sourceProject, source, targetProject, target, relationType, description)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return jsonResult(map[string]interface{}{"id": id, "ok": true})
}

func handleGetLinks(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project, _ := req.GetArguments()["project"].(string)
	entity, _ := req.GetArguments()["entity"].(string)
	relationType, _ := req.GetArguments()["relation_type"].(string)

	limit := 0
	if v, ok := req.GetArguments()["limit"].(float64); ok {
		limit = int(v)
	}

	relations, err := queryRelations(db, project, entity, relationType, limit)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return jsonResult(relations)
}

func handleDeleteLink(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	idFloat, ok := req.GetArguments()["id"].(float64)
	if !ok {
		return mcp.NewToolResultError("id is required"), nil //nolint:nilerr
	}
	id := int64(idFloat)

	err := deleteRelation(db, id)
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

	project, _ := req.GetArguments()["project"].(string)

	limit := 0
	if v, ok := req.GetArguments()["limit"].(float64); ok {
		limit = int(v)
	}

	result, err := searchAll(db, query, project, limit)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return jsonResult(result)
}

func handleExport(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project, _ := req.GetArguments()["project"].(string)

	markdown, err := exportMarkdown(db, project)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(markdown), nil
}

func handleImport(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project, err := req.RequireString("project")
	if err != nil {
		return mcp.NewToolResultError("project is required"), nil //nolint:nilerr
	}

	content, err := req.RequireString("content")
	if err != nil {
		return mcp.NewToolResultError("content is required"), nil //nolint:nilerr
	}

	err = importData(db, project, content)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return jsonResult(map[string]bool{"ok": true})
}

func handleSetNote(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project, err := req.RequireString("project")
	if err != nil {
		return mcp.NewToolResultError("project is required"), nil //nolint:nilerr
	}
	category, err := req.RequireString("category")
	if err != nil {
		return mcp.NewToolResultError("category is required"), nil //nolint:nilerr
	}
	title, err := req.RequireString("title")
	if err != nil {
		return mcp.NewToolResultError("title is required"), nil //nolint:nilerr
	}
	body, err := req.RequireString("body")
	if err != nil {
		return mcp.NewToolResultError("body is required"), nil //nolint:nilerr
	}

	var tags []string
	if rawTags, ok := req.GetArguments()["tags"].([]interface{}); ok {
		for _, t := range rawTags {
			if s, ok := t.(string); ok {
				tags = append(tags, s)
			}
		}
	}

	id, err := upsertNote(db, project, category, title, body, tags)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return jsonResult(map[string]interface{}{"id": id, "ok": true})
}

func handleGetNotes(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// If id is provided, return full note with body
	if idFloat, ok := req.GetArguments()["id"].(float64); ok {
		note, err := getNote(db, int64(idFloat))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if note == nil {
			return mcp.NewToolResultError("note not found"), nil
		}
		return jsonResult(note)
	}

	// Otherwise return summaries (no body)
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

	notes, err := queryNotes(db, project, category, query, tags, limit)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return jsonResult(notes)
}

func handleDeleteNote(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	idFloat, ok := req.GetArguments()["id"].(float64)
	if !ok {
		return mcp.NewToolResultError("id is required"), nil //nolint:nilerr
	}
	if err := deleteNote(db, int64(idFloat)); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return jsonResult(map[string]bool{"ok": true})
}
