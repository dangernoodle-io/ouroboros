package app

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dangernoodle.io/ouroboros/internal/edges"
	"dangernoodle.io/ouroboros/internal/kb"
	"dangernoodle.io/ouroboros/internal/store"
)

func handleKB(db *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Batch-only: entries array required
		entries := parseEntriesArray(req.GetArguments(), "entries")
		if len(entries) == 0 {
			return mcp.NewToolResultError("entries array is required (batch-only mode)"), nil //nolint:nilerr
		}

		// Split entries[] on id presence: id present -> update that doc in
		// place (partial, by key-presence); id absent -> current
		// upsert-by-natural-key (type+project+category+title) behavior,
		// unchanged. The id key must be entirely ABSENT to select create —
		// rowids start at 1, so id:0 (or any non-positive value) is a hard
		// error, never a silent "treat as absent".
		kbEntries := make([]kb.Entry, 0, len(entries))
		var kbUpdates []kb.EntryUpdate
		for _, e := range entries {
			id, present, err := parseKBEntryID(e)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil //nolint:nilerr
			}
			if present {
				kbUpdates = append(kbUpdates, parseKBEntryUpdate(id, e))
				continue
			}
			kbEntries = append(kbEntries, parseKBEntryCreate(e))
		}

		// One atomic call: every create and update in this batch commits
		// together or none do (single tx, single FTS rebuild) — a partial
		// failure (e.g. an update targeting a nonexistent id) must not
		// leave an earlier create in the same call persisted.
		createResults, updateResults, err := kb.WriteAndUpdateBatch(db, kbEntries, kbUpdates, "")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		results := make([]interface{}, 0, len(createResults)+len(updateResults))
		for _, r := range createResults {
			results = append(results, r)
		}
		for _, r := range updateResults {
			results = append(results, r)
		}

		return jsonResult(results)
	}
}

// parseKBEntryID resolves the optional id field of an entries[] item. The
// id key must be entirely ABSENT to select the create/upsert path — rowids
// start at 1, so id:0 (or any non-positive value) is invalid, not a silent
// "treat as absent" sentinel. A present id accepts a JSON number or a
// numeric string (so a string-typed id like "42" still routes to update
// instead of silently misrouting to create); anything else is a hard error.
func parseKBEntryID(e map[string]interface{}) (id int64, present bool, err error) {
	raw, ok := e["id"]
	if !ok {
		return 0, false, nil
	}

	switch v := raw.(type) {
	case float64:
		if v != float64(int64(v)) || v <= 0 {
			return 0, true, fmt.Errorf("invalid id: %v", raw)
		}
		return int64(v), true, nil
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return 0, true, fmt.Errorf("invalid id: %v", raw)
		}
		return n, true, nil
	default:
		return 0, true, fmt.Errorf("invalid id: %v", raw)
	}
}

// parseKBEntryCreate converts a raw entries[] map into a kb.Entry for the
// id-absent upsert-by-natural-key path.
func parseKBEntryCreate(e map[string]interface{}) kb.Entry {
	var entry kb.Entry

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

	if rawTags, ok := e["tags"].([]interface{}); ok {
		for _, t := range rawTags {
			if s, ok := t.(string); ok {
				entry.Tags = append(entry.Tags, s)
			}
		}
	}

	if rawMeta, ok := e["metadata"].(map[string]interface{}); ok {
		entry.Metadata = make(map[string]string)
		for k, v := range rawMeta {
			if s, ok := v.(string); ok {
				entry.Metadata[k] = s
			}
		}
	}

	return entry
}

// parseKBEntryUpdate converts a raw entries[] map into a kb.EntryUpdate for
// the id-addressed update path. Pointer fields track key presence (not just
// non-empty values), so a title-only update leaves content/notes/tags
// untouched instead of clobbering them with zero values.
func parseKBEntryUpdate(id int64, e map[string]interface{}) kb.EntryUpdate {
	u := kb.EntryUpdate{ID: id}

	if v, ok := e["type"].(string); ok {
		u.Type = &v
	}
	if v, ok := e["project"].(string); ok {
		u.Project = &v
	}
	if v, ok := e["category"].(string); ok {
		u.Category = &v
	}
	if v, ok := e["title"].(string); ok {
		u.Title = &v
	}
	if v, ok := e["content"].(string); ok {
		u.Content = &v
	}
	if v, ok := e["notes"].(string); ok {
		u.Notes = &v
	}
	if rawTags, ok := e["tags"].([]interface{}); ok {
		tags := make([]string, 0, len(rawTags))
		for _, t := range rawTags {
			if s, ok := t.(string); ok {
				tags = append(tags, s)
			}
		}
		u.Tags = &tags
	}
	if rawMeta, ok := e["metadata"].(map[string]interface{}); ok {
		meta := make(map[string]string, len(rawMeta))
		for k, v := range rawMeta {
			if s, ok := v.(string); ok {
				meta[k] = s
			}
		}
		u.Metadata = &meta
	}

	return u
}

