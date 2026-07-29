package app

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/dangernoodle-io/shesha/mcpx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/roadmap"
	"dangernoodle.io/ouroboros/internal/store"
)

// brokenFTSDB returns a fresh in-memory db with documents_fts dropped, so a
// store.SearchDocuments call fails — mirrors TestHandleSearch_DomainRoadmap_Error's
// broken-FTS-index approach for exercising a SearchDocuments error path.
func brokenFTSDB(t *testing.T) *sql.DB {
	t.Helper()
	bdb, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = bdb.Close() })
	require.NoError(t, store.ApplySchema(bdb))
	_, err = bdb.Exec("DROP TABLE documents_fts")
	require.NoError(t, err)
	return bdb
}

func callQueryV2(t *testing.T, in queryInputV2) *mcpx.CallToolResult {
	t.Helper()
	res, out, err := handleQueryV2(&serverState{db: db})(context.TODO(), &mcpx.CallToolRequest{}, in)
	require.NoError(t, err)
	require.Nil(t, out)
	return res
}

func unmarshalV2(t *testing.T, res *mcpx.CallToolResult, v any) {
	t.Helper()
	require.NoError(t, jsonUnmarshalText(res, v))
}

// --- fetch/filter mode (formerly handlers_get_v2_test.go) ---

// TestHandleQueryV2_MissingDomain_Errors mirrors TestHandleGet_MissingDomain_Errors.
func TestHandleQueryV2_MissingDomain_Errors(t *testing.T) {
	resetDB(t)
	res := callQueryV2(t, queryInputV2{})
	require.True(t, res.IsError)
	assert.Equal(t, `domain is required: must be "kb", "backlog", or "roadmap"`, mcpx.ResultText(res))
}

// TestHandleQueryV2_InvalidDomain_Errors mirrors TestHandleGet_InvalidDomain_Errors.
func TestHandleQueryV2_InvalidDomain_Errors(t *testing.T) {
	resetDB(t)
	res := callQueryV2(t, queryInputV2{Domain: "bogus"})
	require.True(t, res.IsError)
	assert.Equal(t, `domain is required: must be "kb", "backlog", or "roadmap"`, mcpx.ResultText(res))
}

// TestHandleQueryV2_DomainKB_IdsFetch mirrors TestHandleGetBatch.
func TestHandleQueryV2_DomainKB_IdsFetch(t *testing.T) {
	resetDB(t)

	doc, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "Use PostgreSQL", Content: "c1"})
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "kb", IDs: []any{float64(doc.ID)}})
	require.False(t, res.IsError)

	var docs []map[string]any
	unmarshalV2(t, res, &docs)
	require.Len(t, docs, 1)
	assert.Equal(t, "Use PostgreSQL", docs[0]["title"])
	assert.Nil(t, docs[0]["notes"])
	assert.Nil(t, docs[0]["session_id"])
}

// TestHandleQueryV2_DomainKB_IdsVerbose mirrors TestHandleGet_BatchVerbose.
func TestHandleQueryV2_DomainKB_IdsVerbose(t *testing.T) {
	resetDB(t)

	doc, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "with notes", Content: "c1", Notes: "narrative"})
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "kb", IDs: []any{float64(doc.ID)}, Verbose: true})
	require.False(t, res.IsError)

	var docs []map[string]any
	unmarshalV2(t, res, &docs)
	require.Len(t, docs, 1)
	assert.Equal(t, "narrative", docs[0]["notes"])
}

// TestHandleQueryV2_DomainKB_IdsWrongType mirrors TestHandleGetDomainKB_IdsWrongType.
func TestHandleQueryV2_DomainKB_IdsWrongType(t *testing.T) {
	resetDB(t)
	res := callQueryV2(t, queryInputV2{Domain: "kb", IDs: []any{"not-a-number"}})
	require.True(t, res.IsError)
	assert.Equal(t, "ids for domain=kb must be integers", mcpx.ResultText(res))
}

