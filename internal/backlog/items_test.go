package backlog_test

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/edges"
	"dangernoodle.io/ouroboros/internal/store"
)

func createTestProject(t *testing.T, d *sql.DB) *backlog.Project {
	t.Helper()
	p, err := backlog.CreateProject(d, "acme-corp", "AC")
	require.NoError(t, err)
	return p
}

func TestAddItem(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	item, err := backlog.AddItem(d, p.ID, "AC", "P1", "test-item", "test description", "", "", "")
	require.NoError(t, err)

	assert.Equal(t, "AC-1", item.ID)
	assert.Equal(t, p.ID, item.ProjectID)
	assert.Equal(t, "P1", item.Priority)
	assert.Equal(t, "test-item", item.Title)
	assert.Equal(t, "test description", item.Description)
	assert.Equal(t, "open", item.Status)
	assert.NotEmpty(t, item.Created)
	assert.NotEmpty(t, item.Updated)
}

func TestAddItemWithEpic(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	item, err := backlog.AddItem(d, p.ID, "AC", "P1", "child item", "", "", "widget", "AC-1")
	require.NoError(t, err)
	assert.Equal(t, "AC-1", item.Epic)

	fetched, err := backlog.GetItem(d, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "AC-1", fetched.Epic)
}

func TestUpdateItemEpic(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	item, err := backlog.AddItem(d, p.ID, "AC", "P1", "item", "", "", "", "")
	require.NoError(t, err)
	assert.Empty(t, item.Epic)

	updated, err := backlog.UpdateItem(d, item.ID, map[string]string{"epic": "AC-9"})
	require.NoError(t, err)
	assert.Equal(t, "AC-9", updated.Epic)
}

func TestListItemsSurfacesEpic(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "child item", "", "", "", "AC-1")
	require.NoError(t, err)

	items, err := backlog.ListItems(d, backlog.ItemFilter{ProjectIDs: []int64{p.ID}})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "AC-1", items[0].Epic)
}

func TestSearchItemsSurfacesEpic(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "findableepictitle", "", "", "", "AC-1")
	require.NoError(t, err)

	items, err := backlog.SearchItems(d, "findableepictitle", backlog.ItemFilter{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "AC-1", items[0].Epic)
}

func TestGetItemsByIDsBatchesInOneQuery(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	item1, err := backlog.AddItem(d, p.ID, "AC", "P1", "EPIC: one", "", "", "", "")
	require.NoError(t, err)
	item2, err := backlog.AddItem(d, p.ID, "AC", "P1", "EPIC: two", "", "", "", "")
	require.NoError(t, err)

	items, err := backlog.GetItemsByIDs(d, []string{item1.ID, item2.ID, "AC-999"})
	require.NoError(t, err)
	require.Len(t, items, 2, "a missing id is simply omitted, not an error")

	byID := map[string]backlog.Item{}
	for _, it := range items {
		byID[it.ID] = it
	}
	assert.Equal(t, "EPIC: one", byID[item1.ID].Title)
	assert.Equal(t, "EPIC: two", byID[item2.ID].Title)
}

