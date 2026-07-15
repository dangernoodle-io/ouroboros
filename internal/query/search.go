package query

import (
	"database/sql"
	"errors"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/store"
)

// Search dispatches a full-text search request by req.Domain
// (kb|backlog|roadmap), mirroring the old handleSearchV2 switch — now living
// in the surface-neutral core instead of the MCP handler.
func Search(db *sql.DB, req Request) (Result, error) {
	switch req.Domain {
	case "kb":
		return searchDocuments(db, req)
	case "backlog":
		return searchBacklogItems(db, req)
	case "roadmap":
		return searchRoadmap(db, req)
	default:
		return Result{}, errors.New(ErrDomainRequired)
	}
}

// searchDocuments ports searchDocumentsV2: single query or Queries[] batch.
func searchDocuments(db *sql.DB, req Request) (Result, error) {
	if len(req.Queries) > 0 {
		resultSets := make([][]store.DocumentSummary, 0, len(req.Queries))
		for _, q := range req.Queries {
			rs, err := store.SearchDocuments(db, q, req.Types, req.Projects, req.Categories, req.Limit)
			if err != nil {
				return Result{}, err
			}
			if rs == nil {
				rs = []store.DocumentSummary{} // empty-not-nil invariant
			}
			resultSets = append(resultSets, rs)
		}
		return Result{DocSummarySets: resultSets}, nil
	}

	if req.Query == "" {
		return Result{}, errors.New("query or queries is required")
	}

	summaries, err := store.SearchDocuments(db, req.Query, req.Types, req.Projects, req.Categories, req.Limit)
	if err != nil {
		return Result{}, err
	}

	return Result{DocSummaries: summaries}, nil
}

// searchBacklogItems ports searchBacklogItemsV2: FTS over
// title/description/notes, honoring the same filters as Get's list mode.
func searchBacklogItems(db *sql.DB, req Request) (Result, error) {
	if req.Query == "" {
		return Result{}, errors.New("query is required")
	}

	f, err := buildItemFilter(db, req)
	if err != nil {
		return Result{}, err
	}

	items, err := backlog.SearchItems(db, req.Query, f)
	if err != nil {
		return Result{}, err
	}

	return Result{Items: items}, nil
}

// searchRoadmap ports searchRoadmapV2: FTS over documents type=roadmap.
func searchRoadmap(db *sql.DB, req Request) (Result, error) {
	if req.Query == "" {
		return Result{}, errors.New("query is required")
	}

	summaries, err := store.SearchDocuments(db, req.Query, []string{"roadmap"}, req.Projects, nil, req.Limit)
	if err != nil {
		return Result{}, err
	}

	return Result{DocSummaries: summaries}, nil
}