// TestHandleQueryV2_DomainKB_Filter mirrors TestHandleGet_MultiTypes-style filter list.
func TestHandleQueryV2_DomainKB_Filter(t *testing.T) {
	resetDB(t)
	_, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "d1", Content: "c1"})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{Type: "fact", Project: "acme-corp", Title: "f1", Content: "c2"})
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "kb", Types: []string{"decision"}})
	require.False(t, res.IsError)

	var summaries []map[string]any
	unmarshalV2(t, res, &summaries)
	require.Len(t, summaries, 1)
	assert.Equal(t, "d1", summaries[0]["title"])
}

// TestHandleQueryV2_DomainBacklog_IdsFetch mirrors TestHandleGet_DomainBacklog_IdsFetch.
func TestHandleQueryV2_DomainBacklog_IdsFetch(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "fix the bug", "desc", "", "", "")
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "backlog", IDs: []any{item.ID}})
	require.False(t, res.IsError)

	var items []map[string]any
	unmarshalV2(t, res, &items)
	require.Len(t, items, 1)
	assert.Equal(t, "fix the bug", items[0]["title"])
	assert.Nil(t, items[0]["project_id"])
}

// TestHandleQueryV2_DomainBacklog_IdsVerbose mirrors TestHandleGet_DomainBacklog_IdsFetchVerbose.
func TestHandleQueryV2_DomainBacklog_IdsVerbose(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "with notes", "desc", "narrative", "", "")
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "backlog", IDs: []any{item.ID}, Verbose: true})
	require.False(t, res.IsError)

	var items []map[string]any
	unmarshalV2(t, res, &items)
	require.Len(t, items, 1)
	assert.Equal(t, "narrative", items[0]["notes"])
}

// TestHandleQueryV2_DomainBacklog_IdsWrongType mirrors TestHandleGet_DomainBacklog_IdsWrongType.
func TestHandleQueryV2_DomainBacklog_IdsWrongType(t *testing.T) {
	resetAllDB(t)
	res := callQueryV2(t, queryInputV2{Domain: "backlog", IDs: []any{float64(728)}})
	require.True(t, res.IsError)
	assert.Equal(t, `ids for domain=backlog must be prefixed strings like "B1-728"`, mcpx.ResultText(res))
}

// TestHandleQueryV2_DomainBacklog_FilteredList mirrors TestHandleGet_DomainBacklog_FilteredList.
func TestHandleQueryV2_DomainBacklog_FilteredList(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item one", "desc", "", "", "")
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "backlog", Projects: []string{"acme-corp"}})
	require.False(t, res.IsError)
	assert.Contains(t, mcpx.ResultText(res), "item one")
}

// TestHandleQueryV2_DomainBacklog_NoItems_ReturnsNoItemsText mirrors
// TestHandleGet_DomainBacklog_NoItems_ReturnsNoItemsText.
func TestHandleQueryV2_DomainBacklog_NoItems_ReturnsNoItemsText(t *testing.T) {
	resetAllDB(t)
	res := callQueryV2(t, queryInputV2{Domain: "backlog"})
	require.False(t, res.IsError)
	assert.Equal(t, "no items", mcpx.ResultText(res))
}

// TestHandleQueryV2_DomainRoadmap_Structured mirrors TestHandleGet_DomainRoadmap_Structured.
func TestHandleQueryV2_DomainRoadmap_Structured(t *testing.T) {
	resetDB(t)
	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "Structured item"})
		return err
	}))

	res := callQueryV2(t, queryInputV2{Domain: "roadmap", Projects: []string{"acme-corp"}})
	require.False(t, res.IsError)

	var rm roadmap.Roadmap
	unmarshalV2(t, res, &rm)
	require.Len(t, rm.Sections.Now, 1)
	assert.Equal(t, "Structured item", rm.Sections.Now[0].Title)
}

// TestHandleQueryV2_DomainRoadmap_Markdown mirrors TestHandleGet_DomainRoadmap_Markdown.
func TestHandleQueryV2_DomainRoadmap_Markdown(t *testing.T) {
	resetDB(t)
	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "MD item"})
		return err
	}))

	res := callQueryV2(t, queryInputV2{Domain: "roadmap", Projects: []string{"acme-corp"}, Format: "md"})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	assert.Contains(t, text, "# Roadmap")
	assert.Contains(t, text, "MD item")
}