func TestGetItemsByIDsEmpty(t *testing.T) {
	d := testDB(t)
	items, err := backlog.GetItemsByIDs(d, nil)
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestEpicLabelsStripsPrefixAndFallsBackOnMiss(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	item, err := backlog.AddItem(d, p.ID, "AC", "P1", "EPIC: WiFi map", "", "", "", "")
	require.NoError(t, err)

	labels := backlog.EpicLabels(d, []string{item.ID, "AC-999"})
	assert.Equal(t, "WiFi map", labels[item.ID])
	_, missing := labels["AC-999"]
	assert.False(t, missing, "an id with no matching item is simply omitted")
}

func TestEpicLabelsEmpty(t *testing.T) {
	d := testDB(t)
	assert.Empty(t, backlog.EpicLabels(d, nil))
}

// TestEpicLabelsResolvesRenamedEpicViaAliasFallback verifies the rare
// renamed-epic path: an id missed by the batched query still resolves via
// the per-id GetItem alias fallback.
func TestEpicLabelsResolvesRenamedEpicViaAliasFallback(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	item, err := backlog.AddItem(d, p.ID, "AC", "P1", "EPIC: renamed", "", "", "", "")
	require.NoError(t, err)

	_, err = d.Exec("INSERT INTO item_id_aliases (old_id, new_id, renamed_at) VALUES (?, ?, ?)",
		"OLD-1", item.ID, "2024-01-01T00:00:00Z")
	require.NoError(t, err)

	labels := backlog.EpicLabels(d, []string{"OLD-1"})
	assert.Equal(t, "renamed", labels["OLD-1"], "keyed by the requested (old) id, not the resolved current id")
}

// TestBlockedEpicsForDetectsIncomingBlocksEdge verifies the epic-axis
// blocked-header lookup: an epic id targeted by an incoming "blocks" edge is
// mapped true, an unblocked epic id is simply absent from the result.
func TestBlockedEpicsForDetectsIncomingBlocksEdge(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	epicItem, err := backlog.AddItem(d, p.ID, "AC", "P1", "EPIC: WiFi map", "", "", "", "")
	require.NoError(t, err)
	unblockedEpic, err := backlog.AddItem(d, p.ID, "AC", "P1", "EPIC: unblocked", "", "", "", "")
	require.NoError(t, err)
	blocker, err := backlog.AddItem(d, p.ID, "AC", "P1", "blocking item", "", "", "", "")
	require.NoError(t, err)

	_, err = edges.Link(d, edges.TypeItem, blocker.ID, "blocks", edges.TypeItem, epicItem.ID, p.ID)
	require.NoError(t, err)

	blocked := backlog.BlockedEpicsFor(d, []string{epicItem.ID, unblockedEpic.ID})
	assert.True(t, blocked[epicItem.ID])
	assert.False(t, blocked[unblockedEpic.ID])
}

// TestBlockedEpicsForEmpty covers the no-ids input.
func TestBlockedEpicsForEmpty(t *testing.T) {
	d := testDB(t)
	assert.Empty(t, backlog.BlockedEpicsFor(d, nil))
}

// TestGetItemsBatchesInOneQueryPreservingOrder covers backlog.GetItems: a
// batched fetch of multiple valid ids returns them in the REQUESTED order
// (not insertion/DB order), including notes (needed by verbose get).
func TestGetItemsBatchesInOneQueryPreservingOrder(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	item1, err := backlog.AddItem(d, p.ID, "AC", "P1", "first", "", "secret notes", "", "")
	require.NoError(t, err)
	item2, err := backlog.AddItem(d, p.ID, "AC", "P1", "second", "", "", "", "")
	require.NoError(t, err)

	// Request in reverse-insertion order to prove GetItems re-sorts to
	// match the request, not DB/insertion order.
	items, err := backlog.GetItems(d, []string{item2.ID, item1.ID})
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, item2.ID, items[0].ID, "result order follows request order")
	assert.Equal(t, item1.ID, items[1].ID)
	assert.Equal(t, "secret notes", items[1].Notes, "notes are populated (needed by verbose fetch)")
}

// TestGetItemsMissingIDErrors confirms a nonexistent id (even after alias
// resolution) errors out the whole call, matching the per-id GetItem loop
// this replaces — a miss is NOT silently omitted for backlog gets (unlike
// kb's GetDocuments/GetDocument "nil, nil on miss" contract).
func TestGetItemsMissingIDErrors(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	item, err := backlog.AddItem(d, p.ID, "AC", "P1", "exists", "", "", "", "")
	require.NoError(t, err)

	_, err = backlog.GetItems(d, []string{item.ID, "AC-999"})
	assert.Error(t, err)
}

// TestGetItemsResolvesRenamedIDViaAliasFallback verifies the rare
// renamed-item path: an id missed by the batched IN-query still resolves
// via the per-id GetItem alias fallback.
func TestGetItemsResolvesRenamedIDViaAliasFallback(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	item, err := backlog.AddItem(d, p.ID, "AC", "P1", "renamed", "", "", "", "")
	require.NoError(t, err)

	_, err = d.Exec("INSERT INTO item_id_aliases (old_id, new_id, renamed_at) VALUES (?, ?, ?)",
		"OLD-1", item.ID, "2024-01-01T00:00:00Z")
	require.NoError(t, err)

	items, err := backlog.GetItems(d, []string{"OLD-1"})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, item.ID, items[0].ID)
}

// TestGetItemsEmpty confirms an empty/nil ids slice returns an empty result
// without querying.
func TestGetItemsEmpty(t *testing.T) {
	d := testDB(t)
	items, err := backlog.GetItems(d, nil)
	require.NoError(t, err)
	assert.Empty(t, items)
}

