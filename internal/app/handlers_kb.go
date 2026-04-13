package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dangernoodle.io/ouroboros/internal/kb"
	"dangernoodle.io/ouroboros/internal/store"
)

func handlePut(db *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
}

func handleGet(db *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
}

func handleDelete(db *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
}

func handleSearch(db *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
}

func handleExport(db *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		project, _ := req.GetArguments()["project"].(string)
		docType, _ := req.GetArguments()["type"].(string)

		markdown, err := kb.ExportMarkdown(db, project, docType)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(markdown), nil
	}
}

func handleImport(db *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
}