// TestHandleQueryV2_DomainRoadmap_ByEpicResolvesLabel mirrors
// TestHandleGet_DomainRoadmap_ByEpicResolvesLabel.
func TestHandleQueryV2_DomainRoadmap_ByEpicResolvesLabel(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	epicItem, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "EPIC: WiFi map", "", "", "", "")
	require.NoError(t, err)

	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "epic item", Epic: epicItem.ID})
		return err
	}))

	res := callQueryV2(t, queryInputV2{Domain: "roadmap", Projects: []string{"acme-corp"}, Format: "md", By: "epic"})
	require.False(t, res.IsError)
	assert.Contains(t, mcpx.ResultText(res), "#### epic: WiFi map")
}

// TestHandleQueryV2_DomainRoadmap_HTML mirrors TestHandleGet_DomainRoadmap_HTML.
func TestHandleQueryV2_DomainRoadmap_HTML(t *testing.T) {
	resetDB(t)
	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "HTML item", Component: "widget"})
		return err
	}))

	res := callQueryV2(t, queryInputV2{Domain: "roadmap", Projects: []string{"acme-corp"}, Format: "html"})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	assert.Contains(t, text, "<style>")
	assert.Contains(t, text, "HTML item")
	assert.Contains(t, text, "component: widget")
}

// TestHandleQueryV2_DomainRoadmap_ComponentAndEpicFilters mirrors
// TestHandleGet_DomainRoadmap_ComponentAndEpicFilters.
func TestHandleQueryV2_DomainRoadmap_ComponentAndEpicFilters(t *testing.T) {
	resetDB(t)
	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		if _, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "matches", Component: "widget", Epic: "BB-707"}); err != nil {
			return err
		}
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "other", Component: "gadget"})
		return err
	}))

	res := callQueryV2(t, queryInputV2{Domain: "roadmap", Projects: []string{"acme-corp"}, Component: "widget", Epic: "BB-707"})
	require.False(t, res.IsError)

	var rm roadmap.Roadmap
	unmarshalV2(t, res, &rm)
	require.Len(t, rm.Sections.Now, 1)
	assert.Equal(t, "matches", rm.Sections.Now[0].Title)
}

// TestHandleQueryV2_DomainRoadmap_MissingProject_Errors mirrors
// TestHandleGet_DomainRoadmap_MissingProject_Errors.
func TestHandleQueryV2_DomainRoadmap_MissingProject_Errors(t *testing.T) {
	resetDB(t)
	res := callQueryV2(t, queryInputV2{Domain: "roadmap"})
	require.True(t, res.IsError)
	assert.Equal(t, "projects[0] (project name) is required for domain=roadmap", mcpx.ResultText(res))
}

// TestHandleQueryV2_DomainKB_IdsFetchWithMiss mirrors TestHandleGetBatchWithMiss:
// a nonexistent id in ids[] is silently omitted, not an error.
func TestHandleQueryV2_DomainKB_IdsFetchWithMiss(t *testing.T) {
	resetDB(t)
	doc, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "exists", Content: "c1"})
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "kb", IDs: []any{float64(doc.ID), float64(999999)}})
	require.False(t, res.IsError)

	var docs []map[string]any
	unmarshalV2(t, res, &docs)
	require.Len(t, docs, 1)
	assert.Equal(t, "exists", docs[0]["title"])
}

// TestHandleQueryV2_DomainKB_IdsEmptyArray_RegressionGuard mirrors
// TestHandleGetDomainKB_IdsEmptyArray_RegressionGuard: an explicit empty
// ids[] falls through to filter/list mode, not an error.
func TestHandleQueryV2_DomainKB_IdsEmptyArray_RegressionGuard(t *testing.T) {
	resetDB(t)
	_, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "d1", Content: "c1"})
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "kb", IDs: []any{}})
	require.False(t, res.IsError)

	var summaries []map[string]any
	unmarshalV2(t, res, &summaries)
	require.Len(t, summaries, 1)
}