// TestGetItemsQueryError exercises GetItems' propagation of the batched
// query's error (closed DB) — a hard failure on the primary fetch, distinct
// from the per-id miss path (TestGetItemsMissingIDErrors) which falls back to
// GetItem before erroring.
func TestGetItemsQueryError(t *testing.T) {
	d, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	require.NoError(t, store.ApplySchema(d))
	require.NoError(t, d.Close())

	_, err = backlog.GetItems(d, []string{"AC-1"})
	assert.Error(t, err)
}

// TestAddItemBeginError exercises AddItem's d.Begin() error path (closed DB).
func TestAddItemBeginError(t *testing.T) {
	d := testDB(t)
	require.NoError(t, d.Close())

	_, err := backlog.AddItem(d, 1, "AC", "P1", "x", "", "", "", "")
	assert.Error(t, err)
}

func TestAddItemSequence(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	item1, err := backlog.AddItem(d, p.ID, "AC", "P1", "item1", "", "", "", "")
	require.NoError(t, err)
	assert.Equal(t, "AC-1", item1.ID)

	item2, err := backlog.AddItem(d, p.ID, "AC", "P2", "item2", "", "", "", "")
	require.NoError(t, err)
	assert.Equal(t, "AC-2", item2.ID)

	item3, err := backlog.AddItem(d, p.ID, "AC", "P3", "item3", "", "", "", "")
	require.NoError(t, err)
	assert.Equal(t, "AC-3", item3.ID)
}

func TestGetItem(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	created, err := backlog.AddItem(d, p.ID, "AC", "P1", "test-item", "desc", "", "", "")
	require.NoError(t, err)

	item, err := backlog.GetItem(d, "AC-1")
	require.NoError(t, err)

	assert.Equal(t, created.ID, item.ID)
	assert.Equal(t, "test-item", item.Title)
	assert.Equal(t, "desc", item.Description)
}

func TestGetItemNotFound(t *testing.T) {
	d := testDB(t)

	_, err := backlog.GetItem(d, "NONEXISTENT")
	assert.Error(t, err)
}

func TestUpdateItem(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "old-title", "old-desc", "", "", "")
	require.NoError(t, err)

	updated, err := backlog.UpdateItem(d, "AC-1", map[string]string{
		"title":    "new-title",
		"priority": "P3",
	})
	require.NoError(t, err)

	assert.Equal(t, "new-title", updated.Title)
	assert.Equal(t, "P3", updated.Priority)
	assert.Equal(t, "old-desc", updated.Description)
}

func TestMarkDone(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item", "", "", "", "")
	require.NoError(t, err)

	err = backlog.MarkDone(d, "AC-1")
	require.NoError(t, err)

	item, err := backlog.GetItem(d, "AC-1")
	require.NoError(t, err)
	assert.Equal(t, "done", item.Status)
}

func TestMarkDoneNotFound(t *testing.T) {
	d := testDB(t)

	err := backlog.MarkDone(d, "NONEXISTENT")
	assert.Error(t, err)
}

func TestListItems(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item1", "", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "item2", "", "", "", "")
	require.NoError(t, err)

	items, err := backlog.ListItems(d, backlog.ItemFilter{})
	require.NoError(t, err)

	assert.Len(t, items, 2)
}

func TestListItemsWithLimit(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	for i := 1; i <= 5; i++ {
		_, err := backlog.AddItem(d, p.ID, "AC", "P3", fmt.Sprintf("item%d", i), "", "", "", "")
		require.NoError(t, err)
	}

	items, err := backlog.ListItems(d, backlog.ItemFilter{Limit: 2})
	require.NoError(t, err)

	assert.Len(t, items, 2)
}

// setItemCreated backdoors an item's created timestamp for deterministic
// created-time filter/sort tests (mirrors the raw-SQL pattern used elsewhere
// in this file for item_id_aliases setup).
func setItemCreated(t *testing.T, d *sql.DB, id, created string) {
	t.Helper()
	_, err := d.Exec("UPDATE items SET created = ? WHERE id = ?", created, id)
	require.NoError(t, err)
}

