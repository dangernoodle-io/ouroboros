package app

import (
	"context"
	"strings"
	"testing"

	"github.com/dangernoodle-io/mcpkit/mcpx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/roadmap"
	"dangernoodle.io/ouroboros/internal/store"
)

func callGetV2(t *testing.T, in getInput) *mcpx.CallToolResult {
	t.Helper()
	res, out, err := handleGetV2(db)(context.TODO(), &mcpx.CallToolRequest{}, in)
	require.NoError(t, err)
	require.Nil(t, out)
	return res
}

func unmarshalV2(t *testing.T, res *mcpx.CallToolResult, v any) {
	t.Helper()
	require.NoError(t, jsonUnmarshalText(res, v))
}

// TestHandleGetV2_MissingDomain_Errors mirrors TestHandleGet_MissingDomain_Errors.
func TestHandleGetV2_MissingDomain_Errors(t *testing.T) {
	resetDB(t)
	res := callGetV2(t, getInput{})
	require.True(t, res.IsError)
	assert.Equal(t, `domain is required: must be "kb", "backlog", or "roadmap"`, mcpx.ResultText(res))
}

// TestHandleGetV2_InvalidDomain_Errors mirrors TestHandleGet_InvalidDomain_Errors.
func TestHandleGetV2_InvalidDomain_Errors(t *testing.T) {
	resetDB(t)
	res := callGetV2(t, getInput{Domain: "bogus"})
	require.True(t, res.IsError)
	assert.Equal(t, `domain is required: must be "kb", "backlog", or "roadmap"`, mcpx.ResultText(res))
}

// TestHandleGetV2_DomainKB_IdsFetch mirrors TestHandleGetBatch.
func TestHandleGetV2_DomainKB_IdsFetch(t *testing.T) {
	resetDB(t)

	doc, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "Use PostgreSQL", Content: "c1"})
	require.NoError(t, err)

	res := callGetV2(t, getInput{Domain: "kb", IDs: []any{float64(doc.ID)}})
	require.False(t, res.IsError)

	var docs []map[string]any
	unmarshalV2(t, res, &docs)
	require.Len(t, docs, 1)
	assert.Equal(t, "Use PostgreSQL", docs[0]["title"])
	assert.Nil(t, docs[0]["notes"])
	assert.Nil(t, docs[0]["session_id"])
}

// TestHandleGetV2_DomainKB_IdsVerbose mirrors TestHandleGet_BatchVerbose.
func TestHandleGetV2_DomainKB_IdsVerbose(t *testing.T) {
	resetDB(t)

	doc, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "with notes", Content: "c1", Notes: "narrative"})
	require.NoError(t, err)

	res := callGetV2(t, getInput{Domain: "kb", IDs: []any{float64(doc.ID)}, Verbose: true})
	require.False(t, res.IsError)

	var docs []map[string]any
	unmarshalV2(t, res, &docs)
	require.Len(t, docs, 1)
	assert.Equal(t, "narrative", docs[0]["notes"])
}

// TestHandleGetV2_DomainKB_IdsWrongType mirrors TestHandleGetDomainKB_IdsWrongType.
func TestHandleGetV2_DomainKB_IdsWrongType(t *testing.T) {
	resetDB(t)
	res := callGetV2(t, getInput{Domain: "kb", IDs: []any{"not-a-number"}})
	require.True(t, res.IsError)
	assert.Equal(t, "ids for domain=kb must be integers", mcpx.ResultText(res))
}

// TestHandleGetV2_DomainKB_Filter mirrors TestHandleGet_MultiTypes-style filter list.
func TestHandleGetV2_DomainKB_Filter(t *testing.T) {
	resetDB(t)
	_, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "d1", Content: "c1"})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{Type: "fact", Project: "acme-corp", Title: "f1", Content: "c2"})
	require.NoError(t, err)

	res := callGetV2(t, getInput{Domain: "kb", Types: []string{"decision"}})
	require.False(t, res.IsError)

	var summaries []map[string]any
	unmarshalV2(t, res, &summaries)
	require.Len(t, summaries, 1)
	assert.Equal(t, "d1", summaries[0]["title"])
}

// TestHandleGetV2_DomainBacklog_IdsFetch mirrors TestHandleGet_DomainBacklog_IdsFetch.
func TestHandleGetV2_DomainBacklog_IdsFetch(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "fix the bug", "desc", "", "", "")
	require.NoError(t, err)

	res := callGetV2(t, getInput{Domain: "backlog", IDs: []any{item.ID}})
	require.False(t, res.IsError)

	var items []map[string]any
	unmarshalV2(t, res, &items)
	require.Len(t, items, 1)
	assert.Equal(t, "fix the bug", items[0]["title"])
	assert.Nil(t, items[0]["project_id"])
}

