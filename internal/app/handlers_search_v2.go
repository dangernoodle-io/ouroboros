package app

import (
	"context"
	"database/sql"

	"github.com/dangernoodle-io/mcpkit/mcpx"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/store"
)

// handleSearchV2 is the mcpkit-typed counterpart to handleSearch: dispatches
// by required domain (kb|backlog|roadmap), reading from the typed
// searchInput instead of an untyped arg map.
func handleSearchV2(db *sql.DB) mcpx.Handler[searchInput, any] {
	return func(_ context.Context, _ *mcpx.CallToolRequest, in searchInput) (*mcpx.CallToolResult, any, error) {
		// Explicit missing/empty check — see getInput.Domain's comment;
		// searchInput.Domain is likewise deliberately not schema-required.
		if in.Domain == "" {
			return mcpx.ErrorResult(errDomainRequired), nil, nil
		}
		switch in.Domain {
		case "kb":
			return searchDocumentsV2(db, in)
		case "backlog":
			return searchBacklogItemsV2(db, in)
		case "roadmap":
			return searchRoadmapV2(db, in)
		default:
			return mcpx.ErrorResult(errDomainRequired), nil, nil
		}
	}
}

// searchDocumentsV2 ports searchDocuments: single query or queries[] batch.
func searchDocumentsV2(db *sql.DB, in searchInput) (*mcpx.CallToolResult, any, error) {
	if len(in.Queries) > 0 {
		resultSets := make([][]store.DocumentSummary, 0, len(in.Queries))
		for _, q := range in.Queries {
			rs, err := store.SearchDocuments(db, q, in.Types, in.Projects, in.Categories, in.Limit)
			if err != nil {
				return mcpx.ErrorResult(err.Error()), nil, nil
			}
			if rs == nil {
				rs = []store.DocumentSummary{} // empty-not-nil invariant
			}
			resultSets = append(resultSets, rs)
		}
		return jsonResultV2(resultSets)
	}

	if in.Query == "" {
		return mcpx.ErrorResult("query or queries is required"), nil, nil
	}

	summaries, err := store.SearchDocuments(db, in.Query, in.Types, in.Projects, in.Categories, in.Limit)
	if err != nil {
		return mcpx.ErrorResult(err.Error()), nil, nil
	}

	return jsonResultV2(summaries)
}

// searchBacklogItemsV2 ports searchBacklogItems: FTS over
// title/description/notes, honoring the same filters as get's list mode.
func searchBacklogItemsV2(db *sql.DB, in searchInput) (*mcpx.CallToolResult, any, error) {
	if in.Query == "" {
		return mcpx.ErrorResult("query is required"), nil, nil
	}

	f, err := parseItemFilterV2(db, in.Projects, in.PriorityMin, in.PriorityMax, in.Status, in.Component, in.Epic, in.EpicsOnly, in.Since, in.Sort, in.Limit)
	if err != nil {
		return mcpx.ErrorResult(err.Error()), nil, nil
	}

	items, err := backlog.SearchItems(db, in.Query, f)
	if err != nil {
		return mcpx.ErrorResult(err.Error()), nil, nil
	}

	return mcpx.TextResult(itemLinesTextV2(items)), nil, nil
}

// searchRoadmapV2 ports searchRoadmap: FTS over documents type=roadmap.
func searchRoadmapV2(db *sql.DB, in searchInput) (*mcpx.CallToolResult, any, error) {
	if in.Query == "" {
		return mcpx.ErrorResult("query is required"), nil, nil
	}

	summaries, err := store.SearchDocuments(db, in.Query, []string{"roadmap"}, in.Projects, nil, in.Limit)
	if err != nil {
		return mcpx.ErrorResult(err.Error()), nil, nil
	}

	return jsonResultV2(summaries)
}