// TestListItemsFilterCreatedSince verifies ItemFilter.CreatedSince returns
// only items created at/after the cutoff.
func TestListItemsFilterCreatedSince(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	old, err := backlog.AddItem(d, p.ID, "AC", "P1", "old item", "", "", "", "")
	require.NoError(t, err)
	setItemCreated(t, d, old.ID, "2020-01-01T00:00:00Z")

	recent, err := backlog.AddItem(d, p.ID, "AC", "P1", "recent item", "", "", "", "")
	require.NoError(t, err)
	setItemCreated(t, d, recent.ID, "2026-01-01T00:00:00Z")

	cutoff := "2025-01-01T00:00:00Z"
	items, err := backlog.ListItems(d, backlog.ItemFilter{CreatedSince: &cutoff})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "recent item", items[0].Title)
}

// TestListItemsSortByCreated verifies SortByCreated orders newest-first,
// overriding the default priority ordering.
func TestListItemsSortByCreated(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	first, err := backlog.AddItem(d, p.ID, "AC", "P0", "first", "", "", "", "")
	require.NoError(t, err)
	setItemCreated(t, d, first.ID, "2024-01-01T00:00:00Z")

	second, err := backlog.AddItem(d, p.ID, "AC", "P6", "second", "", "", "", "")
	require.NoError(t, err)
	setItemCreated(t, d, second.ID, "2025-01-01T00:00:00Z")

	third, err := backlog.AddItem(d, p.ID, "AC", "P3", "third", "", "", "", "")
	require.NoError(t, err)
	setItemCreated(t, d, third.ID, "2026-01-01T00:00:00Z")

	items, err := backlog.ListItems(d, backlog.ItemFilter{SortByCreated: true})
	require.NoError(t, err)
	require.Len(t, items, 3)
	assert.Equal(t, "third", items[0].Title)
	assert.Equal(t, "second", items[1].Title)
	assert.Equal(t, "first", items[2].Title)
}

func TestListItemsFilterProject(t *testing.T) {
	d := testDB(t)
	p1 := createTestProject(t, d)
	p2, err := backlog.CreateProject(d, "other-corp", "OC")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p1.ID, "AC", "P1", "item1", "", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p2.ID, "OC", "P1", "item2", "", "", "", "")
	require.NoError(t, err)

	items, err := backlog.ListItems(d, backlog.ItemFilter{ProjectIDs: []int64{p1.ID}})
	require.NoError(t, err)

	assert.Len(t, items, 1)
	assert.Equal(t, "AC-1", items[0].ID)
}

func TestListItemsFilterMultiProject(t *testing.T) {
	d := testDB(t)
	p1 := createTestProject(t, d)
	p2, err := backlog.CreateProject(d, "other-corp", "OC")
	require.NoError(t, err)
	p3, err := backlog.CreateProject(d, "third-corp", "TC")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p1.ID, "AC", "P1", "item1", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(d, p2.ID, "OC", "P1", "item2", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(d, p3.ID, "TC", "P1", "item3", "", "", "", "")
	require.NoError(t, err)

	items, err := backlog.ListItems(d, backlog.ItemFilter{ProjectIDs: []int64{p1.ID, p2.ID}})
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

func TestDeleteItemsSingle(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item1", "", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "item2", "", "", "", "")
	require.NoError(t, err)

	affected, err := backlog.DeleteItems(d, []string{"AC-1"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), affected)

	_, err = backlog.GetItem(d, "AC-1")
	assert.Error(t, err)

	item, err := backlog.GetItem(d, "AC-2")
	require.NoError(t, err)
	assert.Equal(t, "AC-2", item.ID)
}

func TestDeleteItemsMultiple(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item1", "", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "item2", "", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P3", "item3", "", "", "", "")
	require.NoError(t, err)

	affected, err := backlog.DeleteItems(d, []string{"AC-1", "AC-2"})
	require.NoError(t, err)
	assert.Equal(t, int64(2), affected)

	item, err := backlog.GetItem(d, "AC-3")
	require.NoError(t, err)
	assert.Equal(t, "AC-3", item.ID)
}

func TestDeleteItemsNotFound(t *testing.T) {
	d := testDB(t)

	affected, err := backlog.DeleteItems(d, []string{"NONEXISTENT"})
	require.NoError(t, err)
	assert.Equal(t, int64(0), affected)
}

func TestDeleteItemsMixed(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item1", "", "", "", "")
	require.NoError(t, err)

	affected, err := backlog.DeleteItems(d, []string{"AC-1", "NONEXISTENT"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), affected)
}

// TestDeleteItemsBeginError exercises DeleteItems' d.Begin() error path
// (closed DB).
func TestDeleteItemsBeginError(t *testing.T) {
	d := testDB(t)
	require.NoError(t, d.Close())

	_, err := backlog.DeleteItems(d, []string{"AC-1"})
	assert.Error(t, err)
}

func TestDeleteItemsEmpty(t *testing.T) {
	d := testDB(t)

	affected, err := backlog.DeleteItems(d, []string{})
	require.NoError(t, err)
	assert.Equal(t, int64(0), affected)
}

func TestListItemsFilterPriority(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item1", "", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "item2", "", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P3", "item3", "", "", "", "")
	require.NoError(t, err)

	minPriority := 2
	maxPriority := 2
	items, err := backlog.ListItems(d, backlog.ItemFilter{PriorityMin: &minPriority, PriorityMax: &maxPriority})
	require.NoError(t, err)

	assert.Len(t, items, 1)
	assert.Equal(t, "P2", items[0].Priority)
}

