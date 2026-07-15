package query

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/store"
	"dangernoodle.io/ouroboros/internal/testutil"
)

// dropTable returns a fresh schema-applied db with the named table dropped,
// mirroring internal/app's brokenFTSDB approach for exercising a downstream
// store/backlog error path from this package's own tests (rather than
// relying on cross-package coverage attribution from the MCP handler
// tests, which already exercise these paths but don't count toward this
// package's own coverage number).
func dropTable(t *testing.T, table string) *sql.DB {
	t.Helper()
	db := testutil.TestDB(t)
	_, err := db.Exec("DROP TABLE " + table)
	require.NoError(t, err)
	return db
}

func TestGet_DomainKB_IDsFetch_StoreError(t *testing.T) {
	db := dropTable(t, "documents")

	_, err := Get(db, Request{Domain: "kb", IDs: []any{float64(1)}})
	require.Error(t, err)
}

func TestGet_DomainKB_FilterList_StoreError(t *testing.T) {
	db := dropTable(t, "documents")

	_, err := Get(db, Request{Domain: "kb"})
	require.Error(t, err)
}

func TestGet_DomainKB_IDsFetchVerbose_EdgesError(t *testing.T) {
	db := dropTable(t, "edges")
	doc, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "d1", Content: "c1"})
	require.NoError(t, err)

	_, err = Get(db, Request{Domain: "kb", IDs: []any{float64(doc.ID)}, Verbose: true})
	require.Error(t, err)
}

func TestGet_DomainBacklog_IDsFetch_StoreError(t *testing.T) {
	db := dropTable(t, "items")

	_, err := Get(db, Request{Domain: "backlog", IDs: []any{"AC-1"}})
	require.Error(t, err)
}

func TestGet_DomainBacklog_FilterList_StoreError(t *testing.T) {
	db := dropTable(t, "items")

	_, err := Get(db, Request{Domain: "backlog"})
	require.Error(t, err)
}

func TestGet_DomainBacklog_IDsFetchVerbose_EdgesError(t *testing.T) {
	db := dropTable(t, "edges")
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item", "", "", "", "")
	require.NoError(t, err)

	_, err = Get(db, Request{Domain: "backlog", IDs: []any{item.ID}, Verbose: true})
	require.Error(t, err)
}

func TestGet_DomainRoadmap_LoadError(t *testing.T) {
	db := testutil.TestDB(t)
	_, err := store.UpsertDocument(db, store.Document{
		Type: "roadmap", Project: "acme-corp", Title: "roadmap", Content: "# Roadmap",
		Metadata: map[string]string{"data": "not json"},
	})
	require.NoError(t, err)

	_, err = Get(db, Request{Domain: "roadmap", Projects: []string{"acme-corp"}})
	require.Error(t, err)
}

func TestSearch_DomainKB_SingleQuery_StoreError(t *testing.T) {
	db := dropTable(t, "documents_fts")

	_, err := Search(db, Request{Domain: "kb", Query: "widget"})
	require.Error(t, err)
}

func TestSearch_DomainKB_QueriesBatch_StoreError(t *testing.T) {
	db := dropTable(t, "documents_fts")

	_, err := Search(db, Request{Domain: "kb", Queries: []string{"widget"}})
	require.Error(t, err)
}

func TestSearch_DomainBacklog_StoreError(t *testing.T) {
	db := dropTable(t, "items")

	_, err := Search(db, Request{Domain: "backlog", Query: "widget"})
	require.Error(t, err)
}

func TestSearch_DomainRoadmap_StoreError(t *testing.T) {
	db := dropTable(t, "documents_fts")

	_, err := Search(db, Request{Domain: "roadmap", Query: "widget"})
	require.Error(t, err)
}
