package query

import (
	"database/sql"
	"errors"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/roadmap"
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
// req.Tags (OU-330) combines with the full-text query — previously dropped
// in search mode, honored only in list/filter mode via query.Get.
func searchDocuments(db *sql.DB, req Request) (Result, error) {
	if len(req.Queries) > 0 {
		resultSets := make([][]store.DocumentSummary, 0, len(req.Queries))
		for _, q := range req.Queries {
			// SearchDocumentsRelaxed (OU-346): each query in the batch
			// independently gets the AND->OR fallback, surfaced per-row via
			// DocumentSummary.Relaxed.
			rs, err := store.SearchDocumentsRelaxed(db, q, req.Types, req.Projects, req.Categories, req.Limit, req.Tags...)
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

	summaries, err := store.SearchDocumentsRelaxed(db, req.Query, req.Types, req.Projects, req.Categories, req.Limit, req.Tags...)
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

	// SearchItemsRelaxed (OU-346): a zero-row implicit-AND query retries once
	// OR-joined instead of silently returning no matches.
	items, relaxed, err := backlog.SearchItemsRelaxed(db, req.Query, f)
	if err != nil {
		return Result{}, err
	}

	return Result{Items: items, Relaxed: relaxed}, nil
}

// searchRoadmap ports searchRoadmapV2: FTS over documents type=roadmap. Each
// hit is a whole per-project roadmap doc (roadmap is a singleton doc, not
// one row per item), so req.Component/req.Epic (OU-330) can't filter SQL
// rows directly — instead, for each FTS hit, load and roadmap.Filter that
// project's roadmap and keep the hit only if the filtered roadmap still has
// at least one item, mirroring the component/epic filter query.Get applies
// via getRoadmap. No filter set: behavior is unchanged (no extra Load call).
func searchRoadmap(db *sql.DB, req Request) (Result, error) {
	if req.Query == "" {
		return Result{}, errors.New("query is required")
	}

	// SearchDocumentsRelaxed (OU-346): a zero-row implicit-AND roadmap search
	// retries once OR-joined instead of silently returning no matches; the
	// relaxed signal rides the per-row DocumentSummary.Relaxed field, same as
	// kb search, straight to the wire (jsonResultV2(result.DocSummaries)).
	summaries, err := store.SearchDocumentsRelaxed(db, req.Query, []string{"roadmap"}, req.Projects, nil, req.Limit)
	if err != nil {
		return Result{}, err
	}

	if req.Component == "" && req.Epic == "" {
		return Result{DocSummaries: summaries}, nil
	}

	filtered := make([]store.DocumentSummary, 0, len(summaries))
	for _, s := range summaries {
		rm, err := roadmap.Load(db, s.Project)
		if err != nil {
			return Result{}, err
		}
		if roadmapHasItems(roadmap.Filter(rm, req.Component, req.Epic)) {
			filtered = append(filtered, s)
		}
	}

	return Result{DocSummaries: filtered}, nil
}

// roadmapHasItems reports whether rm has at least one item in any section.
func roadmapHasItems(rm *roadmap.Roadmap) bool {
	return len(rm.Sections.Now) > 0 || len(rm.Sections.Next) > 0 ||
		len(rm.Sections.Deferred) > 0 || len(rm.Sections.Parked) > 0 ||
		len(rm.Sections.Dropped) > 0 || len(rm.Sections.Done) > 0
}