func TestListItemsFilterStatus(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item1", "", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "item2", "", "", "", "")
	require.NoError(t, err)

	err = backlog.MarkDone(d, "AC-1")
	require.NoError(t, err)

	status := "open"
	items, err := backlog.ListItems(d, backlog.ItemFilter{Status: &status})
	require.NoError(t, err)

	assert.Len(t, items, 1)
	assert.Equal(t, "AC-2", items[0].ID)
}

func TestAddItemWithComponent(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	item, err := backlog.AddItem(d, p.ID, "AC", "P1", "test-item", "desc", "", "ouroboros-mcp", "")
	require.NoError(t, err)

	assert.Equal(t, "ouroboros-mcp", item.Component)

	// Verify round-trip via GetItem
	fetched, err := backlog.GetItem(d, "AC-1")
	require.NoError(t, err)
	assert.Equal(t, "ouroboros-mcp", fetched.Component)
}

func TestListItemsFilterComponent(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	// Seed 3 items across 2 components
	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item1", "", "", "plugin-a", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "item2", "", "", "plugin-b", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P3", "item3", "", "", "plugin-a", "")
	require.NoError(t, err)

	// Filter to plugin-a
	component := "plugin-a"
	items, err := backlog.ListItems(d, backlog.ItemFilter{Component: &component})
	require.NoError(t, err)

	assert.Len(t, items, 2)
	assert.Equal(t, "AC-1", items[0].ID)
	assert.Equal(t, "AC-3", items[1].ID)
}

// TestListItemsFilterEpic verifies ItemFilter.Epic returns only that
// epic's children, leaving unrelated items and other epics' children out.
func TestListItemsFilterEpic(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	epic, err := backlog.AddItem(d, p.ID, "AC", "P1", "EPIC: demo", "", "", "", "")
	require.NoError(t, err)
	otherEpic, err := backlog.AddItem(d, p.ID, "AC", "P1", "EPIC: other", "", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "child of demo", "", "", "", epic.ID)
	require.NoError(t, err)
	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "child of other", "", "", "", otherEpic.ID)
	require.NoError(t, err)
	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "no epic", "", "", "", "")
	require.NoError(t, err)

	items, err := backlog.ListItems(d, backlog.ItemFilter{Epic: &epic.ID})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "child of demo", items[0].Title)
}

// TestListItemsFilterEpicEmptyUnaffected verifies an unset (nil) Epic filter
// leaves ListItems' result set unaffected.
func TestListItemsFilterEpicEmptyUnaffected(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item1", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "item2", "", "", "", "AC-1")
	require.NoError(t, err)

	items, err := backlog.ListItems(d, backlog.ItemFilter{})
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

// TestListItemsFilterEpicsOnly verifies ItemFilter.EpicsOnly returns only
// EPIC:-titled items.
func TestListItemsFilterEpicsOnly(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "EPIC: demo", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(d, p.ID, "AC", "P1", "EPIC: other", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "regular item", "", "", "", "")
	require.NoError(t, err)

	items, err := backlog.ListItems(d, backlog.ItemFilter{EpicsOnly: true})
	require.NoError(t, err)
	require.Len(t, items, 2)
	for _, item := range items {
		assert.Contains(t, item.Title, "EPIC:")
	}
}

// TestListItemsFilterEpicsOnlyTakesPrecedence verifies EpicsOnly wins when
// both Epic and EpicsOnly are set (they can't sensibly combine).
func TestListItemsFilterEpicsOnlyTakesPrecedence(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	epic, err := backlog.AddItem(d, p.ID, "AC", "P1", "EPIC: demo", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "child", "", "", "", epic.ID)
	require.NoError(t, err)

	items, err := backlog.ListItems(d, backlog.ItemFilter{Epic: &epic.ID, EpicsOnly: true})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "EPIC: demo", items[0].Title)
}