// TestHandleQueryV2_DomainBacklog_PriorityFilter mirrors TestHandleGet_DomainBacklog_PriorityFilter.
func TestHandleQueryV2_DomainBacklog_PriorityFilter(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Critical", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P2", "Normal", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P5", "Low", "", "", "", "")
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "backlog", Projects: []string{"acme-corp"}, PriorityMin: "P1", PriorityMax: "P3"})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	assert.Contains(t, text, "Normal")
	assert.NotContains(t, text, "Critical")
	assert.NotContains(t, text, "Low")
}

// TestHandleQueryV2_DomainBacklog_PriorityMinInvalid_Errors mirrors
// TestHandleGet_DomainBacklog_PriorityMinInvalid_Errors.
func TestHandleQueryV2_DomainBacklog_PriorityMinInvalid_Errors(t *testing.T) {
	resetAllDB(t)
	res := callQueryV2(t, queryInputV2{Domain: "backlog", PriorityMin: "bogus"})
	require.True(t, res.IsError)
}

// TestHandleQueryV2_DomainBacklog_StatusFilter mirrors TestHandleGet_DomainBacklog_StatusFilter.
func TestHandleQueryV2_DomainBacklog_StatusFilter(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "done item", "", "", "", "")
	require.NoError(t, err)
	_, err = db.Exec("UPDATE items SET status = 'done' WHERE id = ?", item.ID)
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "open item", "", "", "", "")
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "backlog", Projects: []string{"acme-corp"}, Status: "done"})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	assert.Contains(t, text, "done item")
	assert.NotContains(t, text, "open item")
}

// TestHandleQueryV2_DomainBacklog_ComponentFilter mirrors TestHandleGet_DomainBacklog_ComponentFilter.
func TestHandleQueryV2_DomainBacklog_ComponentFilter(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "widget item", "", "", "widget", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "gadget item", "", "", "gadget", "")
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "backlog", Projects: []string{"acme-corp"}, Component: "widget"})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	assert.Contains(t, text, "widget item")
	assert.NotContains(t, text, "gadget item")
}

// TestHandleQueryV2_DomainBacklog_EpicFilter mirrors TestHandleGet_DomainBacklog_EpicFilter.
func TestHandleQueryV2_DomainBacklog_EpicFilter(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	epic, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "EPIC: widget", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "child item", "", "", "", epic.ID)
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "unrelated item", "", "", "", "")
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "backlog", Projects: []string{"acme-corp"}, Epic: epic.ID})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	assert.Contains(t, text, "child item")
	assert.NotContains(t, text, "unrelated item")
}

// TestHandleQueryV2_DomainBacklog_EpicsOnlyFilter mirrors TestHandleGet_DomainBacklog_EpicsOnlyFilter.
func TestHandleQueryV2_DomainBacklog_EpicsOnlyFilter(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "EPIC: widget", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "plain item", "", "", "", "")
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "backlog", Projects: []string{"acme-corp"}, EpicsOnly: true})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	assert.Contains(t, text, "EPIC: widget")
	assert.NotContains(t, text, "plain item")
}

// TestHandleQueryV2_DomainBacklog_SinceFilter mirrors TestHandleGet_DomainBacklog_SinceFilter.
func TestHandleQueryV2_DomainBacklog_SinceFilter(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	old, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "old item", "", "", "", "")
	require.NoError(t, err)
	_, err = db.Exec("UPDATE items SET created = ? WHERE id = ?", "2020-01-01T00:00:00Z", old.ID)
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "recent item", "", "", "", "")
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "backlog", Projects: []string{"acme-corp"}, Since: "24h"})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	assert.Contains(t, text, "recent item")
	assert.NotContains(t, text, "old item")
}

// TestHandleQueryV2_DomainBacklog_SinceInvalid_Errors mirrors TestHandleGet_DomainBacklog_SinceInvalid_Errors.
func TestHandleQueryV2_DomainBacklog_SinceInvalid_Errors(t *testing.T) {
	resetAllDB(t)
	res := callQueryV2(t, queryInputV2{Domain: "backlog", Since: "not-a-date"})
	require.True(t, res.IsError)
}

