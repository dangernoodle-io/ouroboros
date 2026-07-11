package app

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/dangernoodle-io/mcpkit/mcpx"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/edges"
	"dangernoodle.io/ouroboros/internal/roadmap"
	"dangernoodle.io/ouroboros/internal/store"
)

// handleGetV2 dispatches by required domain (kb|backlog|roadmap), reading
// from the typed getInput instead of an untyped arg map. req is unused —
// every input comes from in. st.db is read at call time (not at
// construction time) — see serverState's doc comment.
func handleGetV2(st *serverState) mcpx.Handler[getInput, any] {
	return func(_ context.Context, _ *mcpx.CallToolRequest, in getInput) (*mcpx.CallToolResult, any, error) {
		// Explicit missing/empty check (Domain is deliberately not
		// schema-required — see getInput.Domain's comment): both an omitted
		// domain key and domain="" must reach here and emit the same
		// verbatim message, matching old mcp-go behavior.
		if in.Domain == "" {
			return mcpx.ErrorResult(errDomainRequired), nil, nil
		}
		switch in.Domain {
		case "kb":
			return getDocumentsV2(st.db, in)
		case "backlog":
			return getBacklogItemsV2(st.db, in)
		case "roadmap":
			return getRoadmapV2(st.db, in)
		default:
			return mcpx.ErrorResult(errDomainRequired), nil, nil
		}
	}
}

// getDocumentsV2 ports getDocuments: ids[] fetch, or filters list.
func getDocumentsV2(db *sql.DB, in getInput) (*mcpx.CallToolResult, any, error) {
	if len(in.IDs) > 0 {
		ids := make([]int64, 0, len(in.IDs))
		for _, raw := range in.IDs {
			f, ok := raw.(float64)
			if !ok {
				return mcpx.ErrorResult("ids for domain=kb must be integers"), nil, nil
			}
			ids = append(ids, int64(f))
		}

		fetched, err := store.GetDocuments(db, ids)
		if err != nil {
			return mcpx.ErrorResult(err.Error()), nil, nil
		}
		byID := make(map[int64]*store.Document, len(fetched))
		for i := range fetched {
			byID[fetched[i].ID] = &fetched[i]
		}

		docs := make([]any, 0, len(ids))
		for _, id := range ids {
			doc, ok := byID[id]
			if !ok {
				continue // omit misses
			}

			if !in.Verbose {
				doc.Notes = ""
				doc.SessionID = ""
				docs = append(docs, doc)
				continue
			}
			doc.SessionID = ""

			edgeList, err := edges.EdgesFor(db, edges.TypeKB, strconv.FormatInt(doc.ID, 10))
			if err != nil {
				return mcpx.ErrorResult(err.Error()), nil, nil
			}
			docs = append(docs, docWithEdges{Document: doc, Edges: edgeList})
		}

		return jsonResultV2(docs)
	}

	summaries, err := store.QueryDocuments(db, in.Types, in.Projects, in.Categories, in.Query, in.Tags, in.Limit)
	if err != nil {
		return mcpx.ErrorResult(err.Error()), nil, nil
	}

	return jsonResultV2(summaries)
}

// getBacklogItemsV2 ports getBacklogItems: ids[] fetch, or filters list.
func getBacklogItemsV2(db *sql.DB, in getInput) (*mcpx.CallToolResult, any, error) {
	if len(in.IDs) > 0 {
		ids := make([]string, 0, len(in.IDs))
		for _, raw := range in.IDs {
			s, ok := raw.(string)
			if !ok {
				return mcpx.ErrorResult(`ids for domain=backlog must be prefixed strings like "B1-728"`), nil, nil
			}
			ids = append(ids, s)
		}

		fetched, err := backlog.GetItems(db, ids)
		if err != nil {
			return mcpx.ErrorResult(err.Error()), nil, nil
		}

		items := make([]any, 0, len(fetched))
		for _, item := range fetched {
			if !in.Verbose {
				item.Notes = ""
				item.ProjectID = 0
				items = append(items, item)
				continue
			}
			item.ProjectID = 0

			edgeList, err := edges.EdgesFor(db, edges.TypeItem, item.ID)
			if err != nil {
				return mcpx.ErrorResult(err.Error()), nil, nil
			}
			items = append(items, itemWithEdges{Item: item, Edges: edgeList})
		}

		return jsonResultV2(items)
	}

	f, err := parseItemFilterV2(db, in.Projects, in.PriorityMin, in.PriorityMax, in.Status, in.Component, in.Epic, in.EpicsOnly, in.Since, in.Sort, in.Limit)
	if err != nil {
		return mcpx.ErrorResult(err.Error()), nil, nil
	}

	items, err := backlog.ListItems(db, f)
	if err != nil {
		return mcpx.ErrorResult(err.Error()), nil, nil
	}

	return mcpx.TextResult(itemLinesTextV2(items)), nil, nil
}

// getRoadmapV2 ports getRoadmap: the singleton roadmap for projects[0], as
// structured JSON (default), or a Markdown/HTML mirror (format=md|html).
func getRoadmapV2(db *sql.DB, in getInput) (*mcpx.CallToolResult, any, error) {
	if len(in.Projects) == 0 {
		return mcpx.ErrorResult("projects[0] (project name) is required for domain=roadmap"), nil, nil
	}

	rm, err := roadmap.Load(db, in.Projects[0])
	if err != nil {
		return mcpx.ErrorResult(err.Error()), nil, nil
	}

	rm = roadmap.Filter(rm, in.Component, in.Epic)

	switch in.Format {
	case "md":
		epicLabels := backlog.EpicLabels(db, roadmap.EpicIDs(rm))
		return mcpx.TextResult(roadmap.RenderMarkdown(rm, in.By, epicLabels, nil)), nil, nil
	case "html":
		epicIDs := roadmap.EpicIDs(rm)
		epicLabels := backlog.EpicLabels(db, epicIDs)
		blockedEpics := backlog.BlockedEpicsFor(db, epicIDs)
		return mcpx.TextResult(roadmap.RenderHTML(rm, in.By, epicLabels, blockedEpics)), nil, nil
	}

	return jsonResultV2(rm)
}
