package app

import (
	"context"

	"github.com/dangernoodle-io/mcpkit/mcpx"

	"dangernoodle.io/ouroboros/internal/query"
)

// handleGetV2 is a thin wrapper over the surface-neutral query.Get core
// (OU-322): build a query.Request from the typed getInput, call the core,
// then render the structured query.Result back into an mcpx.CallToolResult.
// req is unused — every input comes from in. st.db is read at call time
// (not at construction time) — see serverState's doc comment.
func handleGetV2(st *serverState) mcpx.Handler[getInput, any] {
	return func(_ context.Context, _ *mcpx.CallToolRequest, in getInput) (*mcpx.CallToolResult, any, error) {
		result, err := query.Get(st.db, buildGetRequest(in))
		if err != nil {
			return mcpx.ErrorResult(err.Error()), nil, nil
		}
		return renderGetResult(in, result)
	}
}

// buildGetRequest maps getInput's fields onto the core's surface-neutral
// query.Request 1:1.
func buildGetRequest(in getInput) query.Request {
	return query.Request{
		Domain:      in.Domain,
		IDs:         in.IDs,
		Verbose:     in.Verbose,
		Types:       in.Types,
		Projects:    in.Projects,
		Categories:  in.Categories,
		Query:       in.Query,
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

// renderGetResult mirrors the old inline handler's per-domain response
// shaping (formerly getDocumentsV2/getBacklogItemsV2/getRoadmapV2's return
// statements), now operating on the core's already-computed query.Result
// instead of deriving it inline.
func renderGetResult(in getInput, result query.Result) (*mcpx.CallToolResult, any, error) {
	switch in.Domain {
	case "kb":
		if len(in.IDs) > 0 {
			return jsonResultV2(result.Docs)
		}
		return jsonResultV2(result.DocSummaries)
	case "backlog":
		if len(in.IDs) > 0 {
			return jsonResultV2(result.ItemsJSON)
		}
		return mcpx.TextResult(itemLinesTextV2(result.Items)), nil, nil
	case "roadmap":
		switch in.Format {
		case "md", "html":
			return mcpx.TextResult(result.Text), nil, nil
		}
		return jsonResultV2(result.Roadmap)
	default:
		// Unreachable: query.Get already errors (query.ErrDomainRequired)
		// for any domain other than kb|backlog|roadmap, including "".
		return mcpx.ErrorResult(query.ErrDomainRequired), nil, nil
	}
}
