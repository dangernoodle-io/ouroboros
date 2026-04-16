package backlog_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
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

	item, err := backlog.AddItem(d, p.ID, "AC", "P1", "test-item", "test description", "", "")
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

func TestAddItemSequence(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	item1, err := backlog.AddItem(d, p.ID, "AC", "P1", "item1", "", "", "")
	require.NoError(t, err)
	assert.Equal(t, "AC-1", item1.ID)

	item2, err := backlog.AddItem(d, p.ID, "AC", "P2", "item2", "", "", "")
	require.NoError(t, err)
	assert.Equal(t, "AC-2", item2.ID)

	item3, err := backlog.AddItem(d, p.ID, "AC", "P3", "item3", "", "", "")
	require.NoError(t, err)
	assert.Equal(t, "AC-3", item3.ID)
}

func TestGetItem(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	created, err := backlog.AddItem(d, p.ID, "AC", "P1", "test-item", "desc", "", "")
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

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "old-title", "old-desc", "", "")
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

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item", "", "", "")
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

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item1", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "item2", "", "", "")
	require.NoError(t, err)

	items, err := backlog.ListItems(d, backlog.ItemFilter{})
	require.NoError(t, err)

	assert.Len(t, items, 2)
}

func TestListItemsFilterProject(t *testing.T) {
	d := testDB(t)
	p1 := createTestProject(t, d)
	p2, err := backlog.CreateProject(d, "other-corp", "OC")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p1.ID, "AC", "P1", "item1", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p2.ID, "OC", "P1", "item2", "", "", "")
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

	_, err = backlog.AddItem(d, p1.ID, "AC", "P1", "item1", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(d, p2.ID, "OC", "P1", "item2", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(d, p3.ID, "TC", "P1", "item3", "", "", "")
	require.NoError(t, err)

	items, err := backlog.ListItems(d, backlog.ItemFilter{ProjectIDs: []int64{p1.ID, p2.ID}})
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

func TestDeleteItemsSingle(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item1", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "item2", "", "", "")
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

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item1", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "item2", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P3", "item3", "", "", "")
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

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item1", "", "", "")
	require.NoError(t, err)

	affected, err := backlog.DeleteItems(d, []string{"AC-1", "NONEXISTENT"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), affected)
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

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item1", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "item2", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P3", "item3", "", "", "")
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

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item1", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "item2", "", "", "")
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

	item, err := backlog.AddItem(d, p.ID, "AC", "P1", "test-item", "desc", "", "ouroboros-mcp")
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
	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item1", "", "", "plugin-a")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "item2", "", "", "plugin-b")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P3", "item3", "", "", "plugin-a")
	require.NoError(t, err)

	// Filter to plugin-a
	component := "plugin-a"
	items, err := backlog.ListItems(d, backlog.ItemFilter{Component: &component})
	require.NoError(t, err)

	assert.Len(t, items, 2)
	assert.Equal(t, "AC-1", items[0].ID)
	assert.Equal(t, "AC-3", items[1].ID)
}

func TestUpdateItemComponent(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item", "", "", "plugin-a")
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

	_, err := backlog.AddItem(d, p.ID, "AC", "P0", "item1", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P1", "item2", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P1", "item3", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "item4", "", "", "")
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

	_, err = backlog.AddItem(d, p1.ID, "AC", "P0", "item1", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p1.ID, "AC", "P1", "item2", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p2.ID, "TP", "P0", "item3", "", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p2.ID, "TP", "P0", "item4", "", "", "")
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