func TestUpdateItemComponent(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item", "", "", "plugin-a", "")
	require.NoError(t, err)

	updated, err := backlog.UpdateItem(d, "AC-1", map[string]string{
		"component": "plugin-b",
	})
	require.NoError(t, err)

	assert.Equal(t, "plugin-b", updated.Component)
}

func TestCountItemsByPriority(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P0", "item1", "", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P1", "item2", "", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P1", "item3", "", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "item4", "", "", "", "")
	require.NoError(t, err)

	status := "open"
	counts, err := backlog.CountItemsByPriority(d, backlog.ItemFilter{Status: &status})
	require.NoError(t, err)

	assert.Len(t, counts, 3)
	assert.Equal(t, "P0", counts[0].Priority)
	assert.Equal(t, 1, counts[0].Count)
	assert.Equal(t, "P1", counts[1].Priority)
	assert.Equal(t, 2, counts[1].Count)
	assert.Equal(t, "P2", counts[2].Priority)
	assert.Equal(t, 1, counts[2].Count)
}

func TestCountItemsByPriorityFiltered(t *testing.T) {
	d := testDB(t)
	p1 := createTestProject(t, d)
	p2, err := backlog.CreateProject(d, "test-project", "TP")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p1.ID, "AC", "P0", "item1", "", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p1.ID, "AC", "P1", "item2", "", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p2.ID, "TP", "P0", "item3", "", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p2.ID, "TP", "P0", "item4", "", "", "", "")
	require.NoError(t, err)

	counts, err := backlog.CountItemsByPriority(d, backlog.ItemFilter{ProjectIDs: []int64{p1.ID}})
	require.NoError(t, err)

	assert.Len(t, counts, 2)
	assert.Equal(t, "P0", counts[0].Priority)
	assert.Equal(t, 1, counts[0].Count)
	assert.Equal(t, "P1", counts[1].Priority)
	assert.Equal(t, 1, counts[1].Count)
}

func TestCountItemsByPriorityEmpty(t *testing.T) {
	d := testDB(t)
	_ = createTestProject(t, d)

	status := "open"
	counts, err := backlog.CountItemsByPriority(d, backlog.ItemFilter{Status: &status})
	require.NoError(t, err)

	assert.Empty(t, counts)
}

func TestGetItemViaAlias(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	item, err := backlog.AddItem(d, p.ID, "AC", "P1", "task", "", "", "", "")
	require.NoError(t, err)

	// Manually insert alias row
	_, err = d.Exec("INSERT INTO item_id_aliases (old_id, new_id, renamed_at) VALUES (?, ?, ?)",
		"OLD-1", item.ID, "2024-01-01T00:00:00Z")
	require.NoError(t, err)

	got, err := backlog.GetItem(d, "OLD-1")
	require.NoError(t, err)
	assert.Equal(t, item.ID, got.ID)
	assert.Equal(t, "task", got.Title)
}

func TestGetItemNotFoundNoAlias(t *testing.T) {
	d := testDB(t)

	_, err := backlog.GetItem(d, "NONEXISTENT-99")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "item not found")
}

func TestGetItemViaAliasAfterPrefixRename(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	originalItem, err := backlog.AddItem(d, p.ID, p.Prefix, "P1", "task", "", "", "", "")
	require.NoError(t, err)
	originalID := originalItem.ID

	_, err = backlog.RenameProject(d, p.Name, "", "ZZ")
	require.NoError(t, err)

	// GetItem with original ID should resolve to renamed item
	got, err := backlog.GetItem(d, originalID)
	require.NoError(t, err)
	assert.Equal(t, "ZZ-1", got.ID)
	assert.Equal(t, "task", got.Title)
}

