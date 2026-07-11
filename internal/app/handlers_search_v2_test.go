package app

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/dangernoodle-io/mcpkit/mcpx"
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

func callSearchV2(t *testing.T, in searchInput) *mcpx.CallToolResult {
	t.Helper()
	res, out, err := handleSearchV2(&serverState{db: db})(context.TODO(), &mcpx.CallToolRequest{}, in)
	require.NoError(t, err)
	require.Nil(t, out)
	return res
}

// TestHandleSearchV2_MissingDomain_Errors mirrors TestHandleSearch_MissingDomain_Errors.
func TestHandleSearchV2_MissingDomain_Errors(t *testing.T) {
	resetDB(t)
	res := callSearchV2(t, searchInput{})
	require.True(t, res.IsError)
	assert.Equal(t, `domain is required: must be "kb", "backlog", or "roadmap"`, mcpx.ResultText(res))
}

// TestHandleSearchV2_DomainKB_SingleQuery mirrors TestHandleSearch_SingleQuery_BackwardsCompat.
func TestHandleSearchV2_DomainKB_SingleQuery(t *testing.T) {
	resetDB(t)
	_, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "ouroboros", Title: "tiktoken", Content: "Use tiktoken for token counting"})
	require.NoError(t, err)

	res := callSearchV2(t, searchInput{Domain: "kb", Query: "tiktoken"})
	require.False(t, res.IsError)

	var summaries []map[string]any
	require.NoError(t, jsonUnmarshalText(res, &summaries))
	require.Len(t, summaries, 1)
	assert.Equal(t, "tiktoken", summaries[0]["title"])
}

// TestHandleSearchV2_DomainKB_QueriesBatch mirrors TestHandleSearch_Batch_PositionalResults.
func TestHandleSearchV2_DomainKB_QueriesBatch(t *testing.T) {
	resetDB(t)
	_, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "alpha widget", Content: "c"})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "beta gadget", Content: "c"})
	require.NoError(t, err)

	res := callSearchV2(t, searchInput{Domain: "kb", Queries: []string{"widget", "gadget", "nonexistentterm"}})
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

// TestHandleSearchV2_DomainKB_NeitherQueryNorQueries_Errors mirrors
// TestHandleSearch_NeitherQueryNorQueries_Errors.
func TestHandleSearchV2_DomainKB_NeitherQueryNorQueries_Errors(t *testing.T) {
	resetDB(t)
	res := callSearchV2(t, searchInput{Domain: "kb"})
	require.True(t, res.IsError)
	assert.Equal(t, "query or queries is required", mcpx.ResultText(res))
}

// TestHandleSearchV2_DomainBacklog_ReturnsMatches mirrors TestHandleSearch_DomainBacklog_ReturnsMatches.
func TestHandleSearchV2_DomainBacklog_ReturnsMatches(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "fix the flux capacitor", "desc", "", "", "")
	require.NoError(t, err)

	res := callSearchV2(t, searchInput{Domain: "backlog", Query: "flux"})
	require.False(t, res.IsError)
	assert.Contains(t, mcpx.ResultText(res), "flux capacitor")
}

// TestHandleSearchV2_DomainBacklog_MissingQuery_Errors mirrors
// TestHandleSearch_DomainBacklog_MissingQuery_Errors.
func TestHandleSearchV2_DomainBacklog_MissingQuery_Errors(t *testing.T) {
	resetAllDB(t)
	res := callSearchV2(t, searchInput{Domain: "backlog"})
	require.True(t, res.IsError)
	assert.Equal(t, "query is required", mcpx.ResultText(res))
}

// TestHandleSearchV2_DomainRoadmap mirrors TestHandleSearch_DomainRoadmap.
func TestHandleSearchV2_DomainRoadmap(t *testing.T) {
	resetDB(t)
	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "Unique searchable widget"})
		return err
	}))

	res := callSearchV2(t, searchInput{Domain: "roadmap", Query: "widget"})
	require.False(t, res.IsError)

	var resp []map[string]any
	require.NoError(t, jsonUnmarshalText(res, &resp))
	require.Len(t, resp, 1)
	assert.Equal(t, "roadmap", resp[0]["type"])
}

// TestHandleSearchV2_DomainRoadmap_MissingQuery_Errors mirrors
// TestHandleSearch_DomainRoadmap_MissingQuery_Errors.
func TestHandleSearchV2_DomainRoadmap_MissingQuery_Errors(t *testing.T) {
	resetDB(t)
	res := callSearchV2(t, searchInput{Domain: "roadmap"})
	require.True(t, res.IsError)
	assert.Equal(t, "query is required", mcpx.ResultText(res))
}

