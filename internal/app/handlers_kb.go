package app

import (
	"context"
	"database/sql"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dangernoodle.io/ouroboros/internal/kb"
	"dangernoodle.io/ouroboros/internal/store"
)

func handlePut(db *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Batch-only: entries array required
		entries := parseEntriesArray(req.GetArguments(), "entries")
		if len(entries) == 0 {
			return mcp.NewToolResultError("entries array is required (batch-only mode)"), nil //nolint:nilerr
		}

		// Convert to kb.Entry slice
		kbEntries := make([]kb.Entry, 0, len(entries))
		for _, e := range entries {
			var entry kb.Entry

			// Extract string fields
			if v, ok := e["type"].(string); ok {
				entry.Type = v
			}
			if v, ok := e["project"].(string); ok {
				entry.Project = v
			}
			if v, ok := e["title"].(string); ok {
				entry.Title = v
			}
			if v, ok := e["content"].(string); ok {
				entry.Content = v
			}
			if v, ok := e["category"].(string); ok {
				entry.Category = v
			}
			if v, ok := e["notes"].(string); ok {
				entry.Notes = v
			}

			// Extract tags array
			if rawTags, ok := e["tags"].([]interface{}); ok {
				for _, t := range rawTags {
					if s, ok := t.(string); ok {
						entry.Tags = append(entry.Tags, s)
					}
				}
			}

			// Extract metadata
			if rawMeta, ok := e["metadata"].(map[string]interface{}); ok {
				entry.Metadata = make(map[string]string)
				for k, v := range rawMeta {
					if s, ok := v.(string); ok {
						entry.Metadata[k] = s
					}
				}
			}

			kbEntries = append(kbEntries, entry)
		}

		results, err := kb.WriteBatch(db, kbEntries, "")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(results)
	}
}

func handleGet(db *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// If ids provided, return full documents (omit misses)
		ids := parseInt64Slice(req.GetArguments(), "ids")
		if len(ids) > 0 {
			verbose, _ := req.GetArguments()["verbose"].(bool)
			docs := make([]interface{}, 0, len(ids))

			for _, id := range ids {
				doc, err := store.GetDocument(db, id)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				if doc == nil {
					// Omit misses
					continue
				}

				if !verbose {
					doc.Notes = ""
				}

				docs = append(docs, doc)
			}

			return jsonResult(docs)
		}

		// Filter/list mode
		docType, _ := req.GetArguments()["type"].(string)
		projects := parseStringSlice(req.GetArguments(), "projects")
		category, _ := req.GetArguments()["category"].(string)
		query, _ := req.GetArguments()["query"].(string)

		tags := parseStringSlice(req.GetArguments(), "tags")

		limit := 0
		if v, ok := req.GetArguments()["limit"].(float64); ok {
			limit = int(v)
		}

		summaries, err := store.QueryDocuments(db, docType, projects, category, query, tags, limit)
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
		// Batch mode: if queries[] is provided, loop over all queries with shared filters
		queries := parseStringSlice(req.GetArguments(), "queries")
		if len(queries) > 0 {
			docType, _ := req.GetArguments()["type"].(string)
			projects := parseStringSlice(req.GetArguments(), "projects")

			limit := 0
			if v, ok := req.GetArguments()["limit"].(float64); ok {
				limit = int(v)
			}

			resultSets := make([][]store.DocumentSummary, 0, len(queries))
			for _, q := range queries {
				rs, err := store.SearchDocuments(db, q, docType, projects, limit)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				if rs == nil {
					rs = []store.DocumentSummary{} // empty-not-nil invariant
				}
				resultSets = append(resultSets, rs)
			}
			return jsonResult(resultSets)
		}

		// Single-query mode: unchanged for backwards compat
		query, _ := req.GetArguments()["query"].(string)
		if query == "" {
			return mcp.NewToolResultError("query or queries is required"), nil //nolint:nilerr
		}

		docType, _ := req.GetArguments()["type"].(string)
		projects := parseStringSlice(req.GetArguments(), "projects")

		limit := 0
		if v, ok := req.GetArguments()["limit"].(float64); ok {
			limit = int(v)
		}

		summaries, err := store.SearchDocuments(db, query, docType, projects, limit)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(summaries)
	}
}

func handleExport(db *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projects := parseStringSlice(req.GetArguments(), "projects")
		docType, _ := req.GetArguments()["type"].(string)

		markdown, err := kb.ExportMarkdown(db, projects, docType)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(markdown), nil
	}
}

func handleImport(db *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultError("import is CLI-only; use: ouroboros import <file|->"), nil //nolint:nilerr
	}
}
