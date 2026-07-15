package query

import (
	"database/sql"
	"errors"
	"strconv"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/edges"
	"dangernoodle.io/ouroboros/internal/roadmap"
	"dangernoodle.io/ouroboros/internal/store"
)

// ErrDomainRequired is the verbatim message the old mcp-go/mcpx handlers
// returned for both a missing domain key and an unrecognized domain value
// — preserved here as the single source of truth now that domain dispatch
// lives in the core; internal/app's getInput/searchInput.Domain doc comment
// explains why domain is schema-optional (so this message, not a generic
// schema-validation error, is what a missing key produces).
const ErrDomainRequired = `domain is required: must be "kb", "backlog", or "roadmap"`

// Get dispatches a read-by-id-or-filter request by req.Domain
// (kb|backlog|roadmap), mirroring the old handleGetV2 switch — now living in
// the surface-neutral core instead of the MCP handler.
func Get(db *sql.DB, req Request) (Result, error) {
	switch req.Domain {
	case "kb":
		return getDocuments(db, req)
	case "backlog":
		return getBacklogItems(db, req)
	case "roadmap":
		return getRoadmap(db, req)
	default:
		return Result{}, errors.New(ErrDomainRequired)
	}
}

// getDocuments ports getDocumentsV2: IDs fetch, or filters list.
func getDocuments(db *sql.DB, req Request) (Result, error) {
	if len(req.IDs) > 0 {
		ids := make([]int64, 0, len(req.IDs))
		for _, raw := range req.IDs {
			f, ok := raw.(float64)
			if !ok {
				return Result{}, errors.New("ids for domain=kb must be integers")
			}
			ids = append(ids, int64(f))
		}

		fetched, err := store.GetDocuments(db, ids)
		if err != nil {
			return Result{}, err
		}
		byID := make(map[int64]*store.Document, len(fetched))
		for i := range fetched {
			byID[fetched[i].ID] = &fetched[i]
		}

		docs := make([]DocResult, 0, len(ids))
		for _, id := range ids {
			doc, ok := byID[id]
			if !ok {
				continue // omit misses
			}

			if !req.Verbose {
				doc.Notes = ""
				doc.SessionID = ""
				docs = append(docs, DocResult{Document: doc})
				continue
			}
			doc.SessionID = ""

			edgeList, err := edges.EdgesFor(db, edges.TypeKB, strconv.FormatInt(doc.ID, 10))
			if err != nil {
				return Result{}, err
			}
			docs = append(docs, DocResult{Document: doc, Edges: edgeList})
		}

		return Result{Docs: docs}, nil
	}

	summaries, err := store.QueryDocuments(db, req.Types, req.Projects, req.Categories, req.Query, req.Tags, req.Limit)
	if err != nil {
		return Result{}, err
	}

	return Result{DocSummaries: summaries}, nil
}

// getBacklogItems ports getBacklogItemsV2: IDs fetch, or filters list.
func getBacklogItems(db *sql.DB, req Request) (Result, error) {
	if len(req.IDs) > 0 {
		ids := make([]string, 0, len(req.IDs))
		for _, raw := range req.IDs {
			s, ok := raw.(string)
			if !ok {
				return Result{}, errors.New(`ids for domain=backlog must be prefixed strings like "B1-728"`)
			}
			ids = append(ids, s)
		}

		fetched, err := backlog.GetItems(db, ids)
		if err != nil {
			return Result{}, err
		}

		items := make([]ItemResult, 0, len(fetched))
		for _, item := range fetched {
			if !req.Verbose {
				item.Notes = ""
				item.ProjectID = 0
				items = append(items, ItemResult{Item: item})
				continue
			}
			item.ProjectID = 0

			edgeList, err := edges.EdgesFor(db, edges.TypeItem, item.ID)
			if err != nil {
				return Result{}, err
			}
			items = append(items, ItemResult{Item: item, Edges: edgeList})
		}

		return Result{ItemsJSON: items}, nil
	}

	f, err := buildItemFilter(db, req)
	if err != nil {
		return Result{}, err
	}

	items, err := backlog.ListItems(db, f)
	if err != nil {
		return Result{}, err
	}

	return Result{Items: items}, nil
}

// getRoadmap ports getRoadmapV2: the singleton roadmap for Projects[0], as
// structured Result.Roadmap (default), or a Markdown/HTML Result.Text
// (Format md|html).
func getRoadmap(db *sql.DB, req Request) (Result, error) {
	if len(req.Projects) == 0 {
		return Result{}, errors.New("projects[0] (project name) is required for domain=roadmap")
	}

	rm, err := roadmap.Load(db, req.Projects[0])
	if err != nil {
		return Result{}, err
	}

	rm = roadmap.Filter(rm, req.Component, req.Epic)

	switch req.Format {
	case "md":
		epicLabels := backlog.EpicLabels(db, roadmap.EpicIDs(rm))
		return Result{Text: roadmap.RenderMarkdown(rm, req.By, epicLabels, nil)}, nil
	case "html":
		epicIDs := roadmap.EpicIDs(rm)
		epicLabels := backlog.EpicLabels(db, epicIDs)
		blockedEpics := backlog.BlockedEpicsFor(db, epicIDs)
		return Result{Text: roadmap.RenderHTML(rm, req.By, epicLabels, blockedEpics)}, nil
	}

	return Result{Roadmap: rm}, nil
}
