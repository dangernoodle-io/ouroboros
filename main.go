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

	var err error
	db, err = store.InitDB()
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
	category, _ := req.GetArguments()["category"].(string)

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
		Metadata: metadata,
		Tags:     tags,
	}

	id, err := store.UpsertDocument(db, doc)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return jsonResult(map[string]interface{}{"id": id, "ok": true})
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