// TestHandleGetV2_DomainBacklog_IdsVerbose mirrors TestHandleGet_DomainBacklog_IdsFetchVerbose.
func TestHandleGetV2_DomainBacklog_IdsVerbose(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "with notes", "desc", "narrative", "", "")
	require.NoError(t, err)

	res := callGetV2(t, getInput{Domain: "backlog", IDs: []any{item.ID}, Verbose: true})
	require.False(t, res.IsError)

	var items []map[string]any
	unmarshalV2(t, res, &items)
	require.Len(t, items, 1)
	assert.Equal(t, "narrative", items[0]["notes"])
}

// TestHandleGetV2_DomainBacklog_IdsWrongType mirrors TestHandleGet_DomainBacklog_IdsWrongType.
func TestHandleGetV2_DomainBacklog_IdsWrongType(t *testing.T) {
	resetAllDB(t)
	res := callGetV2(t, getInput{Domain: "backlog", IDs: []any{float64(728)}})
	require.True(t, res.IsError)
	assert.Equal(t, `ids for domain=backlog must be prefixed strings like "B1-728"`, mcpx.ResultText(res))
}

// TestHandleGetV2_DomainBacklog_FilteredList mirrors TestHandleGet_DomainBacklog_FilteredList.
func TestHandleGetV2_DomainBacklog_FilteredList(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item one", "desc", "", "", "")
	require.NoError(t, err)

	res := callGetV2(t, getInput{Domain: "backlog", Projects: []string{"acme-corp"}})
	require.False(t, res.IsError)
	assert.Contains(t, mcpx.ResultText(res), "item one")
}

// TestHandleGetV2_DomainBacklog_NoItems_ReturnsNoItemsText mirrors
// TestHandleGet_DomainBacklog_NoItems_ReturnsNoItemsText.
func TestHandleGetV2_DomainBacklog_NoItems_ReturnsNoItemsText(t *testing.T) {
	resetAllDB(t)
	res := callGetV2(t, getInput{Domain: "backlog"})
	require.False(t, res.IsError)
	assert.Equal(t, "no items", mcpx.ResultText(res))
}

// TestHandleGetV2_DomainRoadmap_Structured mirrors TestHandleGet_DomainRoadmap_Structured.
func TestHandleGetV2_DomainRoadmap_Structured(t *testing.T) {
	resetDB(t)
	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "Structured item"})
		return err
	}))

	res := callGetV2(t, getInput{Domain: "roadmap", Projects: []string{"acme-corp"}})
	require.False(t, res.IsError)

	var rm roadmap.Roadmap
	unmarshalV2(t, res, &rm)
	require.Len(t, rm.Sections.Now, 1)
	assert.Equal(t, "Structured item", rm.Sections.Now[0].Title)
}

// TestHandleGetV2_DomainRoadmap_Markdown mirrors TestHandleGet_DomainRoadmap_Markdown.
func TestHandleGetV2_DomainRoadmap_Markdown(t *testing.T) {
	resetDB(t)
	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "MD item"})
		return err
	}))

	res := callGetV2(t, getInput{Domain: "roadmap", Projects: []string{"acme-corp"}, Format: "md"})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	assert.Contains(t, text, "# Roadmap")
	assert.Contains(t, text, "MD item")
}

// TestHandleGetV2_DomainRoadmap_ByEpicResolvesLabel mirrors
// TestHandleGet_DomainRoadmap_ByEpicResolvesLabel.
func TestHandleGetV2_DomainRoadmap_ByEpicResolvesLabel(t *testing.T) {
	resetDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	epicItem, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "EPIC: WiFi map", "", "", "", "")
	require.NoError(t, err)

	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "epic item", Epic: epicItem.ID})
		return err
	}))

	res := callGetV2(t, getInput{Domain: "roadmap", Projects: []string{"acme-corp"}, Format: "md", By: "epic"})
	require.False(t, res.IsError)
	assert.Contains(t, mcpx.ResultText(res), "#### epic: WiFi map")
}

// TestHandleGetV2_DomainRoadmap_HTML mirrors TestHandleGet_DomainRoadmap_HTML.
func TestHandleGetV2_DomainRoadmap_HTML(t *testing.T) {
	resetDB(t)
	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "HTML item", Component: "widget"})
		return err
	}))

	res := callGetV2(t, getInput{Domain: "roadmap", Projects: []string{"acme-corp"}, Format: "html"})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	assert.Contains(t, text, "<style>")
	assert.Contains(t, text, "HTML item")
	assert.Contains(t, text, "component: widget")
}

