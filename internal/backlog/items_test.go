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

	item, err := backlog.AddItem(d, p.ID, "AC", "P1", "test-item", "test description", "")
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

	item1, err := backlog.AddItem(d, p.ID, "AC", "P1", "item1", "", "")
	require.NoError(t, err)
	assert.Equal(t, "AC-1", item1.ID)

	item2, err := backlog.AddItem(d, p.ID, "AC", "P2", "item2", "", "")
	require.NoError(t, err)
	assert.Equal(t, "AC-2", item2.ID)

	item3, err := backlog.AddItem(d, p.ID, "AC", "P3", "item3", "", "")
	require.NoError(t, err)
	assert.Equal(t, "AC-3", item3.ID)
}

func TestGetItem(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	created, err := backlog.AddItem(d, p.ID, "AC", "P1", "test-item", "desc", "")
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

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "old-title", "old-desc", "")
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

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item", "", "")
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

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item1", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "item2", "", "")
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

	_, err = backlog.AddItem(d, p1.ID, "AC", "P1", "item1", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p2.ID, "OC", "P1", "item2", "", "")
	require.NoError(t, err)

	items, err := backlog.ListItems(d, backlog.ItemFilter{ProjectID: &p1.ID})
	require.NoError(t, err)

	assert.Len(t, items, 1)
	assert.Equal(t, "AC-1", items[0].ID)
}

func TestListItemsFilterPriority(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item1", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "item2", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P3", "item3", "", "")
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

	_, err := backlog.AddItem(d, p.ID, "AC", "P1", "item1", "", "")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, p.ID, "AC", "P2", "item2", "", "")
	require.NoError(t, err)

	err = backlog.MarkDone(d, "AC-1")
	require.NoError(t, err)

	status := "open"
	items, err := backlog.ListItems(d, backlog.ItemFilter{Status: &status})
	require.NoError(t, err)

	assert.Len(t, items, 1)
	assert.Equal(t, "AC-2", items[0].ID)
}