func TestUpdateItemInvalidStatus(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item", "", "", "", "")
	require.NoError(t, err)

	_, err = backlog.UpdateItem(d, "AC-1", map[string]string{"status": "invalid-status"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid status")
	assert.Contains(t, err.Error(), "open, done")
}

func TestUpdateItemValidStatuses(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	for _, status := range []string{"open", "done"} {
		_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item-"+status, "", "", "", "")
		require.NoError(t, err)

		item, err := backlog.GetItem(d, "AC-1")
		require.NoError(t, err)

		updated, err := backlog.UpdateItem(d, item.ID, map[string]string{"status": status})
		require.NoError(t, err)
		assert.Equal(t, status, updated.Status)

		_, err = backlog.DeleteItems(d, []string{item.ID})
		require.NoError(t, err)
	}
}

func TestAddItemDescriptionTooBig(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	bigDesc := string(make([]byte, backlog.MaxItemDescBytes+1))
	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item", bigDesc, "", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "description exceeds")
}

func TestAddItemNotesTooBig(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	bigNotes := string(make([]byte, backlog.MaxItemNotesBytes+1))
	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item", "", bigNotes, "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "notes exceeds")
}

func TestUpdateItemDescriptionTooBig(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item", "", "", "", "")
	require.NoError(t, err)

	bigDesc := string(make([]byte, backlog.MaxItemDescBytes+1))
	_, err = backlog.UpdateItem(d, "AC-1", map[string]string{"description": bigDesc})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "description exceeds")
}

func TestUpdateItemNotesTooBig(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item", "", "", "", "")
	require.NoError(t, err)

	bigNotes := string(make([]byte, backlog.MaxItemNotesBytes+1))
	_, err = backlog.UpdateItem(d, "AC-1", map[string]string{"notes": bigNotes})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "notes exceeds")
}

func TestSearchItemsByTitle(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "alphaqrx title", "desc", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "other item", "desc", "", "", "")
	require.NoError(t, err)

	items, err := backlog.SearchItems(d, "alphaqrx", backlog.ItemFilter{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "AC-1", items[0].ID)
}

func TestSearchItemsByHyphenatedTitle(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "fix-hyphen-bug title", "desc", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "other item", "desc", "", "", "")
	require.NoError(t, err)

	// Hyphenated query must find the hyphenated title.
	items, err := backlog.SearchItems(d, "fix-hyphen-bug", backlog.ItemFilter{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "AC-1", items[0].ID)

	// Multi-word hyphenated query.
	items, err = backlog.SearchItems(d, "fix-hyphen-bug title", backlog.ItemFilter{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "AC-1", items[0].ID)
}

func TestSearchItemsByDescriptionAndNotes(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "task one", "matchingdescterm", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "task two", "", "matchingnotesterm", "", "")
	require.NoError(t, err)

	byDesc, err := backlog.SearchItems(d, "matchingdescterm", backlog.ItemFilter{})
	require.NoError(t, err)
	require.Len(t, byDesc, 1)
	assert.Equal(t, "AC-1", byDesc[0].ID)

	byNotes, err := backlog.SearchItems(d, "matchingnotesterm", backlog.ItemFilter{})
	require.NoError(t, err)
	require.Len(t, byNotes, 1)
	assert.Equal(t, "AC-2", byNotes[0].ID)
}

func TestSearchItemsReflectsUpdates(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	item, err := backlog.AddItem(d, p.ID, "AC", "P1", "staletermtitle", "desc", "", "", "")
	require.NoError(t, err)

	_, err = backlog.UpdateItem(d, item.ID, map[string]string{"title": "freshtermtitle"})
	require.NoError(t, err)

	stale, err := backlog.SearchItems(d, "staletermtitle", backlog.ItemFilter{})
	require.NoError(t, err)
	assert.Empty(t, stale, "search index must not return stale pre-update title")

	fresh, err := backlog.SearchItems(d, "freshtermtitle", backlog.ItemFilter{})
	require.NoError(t, err)
	require.Len(t, fresh, 1)
	assert.Equal(t, item.ID, fresh[0].ID)
}