// TestHandleQueryV2_DomainBacklog_SortCreated mirrors TestHandleGet_DomainBacklog_SortCreated.
func TestHandleQueryV2_DomainBacklog_SortCreated(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	older, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "older", "", "", "", "")
	require.NoError(t, err)
	_, err = db.Exec("UPDATE items SET created = ? WHERE id = ?", "2025-01-01T00:00:00Z", older.ID)
	require.NoError(t, err)
	newer, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P6", "newer", "", "", "", "")
	require.NoError(t, err)
	_, err = db.Exec("UPDATE items SET created = ? WHERE id = ?", "2026-01-01T00:00:00Z", newer.ID)
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "backlog", Projects: []string{"acme-corp"}, Sort: "created"})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	newerIdx := strings.Index(text, "newer")
	olderIdx := strings.Index(text, "older")
	require.NotEqual(t, -1, newerIdx)
	require.NotEqual(t, -1, olderIdx)
	assert.Less(t, newerIdx, olderIdx)
}

// TestHandleQueryV2_DomainBacklog_SortInvalid_Errors mirrors TestHandleGet_DomainBacklog_SortInvalid_Errors.
func TestHandleQueryV2_DomainBacklog_SortInvalid_Errors(t *testing.T) {
	resetAllDB(t)
	res := callQueryV2(t, queryInputV2{Domain: "backlog", Sort: "bogus"})
	require.True(t, res.IsError)
}

// TestHandleQueryV2_DomainBacklog_NonexistentProject_Errors mirrors
// TestHandleGet_DomainBacklog_NonexistentProject_Errors.
func TestHandleQueryV2_DomainBacklog_NonexistentProject_Errors(t *testing.T) {
	resetAllDB(t)
	res := callQueryV2(t, queryInputV2{Domain: "backlog", Projects: []string{"nonexistent"}})
	require.True(t, res.IsError)
}

// TestHandleQueryV2_DomainBacklog_IdsFetchMiss mirrors TestHandleGet_DomainBacklog_IdsFetchMiss:
// unlike kb, a backlog ids[] miss errors the whole call.
func TestHandleQueryV2_DomainBacklog_IdsFetchMiss(t *testing.T) {
	resetAllDB(t)
	res := callQueryV2(t, queryInputV2{Domain: "backlog", IDs: []any{"AC-9999"}})
	require.True(t, res.IsError)
}

// TestHandleQueryV2_DomainRoadmap_LoadError mirrors TestHandleGet_DomainRoadmap_LoadError:
// corrupt roadmap metadata surfaces roadmap.Load's error as a tool error.
func TestHandleQueryV2_DomainRoadmap_LoadError(t *testing.T) {
	resetDB(t)
	_, err := store.UpsertDocument(db, store.Document{
		Type: "roadmap", Project: "acme-corp", Title: "roadmap", Content: "# Roadmap",
		Metadata: map[string]string{"data": "not json"},
	})
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "roadmap", Projects: []string{"acme-corp"}})
	require.True(t, res.IsError)
}

// --- full-text search mode (formerly handlers_search_v2_test.go) ---

// TestHandleQueryV2_Search_DomainKB_SingleQuery mirrors TestHandleSearch_SingleQuery_BackwardsCompat.
func TestHandleQueryV2_Search_DomainKB_SingleQuery(t *testing.T) {
	resetDB(t)
	_, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "ouroboros", Title: "tiktoken", Content: "Use tiktoken for token counting"})
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "kb", Query: "tiktoken"})
	require.False(t, res.IsError)

	var summaries []map[string]any
	require.NoError(t, jsonUnmarshalText(res, &summaries))
	require.Len(t, summaries, 1)
	assert.Equal(t, "tiktoken", summaries[0]["title"])
}

