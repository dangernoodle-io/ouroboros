package app

import (
	"context"

	"github.com/dangernoodle-io/mcpkit/mcpx"

	"dangernoodle.io/ouroboros/internal/query"
)

// handleSearchV2 is a thin wrapper over the surface-neutral query.Search
// core (OU-322): build a query.Request from the typed searchInput, call the
// core, then render the structured query.Result back into an
// mcpx.CallToolResult. st.db is read at call time (not at construction
// time) — see serverState's doc comment.
func handleSearchV2(st *serverState) mcpx.Handler[searchInput, any] {
	return func(_ context.Context, _ *mcpx.CallToolRequest, in searchInput) (*mcpx.CallToolResult, any, error) {
		result, err := query.Search(st.db, buildSearchRequest(in))
		if err != nil {
			return mcpx.ErrorResult(err.Error()), nil, nil
		}
		return renderSearchResult(in, result)
	}
}

// buildSearchRequest maps searchInput's fields onto the core's
// surface-neutral query.Request 1:1.
func buildSearchRequest(in searchInput) query.Request {
	return query.Request{
		Domain:      in.Domain,
		Query:       in.Query,
		Queries:     in.Queries,
		Types:       in.Types,
		Projects:    in.Projects,
		Categories:  in.Categories,
		Limit:       in.Limit,
		PriorityMin: in.PriorityMin,
		PriorityMax: in.PriorityMax,
		Status:      in.Status,
		Component:   in.Component,
		Epic:        in.Epic,
		EpicsOnly:   in.EpicsOnly,
		Since:       in.Since,
		Sort:        in.Sort,
	}
}

// renderSearchResult mirrors the old inline handler's per-domain response
// shaping (formerly searchDocumentsV2/searchBacklogItemsV2/searchRoadmapV2's
// return statements), now operating on the core's already-computed
// query.Result instead of deriving it inline.
func renderSearchResult(in searchInput, result query.Result) (*mcpx.CallToolResult, any, error) {
	switch in.Domain {
	case "kb":
		if len(in.Queries) > 0 {
			return jsonResultV2(result.DocSummarySets)
		}
		return jsonResultV2(result.DocSummaries)
	case "backlog":
		return mcpx.TextResult(itemLinesTextV2(result.Items)), nil, nil
	case "roadmap":
		return jsonResultV2(result.DocSummaries)
	default:
		// Unreachable: query.Search already errors (query.ErrDomainRequired)
		// for any domain other than kb|backlog|roadmap, including "".
		return mcpx.ErrorResult(query.ErrDomainRequired), nil, nil
	}
}