// TestHandleGetV2_DomainRoadmap_ComponentAndEpicFilters mirrors
// TestHandleGet_DomainRoadmap_ComponentAndEpicFilters.
func TestHandleGetV2_DomainRoadmap_ComponentAndEpicFilters(t *testing.T) {
	resetDB(t)
	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		if _, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "matches", Component: "widget", Epic: "BB-707"}); err != nil {
			return err
		}
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "other", Component: "gadget"})
		return err
	}))

	res := callGetV2(t, getInput{Domain: "roadmap", Projects: []string{"acme-corp"}, Component: "widget", Epic: "BB-707"})
	require.False(t, res.IsError)

	var rm roadmap.Roadmap
	unmarshalV2(t, res, &rm)
	require.Len(t, rm.Sections.Now, 1)
	assert.Equal(t, "matches", rm.Sections.Now[0].Title)
}

// TestHandleGetV2_DomainRoadmap_MissingProject_Errors mirrors
// TestHandleGet_DomainRoadmap_MissingProject_Errors.
func TestHandleGetV2_DomainRoadmap_MissingProject_Errors(t *testing.T) {
	resetDB(t)
	res := callGetV2(t, getInput{Domain: "roadmap"})
	require.True(t, res.IsError)
	assert.Equal(t, "projects[0] (project name) is required for domain=roadmap", mcpx.ResultText(res))
}

// TestHandleGetV2_DomainKB_IdsFetchWithMiss mirrors TestHandleGetBatchWithMiss:
// a nonexistent id in ids[] is silently omitted, not an error.
func TestHandleGetV2_DomainKB_IdsFetchWithMiss(t *testing.T) {
	resetDB(t)
	doc, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "exists", Content: "c1"})
	require.NoError(t, err)

	res := callGetV2(t, getInput{Domain: "kb", IDs: []any{float64(doc.ID), float64(999999)}})
	require.False(t, res.IsError)

	var docs []map[string]any
	unmarshalV2(t, res, &docs)
	require.Len(t, docs, 1)
	assert.Equal(t, "exists", docs[0]["title"])
}

// TestHandleGetV2_DomainKB_IdsEmptyArray_RegressionGuard mirrors
// TestHandleGetDomainKB_IdsEmptyArray_RegressionGuard: an explicit empty
// ids[] falls through to filter/list mode, not an error.
func TestHandleGetV2_DomainKB_IdsEmptyArray_RegressionGuard(t *testing.T) {
	resetDB(t)
	_, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "d1", Content: "c1"})
	require.NoError(t, err)

	res := callGetV2(t, getInput{Domain: "kb", IDs: []any{}})
	require.False(t, res.IsError)

	var summaries []map[string]any
	unmarshalV2(t, res, &summaries)
	require.Len(t, summaries, 1)
}

// TestHandleGetV2_DomainBacklog_PriorityFilter mirrors TestHandleGet_DomainBacklog_PriorityFilter.
func TestHandleGetV2_DomainBacklog_PriorityFilter(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Critical", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P2", "Normal", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P5", "Low", "", "", "", "")
	require.NoError(t, err)

	res := callGetV2(t, getInput{Domain: "backlog", Projects: []string{"acme-corp"}, PriorityMin: "P1", PriorityMax: "P3"})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	assert.Contains(t, text, "Normal")
	assert.NotContains(t, text, "Critical")
	assert.NotContains(t, text, "Low")
}

// TestHandleGetV2_DomainBacklog_PriorityMinInvalid_Errors mirrors
// TestHandleGet_DomainBacklog_PriorityMinInvalid_Errors.
func TestHandleGetV2_DomainBacklog_PriorityMinInvalid_Errors(t *testing.T) {
	resetAllDB(t)
	res := callGetV2(t, getInput{Domain: "backlog", PriorityMin: "bogus"})
	require.True(t, res.IsError)
}

// TestHandleGetV2_DomainBacklog_StatusFilter mirrors TestHandleGet_DomainBacklog_StatusFilter.
func TestHandleGetV2_DomainBacklog_StatusFilter(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "done item", "", "", "", "")
	require.NoError(t, err)
	_, err = db.Exec("UPDATE items SET status = 'done' WHERE id = ?", item.ID)
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "open item", "", "", "", "")
	require.NoError(t, err)

	res := callGetV2(t, getInput{Domain: "backlog", Projects: []string{"acme-corp"}, Status: "done"})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	assert.Contains(t, text, "done item")
	assert.NotContains(t, text, "open item")
}

// TestHandleGetV2_DomainBacklog_ComponentFilter mirrors TestHandleGet_DomainBacklog_ComponentFilter.
func TestHandleGetV2_DomainBacklog_ComponentFilter(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "widget item", "", "", "widget", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "gadget item", "", "", "gadget", "")
	require.NoError(t, err)

	res := callGetV2(t, getInput{Domain: "backlog", Projects: []string{"acme-corp"}, Component: "widget"})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	assert.Contains(t, text, "widget item")
	assert.NotContains(t, text, "gadget item")
}