// TestHandleQueryV2_Search_DomainKB_QueriesBatch mirrors TestHandleSearch_Batch_PositionalResults.
func TestHandleQueryV2_Search_DomainKB_QueriesBatch(t *testing.T) {
	resetDB(t)
	_, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "alpha widget", Content: "c"})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "beta gadget", Content: "c"})
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "kb", Queries: []string{"widget", "gadget", "nonexistentterm"}})
	require.False(t, res.IsError)

	var resultSets [][]map[string]any
	require.NoError(t, jsonUnmarshalText(res, &resultSets))
	require.Len(t, resultSets, 3)
	require.Len(t, resultSets[0], 1)
	assert.Equal(t, "alpha widget", resultSets[0][0]["title"])
	require.Len(t, resultSets[1], 1)
	assert.Equal(t, "beta gadget", resultSets[1][0]["title"])
	assert.Len(t, resultSets[2], 0) // empty-not-nil invariant
	assert.NotNil(t, resultSets[2])
}

// TestHandleQueryV2_Search_DomainBacklog_ReturnsMatches mirrors TestHandleSearch_DomainBacklog_ReturnsMatches.
func TestHandleQueryV2_Search_DomainBacklog_ReturnsMatches(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "fix the flux capacitor", "desc", "", "", "")
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "backlog", Query: "flux"})
	require.False(t, res.IsError)
	assert.Contains(t, mcpx.ResultText(res), "flux capacitor")
}

// TestHandleQueryV2_Search_DomainBacklog_ORFallback_PartialMatch is the
// OU-346 regression at the MCP wire level: a multi-term query where AND
// matches nothing but OR matches something surfaces the partial match, and
// the response text says so instead of silently reading like an exact hit.
func TestHandleQueryV2_Search_DomainBacklog_ORFallback_PartialMatch(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "bb_data egress design", "notes about egress, no bb_event or bb_sink here", "", "", "")
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "backlog", Query: "bb_data bb_event bb_sink egress transport"})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	assert.Contains(t, text, "bb_data egress design")
	assert.Contains(t, text, "relaxed")
}

// TestHandleQueryV2_Search_DomainBacklog_ANDMatches_NoRelaxedNote confirms
// an exact AND match's response carries no relaxed note.
func TestHandleQueryV2_Search_DomainBacklog_ANDMatches_NoRelaxedNote(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "fix the flux capacitor", "desc", "", "", "")
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "backlog", Query: "flux"})
	require.False(t, res.IsError)
	assert.NotContains(t, mcpx.ResultText(res), "relaxed")
}

// TestHandleQueryV2_Search_DomainKB_ORFallback_PartialMatch is the OU-346
// regression for kb at the MCP wire level: DocumentSummary.Relaxed is
// visible on the JSON response.
func TestHandleQueryV2_Search_DomainKB_ORFallback_PartialMatch(t *testing.T) {
	resetAllDB(t)
	_, err := store.UpsertDocument(db, store.Document{Type: "note", Project: "acme-corp", Title: "bb_data egress design", Content: "notes about egress, no bb_event or bb_sink here"})
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "kb", Query: "bb_data bb_event bb_sink egress transport"})
	require.False(t, res.IsError)

	var resp []map[string]any
	require.NoError(t, jsonUnmarshalText(res, &resp))
	require.Len(t, resp, 1)
	assert.Equal(t, true, resp[0]["relaxed"])
}

// TestHandleQueryV2_DomainBacklog_InvertedPriorityRange_Errors is the OU-347
// regression at the MCP wire level for Get's list mode.
func TestHandleQueryV2_DomainBacklog_InvertedPriorityRange_Errors(t *testing.T) {
	resetAllDB(t)
	res := callQueryV2(t, queryInputV2{Domain: "backlog", PriorityMin: "P2", PriorityMax: "P1"})
	require.True(t, res.IsError)
	assert.Contains(t, mcpx.ResultText(res), "invalid priority range")
}

// TestHandleQueryV2_Search_DomainBacklog_InvertedPriorityRange_Errors is the
// OU-347 regression at the MCP wire level for Search mode.
func TestHandleQueryV2_Search_DomainBacklog_InvertedPriorityRange_Errors(t *testing.T) {
	resetAllDB(t)
	res := callQueryV2(t, queryInputV2{Domain: "backlog", Query: "flux", PriorityMin: "P2", PriorityMax: "P1"})
	require.True(t, res.IsError)
	assert.Contains(t, mcpx.ResultText(res), "invalid priority range")
}

