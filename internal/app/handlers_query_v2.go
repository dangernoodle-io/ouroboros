package app

import (
	"context"

	"github.com/dangernoodle-io/shesha/mcpx"

	"dangernoodle.io/ouroboros/internal/query"
)

// handleQueryV2 is a thin wrapper over the surface-neutral query.Get/
// query.Search core (OU-322): build a query.Request from the typed
// queryInputV2, dispatch to Get or Search, then render the structured
// query.Result back into an mcpx.CallToolResult. This collapses the former
// get and search tools into one (OU-323). st.db is read at call time (not at
// construction time) — see serverState's doc comment.
//
// Dispatch precedence: ids[] wins over query/queries — an id-fetch is exact
// match and "skips search/filter mode" (see queryInputV2.IDs' schema doc and
// descQueryToolV2), so ids[] set alongside a full-text query/queries[]
// dispatches to query.Get, not query.Search, ignoring the query/queries.
// Absent ids[], query/queries[] present dispatches to query.Search; absent
// both, it's a plain filter/list via query.Get.
func handleQueryV2(st *serverState) mcpx.Handler[queryInputV2, any] {
	return func(_ context.Context, _ *mcpx.CallToolRequest, in queryInputV2) (*mcpx.CallToolResult, any, error) {
		req := buildQueryRequest(in)
		isSearch := len(in.IDs) == 0 && (in.Query != "" || len(in.Queries) > 0)

		var (
			result query.Result
			err    error
		)
		if isSearch {
			result, err = query.Search(st.db, req)
		} else {
			result, err = query.Get(st.db, req)
		}
		if err != nil {
			return mcpx.ErrorResult(err.Error()), nil, nil
		}
		return renderQueryResult(in, result, isSearch)
	}
}

// buildQueryRequest maps queryInputV2's fields onto the core's
// surface-neutral query.Request 1:1.
func buildQueryRequest(in queryInputV2) query.Request {
	return query.Request{
		Domain:      in.Domain,
		IDs:         in.IDs,
		Verbose:     in.Verbose,
		Query:       in.Query,
		Queries:     in.Queries,
		Types:       in.Types,
		Projects:    in.Projects,
		Categories:  in.Categories,
		Tags:        in.Tags,
		Limit:       in.Limit,
		PriorityMin: in.PriorityMin,
		PriorityMax: in.PriorityMax,
		Status:      in.Status,
		Component:   in.Component,
		Epic:        in.Epic,
		EpicsOnly:   in.EpicsOnly,
		Since:       in.Since,
		Sort:        in.Sort,
		By:          in.By,
		Format:      in.Format,
	}
}

// renderQueryResult mirrors the former renderGetResult/renderSearchResult's
// per-domain response shaping, now merged and branching on isSearch (which
// mode dispatched) in addition to domain/ids/queries/format.
func renderQueryResult(in queryInputV2, result query.Result, isSearch bool) (*mcpx.CallToolResult, any, error) {
	switch in.Domain {
	case "kb":
		if isSearch {
			if len(in.Queries) > 0 {
				return jsonResultV2(result.DocSummarySets)
			}
			return jsonResultV2(result.DocSummaries)
		}
		if len(in.IDs) > 0 {
			return jsonResultV2(result.Docs)
		}
		return jsonResultV2(result.DocSummaries)
	case "backlog":
		if !isSearch && len(in.IDs) > 0 {
			return jsonResultV2(result.ItemsJSON)
		}
		text := itemLinesTextV2(result.Items)
		if isSearch && result.Relaxed {
			// OU-346: the AND query matched nothing and this is the OR
			// fallback's widened result — say so, so a caller can tell an
			// exact match from a relaxed one instead of assuming a literal
			// AND hit.
			text = "(relaxed: no exact match, widened query terms to OR)\n" + text
		}
		return mcpx.TextResult(text), nil, nil
	case "roadmap":
		if isSearch {
			return jsonResultV2(result.DocSummaries)
		}
		switch in.Format {
		case "md", "html":
			return mcpx.TextResult(result.Text), nil, nil
		}
		return jsonResultV2(result.Roadmap)
	default:
		// Unreachable: query.Get/query.Search already error
		// (query.ErrDomainRequired) for any domain other than
		// kb|backlog|roadmap, including "".
		return mcpx.ErrorResult(query.ErrDomainRequired), nil, nil
	}
}