func TestSearchItemsExcludesDeleted(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	item, err := backlog.AddItem(d, p.ID, "AC", "P1", "deletemetermtitle", "desc", "", "", "")
	require.NoError(t, err)

	_, err = backlog.DeleteItems(d, []string{item.ID})
	require.NoError(t, err)

	items, err := backlog.SearchItems(d, "deletemetermtitle", backlog.ItemFilter{})
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestSearchItemsWithFilters(t *testing.T) {
	d := testDB(t)
	p1 := createTestProject(t, d)
	p2, err := backlog.CreateProject(d, "other-corp", "OC")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p1.ID, "AC", "P1", "sharedterm apionly", "", "", "api", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(d, p2.ID, "OC", "P1", "sharedterm apitwo", "", "", "api", "")
	require.NoError(t, err)

	items, err := backlog.SearchItems(d, "sharedterm", backlog.ItemFilter{ProjectIDs: []int64{p1.ID}})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "AC-1", items[0].ID)

	component := "api"
	items, err = backlog.SearchItems(d, "sharedterm", backlog.ItemFilter{Component: &component})
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

// TestSearchItemsSortByCreated verifies SearchItems' SortByCreated override
// orders newest-first instead of the default bm25-relevance ordering.
func TestSearchItemsSortByCreated(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	older, err := backlog.AddItem(d, p.ID, "AC", "P1", "sortableterm older", "", "", "", "")
	require.NoError(t, err)
	setItemCreated(t, d, older.ID, "2025-01-01T00:00:00Z")

	newer, err := backlog.AddItem(d, p.ID, "AC", "P1", "sortableterm newer", "", "", "", "")
	require.NoError(t, err)
	setItemCreated(t, d, newer.ID, "2026-01-01T00:00:00Z")

	items, err := backlog.SearchItems(d, "sortableterm", backlog.ItemFilter{SortByCreated: true})
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "sortableterm newer", items[0].Title)
	assert.Equal(t, "sortableterm older", items[1].Title)
}

func TestSearchItemsEmptyQuery(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "some title", "", "", "", "")
	require.NoError(t, err)

	items, err := backlog.SearchItems(d, "", backlog.ItemFilter{})
	require.NoError(t, err)
	assert.Empty(t, items)
}

// TestSearchItemsRelaxed_ORFallback_PartialMatch is the OU-346 regression
// for backlog items: a multi-term query where AND matches nothing but OR
// matches something must surface the partial match with relaxed=true.
func TestSearchItemsRelaxed_ORFallback_PartialMatch(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "bb_data egress design", "notes about egress, no bb_event or bb_sink here", "", "", "")
	require.NoError(t, err)

	andOnly, err := backlog.SearchItems(d, "bb_data bb_event bb_sink egress transport", backlog.ItemFilter{})
	require.NoError(t, err)
	require.Len(t, andOnly, 0)

	items, relaxed, err := backlog.SearchItemsRelaxed(d, "bb_data bb_event bb_sink egress transport", backlog.ItemFilter{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "bb_data egress design", items[0].Title)
	assert.True(t, relaxed)
}

// TestSearchItemsRelaxed_ANDMatches_NotRelaxed confirms AND stays primary:
// when the AND query already matches, the OR fallback never fires.
func TestSearchItemsRelaxed_ANDMatches_NotRelaxed(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "alpha and beta item", "desc", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "only alpha item", "desc", "", "", "")
	require.NoError(t, err)

	items, relaxed, err := backlog.SearchItemsRelaxed(d, "alpha beta", backlog.ItemFilter{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "alpha and beta item", items[0].Title)
	assert.False(t, relaxed)
}

// TestSearchItemsRelaxed_SingleToken_NoFallback confirms a single-token
// query behaves exactly as SearchItems — no OR retry is possible.
func TestSearchItemsRelaxed_SingleToken_NoFallback(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "some title", "", "", "", "")
	require.NoError(t, err)

	items, relaxed, err := backlog.SearchItemsRelaxed(d, "zzznothingmatchesthis", backlog.ItemFilter{})
	require.NoError(t, err)
	assert.Empty(t, items)
	assert.False(t, relaxed)
}

// TestSearchItemsRelaxed_ORAlsoEmpty_NotRelaxed confirms relaxed stays false
// when even the OR retry matches nothing.
func TestSearchItemsRelaxed_ORAlsoEmpty_NotRelaxed(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "some title", "", "", "", "")
	require.NoError(t, err)

	items, relaxed, err := backlog.SearchItemsRelaxed(d, "zzznothere zzzalsonothere", backlog.ItemFilter{})
	require.NoError(t, err)
	assert.Empty(t, items)
	assert.False(t, relaxed)
}