// TestHandleQueryV2_NoQueryFallsThroughToFilterMode documents a behavior
// change from the old separate get/search tools: the unified query tool
// dispatches search mode ONLY when query/queries[] is present (per OU-323),
// so the old search tool's "query or queries is required" /
// "query is required" errors (raised when domain=backlog/kb/roadmap was
// called with neither) are unreachable through query -- an empty
// query/queries[] instead falls through to fetch/filter mode, exactly as
// domain-only get always did.
func TestHandleQueryV2_NoQueryFallsThroughToFilterMode(t *testing.T) {
	resetAllDB(t)
	res := callQueryV2(t, queryInputV2{Domain: "backlog", Queries: []string{}})
	require.False(t, res.IsError)
	assert.Equal(t, "no items", mcpx.ResultText(res))
}

// TestHandleQueryV2_Search_DomainRoadmap mirrors TestHandleSearch_DomainRoadmap.
func TestHandleQueryV2_Search_DomainRoadmap(t *testing.T) {
	resetDB(t)
	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "Unique searchable widget"})
		return err
	}))

	res := callQueryV2(t, queryInputV2{Domain: "roadmap", Query: "widget"})
	require.False(t, res.IsError)

	var resp []map[string]any
	require.NoError(t, jsonUnmarshalText(res, &resp))
	require.Len(t, resp, 1)
	assert.Equal(t, "roadmap", resp[0]["type"])
}

// TestHandleQueryV2_Search_DomainBacklog_EpicFilter mirrors TestHandleSearch_DomainBacklog_EpicFilter.
func TestHandleQueryV2_Search_DomainBacklog_EpicFilter(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	epic, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "EPIC: widget", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "flux child item", "", "", "", epic.ID)
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "flux unrelated item", "", "", "", "")
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "backlog", Query: "flux", Epic: epic.ID})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	assert.Contains(t, text, "child item")
	assert.NotContains(t, text, "unrelated item")
}

// TestHandleQueryV2_Search_DomainBacklog_SinceFilter mirrors TestHandleSearch_DomainBacklog_SinceFilter.
func TestHandleQueryV2_Search_DomainBacklog_SinceFilter(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	old, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "flux old item", "", "", "", "")
	require.NoError(t, err)
	_, err = db.Exec("UPDATE items SET created = ? WHERE id = ?", "2020-01-01T00:00:00Z", old.ID)
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "flux recent item", "", "", "", "")
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "backlog", Query: "flux", Since: "24h"})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	assert.Contains(t, text, "recent item")
	assert.NotContains(t, text, "old item")
}

// TestHandleQueryV2_Search_DomainBacklog_SortCreated mirrors TestHandleSearch_DomainBacklog_SortCreated.
func TestHandleQueryV2_Search_DomainBacklog_SortCreated(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	older, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "flux older", "", "", "", "")
	require.NoError(t, err)
	_, err = db.Exec("UPDATE items SET created = ? WHERE id = ?", "2025-01-01T00:00:00Z", older.ID)
	require.NoError(t, err)
	newer, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P6", "flux newer", "", "", "", "")
	require.NoError(t, err)
	_, err = db.Exec("UPDATE items SET created = ? WHERE id = ?", "2026-01-01T00:00:00Z", newer.ID)
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "backlog", Query: "flux", Sort: "created"})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	newerIdx := strings.Index(text, "newer")
	olderIdx := strings.Index(text, "older")
	require.NotEqual(t, -1, newerIdx)
	require.NotEqual(t, -1, olderIdx)
	assert.Less(t, newerIdx, olderIdx)
}

// TestHandleQueryV2_Search_DomainBacklog_SortInvalid_Errors mirrors TestHandleSearch_DomainBacklog_SortInvalid_Errors.
func TestHandleQueryV2_Search_DomainBacklog_SortInvalid_Errors(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "flux item", "", "", "", "")
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "backlog", Query: "flux", Sort: "bogus"})
	require.True(t, res.IsError)
}