// handleGet dispatches the read tool by required domain (kb|backlog).
func handleGet(db *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		domain, _ := req.GetArguments()["domain"].(string)
		switch domain {
		case "kb":
			return getDocuments(db, req)
		case "backlog":
			return getBacklogItems(db, req)
		case "roadmap":
			return getRoadmap(db, req)
		default:
			return mcp.NewToolResultError(`domain is required: must be "kb", "backlog", or "roadmap"`), nil //nolint:nilerr
		}
	}
}

// docWithEdges wraps a KB document with its edges sidecar, only populated
// on verbose=true reads (see getDocuments).
type docWithEdges struct {
	*store.Document
	Edges []edges.Edge `json:"edges,omitempty"`
}

// getDocuments handles domain=kb reads: ids[] fetch, or filters list.
func getDocuments(db *sql.DB, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// If ids provided, return full documents (omit misses)
	ids := parseInt64Slice(req.GetArguments(), "ids")

	// kb ids are integers (JSON numbers). If the caller supplied an ids[]
	// array where ANY element failed to parse as int64 (e.g. a string-typed
	// id like "1364", or a null), that's a type mismatch, not "no ids" —
	// silently dropping the bad element and returning the rest would return
	// silently-wrong data. Correctly-typed-but-nonexistent ids still parse
	// fine here (parsed length == raw length) and fall through to the
	// ids-fetch branch below (empty result), unaffected. An explicitly
	// empty ids[] array (len 0 == len 0) falls through to filter/list mode.
	if rawIDs, ok := req.GetArguments()["ids"].([]interface{}); ok && len(ids) != len(rawIDs) {
		return mcp.NewToolResultError("ids for domain=kb must be integers"), nil //nolint:nilerr
	}

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
				doc.SessionID = ""
				docs = append(docs, doc)
				continue
			}
			doc.SessionID = ""

			edgeList, err := edges.EdgesFor(db, edges.TypeKB, strconv.FormatInt(doc.ID, 10))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			docs = append(docs, docWithEdges{Document: doc, Edges: edgeList})
		}

		return jsonResult(docs)
	}

	// Filter/list mode
	types := parseStringSlice(req.GetArguments(), "types")
	projects := parseStringSlice(req.GetArguments(), "projects")
	categories := parseStringSlice(req.GetArguments(), "categories")
	query, _ := req.GetArguments()["query"].(string)

	tags := parseStringSlice(req.GetArguments(), "tags")

	limit := 0
	if v, ok := req.GetArguments()["limit"].(float64); ok {
		limit = int(v)
	}

	summaries, err := store.QueryDocuments(db, types, projects, categories, query, tags, limit)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return jsonResult(summaries)
}

// handleSearch dispatches the search tool by required domain (kb|backlog).
func handleSearch(db *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		domain, _ := req.GetArguments()["domain"].(string)
		switch domain {
		case "kb":
			return searchDocuments(db, req)
		case "backlog":
			return searchBacklogItems(db, req)
		case "roadmap":
			return searchRoadmap(db, req)
		default:
			return mcp.NewToolResultError(`domain is required: must be "kb", "backlog", or "roadmap"`), nil //nolint:nilerr
		}
	}
}

// searchDocuments handles domain=kb search: single query or queries[] batch.
func searchDocuments(db *sql.DB, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Batch mode: if queries[] is provided, loop over all queries with shared filters
	queries := parseStringSlice(req.GetArguments(), "queries")
	if len(queries) > 0 {
		types := parseStringSlice(req.GetArguments(), "types")
		projects := parseStringSlice(req.GetArguments(), "projects")
		categories := parseStringSlice(req.GetArguments(), "categories")

		limit := 0
		if v, ok := req.GetArguments()["limit"].(float64); ok {
			limit = int(v)
		}

		resultSets := make([][]store.DocumentSummary, 0, len(queries))
		for _, q := range queries {
			rs, err := store.SearchDocuments(db, q, types, projects, categories, limit)
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

	// Single-query mode
	query, _ := req.GetArguments()["query"].(string)
	if query == "" {
		return mcp.NewToolResultError("query or queries is required"), nil //nolint:nilerr
	}

	types := parseStringSlice(req.GetArguments(), "types")
	projects := parseStringSlice(req.GetArguments(), "projects")
	categories := parseStringSlice(req.GetArguments(), "categories")

	limit := 0
	if v, ok := req.GetArguments()["limit"].(float64); ok {
		limit = int(v)
	}

	summaries, err := store.SearchDocuments(db, query, types, projects, categories, limit)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return jsonResult(summaries)
}