// TestHandleGetV2_DomainBacklog_EpicFilter mirrors TestHandleGet_DomainBacklog_EpicFilter.
func TestHandleGetV2_DomainBacklog_EpicFilter(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	epic, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "EPIC: widget", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "child item", "", "", "", epic.ID)
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "unrelated item", "", "", "", "")
	require.NoError(t, err)

	res := callGetV2(t, getInput{Domain: "backlog", Projects: []string{"acme-corp"}, Epic: epic.ID})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	assert.Contains(t, text, "child item")
	assert.NotContains(t, text, "unrelated item")
}

// TestHandleGetV2_DomainBacklog_EpicsOnlyFilter mirrors TestHandleGet_DomainBacklog_EpicsOnlyFilter.
func TestHandleGetV2_DomainBacklog_EpicsOnlyFilter(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "EPIC: widget", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "plain item", "", "", "", "")
	require.NoError(t, err)

	res := callGetV2(t, getInput{Domain: "backlog", Projects: []string{"acme-corp"}, EpicsOnly: true})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	assert.Contains(t, text, "EPIC: widget")
	assert.NotContains(t, text, "plain item")
}

// TestHandleGetV2_DomainBacklog_SinceFilter mirrors TestHandleGet_DomainBacklog_SinceFilter.
func TestHandleGetV2_DomainBacklog_SinceFilter(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	old, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "old item", "", "", "", "")
	require.NoError(t, err)
	_, err = db.Exec("UPDATE items SET created = ? WHERE id = ?", "2020-01-01T00:00:00Z", old.ID)
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "recent item", "", "", "", "")
	require.NoError(t, err)

	res := callGetV2(t, getInput{Domain: "backlog", Projects: []string{"acme-corp"}, Since: "24h"})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	assert.Contains(t, text, "recent item")
	assert.NotContains(t, text, "old item")
}

// TestHandleGetV2_DomainBacklog_SinceInvalid_Errors mirrors TestHandleGet_DomainBacklog_SinceInvalid_Errors.
func TestHandleGetV2_DomainBacklog_SinceInvalid_Errors(t *testing.T) {
	resetAllDB(t)
	res := callGetV2(t, getInput{Domain: "backlog", Since: "not-a-date"})
	require.True(t, res.IsError)
}

// TestHandleGetV2_DomainBacklog_SortCreated mirrors TestHandleGet_DomainBacklog_SortCreated.
func TestHandleGetV2_DomainBacklog_SortCreated(t *testing.T) {
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

	res := callGetV2(t, getInput{Domain: "backlog", Projects: []string{"acme-corp"}, Sort: "created"})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	newerIdx := strings.Index(text, "newer")
	olderIdx := strings.Index(text, "older")
	require.NotEqual(t, -1, newerIdx)
	require.NotEqual(t, -1, olderIdx)
	assert.Less(t, newerIdx, olderIdx)
}

// TestHandleGetV2_DomainBacklog_SortInvalid_Errors mirrors TestHandleGet_DomainBacklog_SortInvalid_Errors.
func TestHandleGetV2_DomainBacklog_SortInvalid_Errors(t *testing.T) {
	resetAllDB(t)
	res := callGetV2(t, getInput{Domain: "backlog", Sort: "bogus"})
	require.True(t, res.IsError)
}

// TestHandleGetV2_DomainBacklog_NonexistentProject_Errors mirrors
// TestHandleGet_DomainBacklog_NonexistentProject_Errors.
func TestHandleGetV2_DomainBacklog_NonexistentProject_Errors(t *testing.T) {
	resetAllDB(t)
	res := callGetV2(t, getInput{Domain: "backlog", Projects: []string{"nonexistent"}})
	require.True(t, res.IsError)
}

// TestHandleGetV2_DomainBacklog_IdsFetchMiss mirrors TestHandleGet_DomainBacklog_IdsFetchMiss:
// unlike kb, a backlog ids[] miss errors the whole call.
func TestHandleGetV2_DomainBacklog_IdsFetchMiss(t *testing.T) {
	resetAllDB(t)
	res := callGetV2(t, getInput{Domain: "backlog", IDs: []any{"AC-9999"}})
	require.True(t, res.IsError)
}

// TestHandleGetV2_DomainRoadmap_LoadError mirrors TestHandleGet_DomainRoadmap_LoadError:
// corrupt roadmap metadata surfaces roadmap.Load's error as a tool error.
func TestHandleGetV2_DomainRoadmap_LoadError(t *testing.T) {
	resetDB(t)
	_, err := store.UpsertDocument(db, store.Document{
		Type: "roadmap", Project: "acme-corp", Title: "roadmap", Content: "# Roadmap",
		Metadata: map[string]string{"data": "not json"},
	})
	require.NoError(t, err)

	res := callGetV2(t, getInput{Domain: "roadmap", Projects: []string{"acme-corp"}})
	require.True(t, res.IsError)
}