// TestHandleQueryV2_Search_DomainKB_WithLimit mirrors TestHandleSearch_SingleQuery_WithLimit.
func TestHandleQueryV2_Search_DomainKB_WithLimit(t *testing.T) {
	resetDB(t)
	_, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "limited widget", Content: "c"})
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "kb", Query: "widget", Limit: 5})
	require.False(t, res.IsError)

	var summaries []map[string]any
	require.NoError(t, jsonUnmarshalText(res, &summaries))
	require.Len(t, summaries, 1)
}

// TestHandleQueryV2_Search_DomainKB_SingleQueryError mirrors the
// store.SearchDocuments error path a broken FTS index triggers, for the
// single-query branch.
func TestHandleQueryV2_Search_DomainKB_SingleQueryError(t *testing.T) {
	bdb := brokenFTSDB(t)
	res, out, err := handleQueryV2(&serverState{db: bdb})(context.TODO(), &mcpx.CallToolRequest{}, queryInputV2{Domain: "kb", Query: "widget"})
	require.NoError(t, err)
	require.Nil(t, out)
	assert.True(t, res.IsError)
}

// TestHandleQueryV2_Search_DomainKB_QueriesBatchError mirrors the same error
// path for the queries[] batch branch.
func TestHandleQueryV2_Search_DomainKB_QueriesBatchError(t *testing.T) {
	bdb := brokenFTSDB(t)
	res, out, err := handleQueryV2(&serverState{db: bdb})(context.TODO(), &mcpx.CallToolRequest{}, queryInputV2{Domain: "kb", Queries: []string{"widget"}})
	require.NoError(t, err)
	require.Nil(t, out)
	assert.True(t, res.IsError)
}

// TestHandleQueryV2_Search_DomainRoadmap_Error mirrors TestHandleSearch_DomainRoadmap_Error.
func TestHandleQueryV2_Search_DomainRoadmap_Error(t *testing.T) {
	bdb := brokenFTSDB(t)
	res, out, err := handleQueryV2(&serverState{db: bdb})(context.TODO(), &mcpx.CallToolRequest{}, queryInputV2{Domain: "roadmap", Query: "widget"})
	require.NoError(t, err)
	require.Nil(t, out)
	assert.True(t, res.IsError)
}

// TestHandleQueryV2_DomainKB_IdsWinOverQuery asserts ids[] takes precedence
// over query when both are set: the call must return the id-fetched doc
// (with notes/session_id stripped per non-verbose ids fetch), not a
// full-text search result.
func TestHandleQueryV2_DomainKB_IdsWinOverQuery(t *testing.T) {
	resetDB(t)
	doc, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "tiktoken", Content: "Use tiktoken for token counting"})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "unrelated widget", Content: "c"})
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "kb", IDs: []any{float64(doc.ID)}, Query: "widget"})
	require.False(t, res.IsError)

	var docs []map[string]any
	unmarshalV2(t, res, &docs)
	require.Len(t, docs, 1)
	assert.Equal(t, "tiktoken", docs[0]["title"])
}

// TestHandleQueryV2_DomainBacklog_IdsWinOverQuery is the backlog counterpart
// of TestHandleQueryV2_DomainKB_IdsWinOverQuery.
func TestHandleQueryV2_DomainBacklog_IdsWinOverQuery(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "fix the bug", "desc", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "flux capacitor", "desc", "", "", "")
	require.NoError(t, err)

	res := callQueryV2(t, queryInputV2{Domain: "backlog", IDs: []any{item.ID}, Query: "flux"})
	require.False(t, res.IsError)

	var items []map[string]any
	unmarshalV2(t, res, &items)
	require.Len(t, items, 1)
	assert.Equal(t, "fix the bug", items[0]["title"])
}

// TestJSONResultV2_MarshalError mirrors TestJSONResult_MarshalError.
func TestJSONResultV2_MarshalError(t *testing.T) {
	type unmarshalable struct {
		Ch chan int
	}
	res, out, err := jsonResultV2(unmarshalable{Ch: make(chan int)})
	require.NoError(t, err)
	require.Nil(t, out)
	require.NotNil(t, res)
	assert.True(t, res.IsError)
}