// TestHandleSearchV2_DomainBacklog_EpicFilter mirrors TestHandleSearch_DomainBacklog_EpicFilter.
func TestHandleSearchV2_DomainBacklog_EpicFilter(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	epic, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "EPIC: widget", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "flux child item", "", "", "", epic.ID)
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "flux unrelated item", "", "", "", "")
	require.NoError(t, err)

	res := callSearchV2(t, searchInput{Domain: "backlog", Query: "flux", Epic: epic.ID})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	assert.Contains(t, text, "child item")
	assert.NotContains(t, text, "unrelated item")
}

// TestHandleSearchV2_DomainBacklog_SinceFilter mirrors TestHandleSearch_DomainBacklog_SinceFilter.
func TestHandleSearchV2_DomainBacklog_SinceFilter(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	old, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "flux old item", "", "", "", "")
	require.NoError(t, err)
	_, err = db.Exec("UPDATE items SET created = ? WHERE id = ?", "2020-01-01T00:00:00Z", old.ID)
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "flux recent item", "", "", "", "")
	require.NoError(t, err)

	res := callSearchV2(t, searchInput{Domain: "backlog", Query: "flux", Since: "24h"})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	assert.Contains(t, text, "recent item")
	assert.NotContains(t, text, "old item")
}

// TestHandleSearchV2_DomainBacklog_SortCreated mirrors TestHandleSearch_DomainBacklog_SortCreated.
func TestHandleSearchV2_DomainBacklog_SortCreated(t *testing.T) {
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

	res := callSearchV2(t, searchInput{Domain: "backlog", Query: "flux", Sort: "created"})
	require.False(t, res.IsError)
	text := mcpx.ResultText(res)
	newerIdx := strings.Index(text, "newer")
	olderIdx := strings.Index(text, "older")
	require.NotEqual(t, -1, newerIdx)
	require.NotEqual(t, -1, olderIdx)
	assert.Less(t, newerIdx, olderIdx)
}

// TestHandleSearchV2_DomainBacklog_SortInvalid_Errors mirrors TestHandleSearch_DomainBacklog_SortInvalid_Errors.
func TestHandleSearchV2_DomainBacklog_SortInvalid_Errors(t *testing.T) {
	resetAllDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "flux item", "", "", "", "")
	require.NoError(t, err)

	res := callSearchV2(t, searchInput{Domain: "backlog", Query: "flux", Sort: "bogus"})
	require.True(t, res.IsError)
}

// TestHandleSearchV2_DomainKB_WithLimit mirrors TestHandleSearch_SingleQuery_WithLimit.
func TestHandleSearchV2_DomainKB_WithLimit(t *testing.T) {
	resetDB(t)
	_, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "limited widget", Content: "c"})
	require.NoError(t, err)

	res := callSearchV2(t, searchInput{Domain: "kb", Query: "widget", Limit: 5})
	require.False(t, res.IsError)

	var summaries []map[string]any
	require.NoError(t, jsonUnmarshalText(res, &summaries))
	require.Len(t, summaries, 1)
}

// TestHandleSearchV2_DomainKB_SingleQueryError mirrors the store.SearchDocuments
// error path a broken FTS index triggers, for the single-query branch.
func TestHandleSearchV2_DomainKB_SingleQueryError(t *testing.T) {
	bdb := brokenFTSDB(t)
	res, out, err := handleSearchV2(&serverState{db: bdb})(context.TODO(), &mcpx.CallToolRequest{}, searchInput{Domain: "kb", Query: "widget"})
	require.NoError(t, err)
	require.Nil(t, out)
	assert.True(t, res.IsError)
}

// TestHandleSearchV2_DomainKB_QueriesBatchError mirrors the same error path
// for the queries[] batch branch.
func TestHandleSearchV2_DomainKB_QueriesBatchError(t *testing.T) {
	bdb := brokenFTSDB(t)
	res, out, err := handleSearchV2(&serverState{db: bdb})(context.TODO(), &mcpx.CallToolRequest{}, searchInput{Domain: "kb", Queries: []string{"widget"}})
	require.NoError(t, err)
	require.Nil(t, out)
	assert.True(t, res.IsError)
}

// TestHandleSearchV2_DomainRoadmap_Error mirrors TestHandleSearch_DomainRoadmap_Error.
func TestHandleSearchV2_DomainRoadmap_Error(t *testing.T) {
	bdb := brokenFTSDB(t)
	res, out, err := handleSearchV2(&serverState{db: bdb})(context.TODO(), &mcpx.CallToolRequest{}, searchInput{Domain: "roadmap", Query: "widget"})
	require.NoError(t, err)
	require.Nil(t, out)
	assert.True(t, res.IsError)
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
