package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
)

func TestLSItemsList(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Critical bug", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "Feature", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSItems(&buf, db, "", "", "", "", "", false, "", "", 20, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "ID")
	assert.Contains(t, output, "PRIORITY")
	assert.Contains(t, output, "AC-1")
	assert.Contains(t, output, "P0")
	assert.Contains(t, output, "AC-2")
	assert.Contains(t, output, "P3")
}

func TestLSItemsListJSON(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Task", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSItems(&buf, db, "", "", "", "", "", false, "", "", 20, true)
	require.NoError(t, err)

	var items []backlog.Item
	err = json.Unmarshal(buf.Bytes(), &items)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "AC-1", items[0].ID)
	assert.Equal(t, "Task", items[0].Title)
}

func TestLSItemsProjectFilter(t *testing.T) {
	db := newTestDB(t)
	proj1, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	proj2, err := backlog.CreateProject(db, "other-project", "OP")
	require.NoError(t, err)

	_, err = backlog.AddItem(db, proj1.ID, proj1.Prefix, "P0", "AC item", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj2.ID, proj2.Prefix, "P0", "OP item", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSItems(&buf, db, "acme-corp", "", "", "", "", false, "", "", 20, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "AC-1")
	assert.NotContains(t, output, "OP-1")
}

func TestLSItemsStatusFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Open", "", "", "", "")
	require.NoError(t, err)
	item2, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Done", "", "", "", "")
	require.NoError(t, err)

	_, err = backlog.UpdateItem(db, item2.ID, map[string]string{"status": "done"})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSItems(&buf, db, "", "done", "", "", "", false, "", "", 20, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "AC-2")
	assert.NotContains(t, output, "AC-1")
}

func TestLSItemsPriorityFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Critical", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "Medium", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSItems(&buf, db, "", "", "P0", "", "", false, "", "", 20, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "AC-1")
	assert.NotContains(t, output, "AC-2")
}

func TestLSItemsComponentFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Auth task", "", "", "auth", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "DB task", "", "", "db", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSItems(&buf, db, "", "", "", "auth", "", false, "", "", 20, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "AC-1")
	assert.NotContains(t, output, "AC-2")
}

func TestLSItemsEpicFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	epic, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "EPIC: demo", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P2", "child of demo", "", "", "", epic.ID)
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P2", "unrelated", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSItems(&buf, db, "", "", "", "", epic.ID, false, "", "", 20, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "child of demo")
	assert.Contains(t, output, epic.ID)
	assert.NotContains(t, output, "unrelated")
}

func TestLSItemsEpicsOnlyFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "EPIC: demo", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P2", "regular task", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSItems(&buf, db, "", "", "", "", "", true, "", "", 20, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "EPIC: demo")
	assert.NotContains(t, output, "regular task")
}

func TestLSItemsCreatedShownInRows(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "task", "", "", "", "")
	require.NoError(t, err)
	_, err = db.Exec("UPDATE items SET created = ? WHERE id = ?", "2026-03-01T00:00:00Z", item.ID)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSItems(&buf, db, "", "", "", "", "", false, "", "", 20, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "CREATED")
	assert.Contains(t, output, "2026-03-01T00:00:00Z")
}

func TestLSItemsSortCreated(t *testing.T) {
	db := newTestDB(t)
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

	var buf bytes.Buffer
	err = runLSItems(&buf, db, "", "", "", "", "", false, "", "created", 20, false)
	require.NoError(t, err)

	output := buf.String()
	newerIdx := strings.Index(output, "newer")
	olderIdx := strings.Index(output, "older")
	require.NotEqual(t, -1, newerIdx)
	require.NotEqual(t, -1, olderIdx)
	assert.Less(t, newerIdx, olderIdx, "newest-first: newer row must appear before older")
}

func TestLSItemsSortInvalid_Errors(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runLSItems(&buf, db, "", "", "", "", "", false, "", "bogus", 20, false)
	assert.Error(t, err)
}

func TestLSItemsSinceDuration(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	old, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "old item", "", "", "", "")
	require.NoError(t, err)
	_, err = db.Exec("UPDATE items SET created = ? WHERE id = ?", "2020-01-01T00:00:00Z", old.ID)
	require.NoError(t, err)

	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "recent item", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSItems(&buf, db, "", "", "", "", "", false, "24h", "", 20, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "recent item")
	assert.NotContains(t, output, "old item")
}

func TestLSItemsSinceDate(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	old, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "old item", "", "", "", "")
	require.NoError(t, err)
	_, err = db.Exec("UPDATE items SET created = ? WHERE id = ?", "2020-01-01T00:00:00Z", old.ID)
	require.NoError(t, err)

	recent, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "recent item", "", "", "", "")
	require.NoError(t, err)
	_, err = db.Exec("UPDATE items SET created = ? WHERE id = ?", "2026-06-01T00:00:00Z", recent.ID)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSItems(&buf, db, "", "", "", "", "", false, "2026-01-01", "", 20, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "recent item")
	assert.NotContains(t, output, "old item")
}

func TestLSItemsSinceInvalid_Errors(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runLSItems(&buf, db, "", "", "", "", "", false, "not-a-date", "", 20, false)
	assert.Error(t, err)
}

// TestLSItemsEpicWithSortCreated verifies --epic combined with --sort
// created: that epic's children, newest-first.
func TestLSItemsEpicWithSortCreated(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	epic, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "EPIC: demo", "", "", "", "")
	require.NoError(t, err)

	older, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "older child", "", "", "", epic.ID)
	require.NoError(t, err)
	_, err = db.Exec("UPDATE items SET created = ? WHERE id = ?", "2025-01-01T00:00:00Z", older.ID)
	require.NoError(t, err)

	newer, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "newer child", "", "", "", epic.ID)
	require.NoError(t, err)
	_, err = db.Exec("UPDATE items SET created = ? WHERE id = ?", "2026-01-01T00:00:00Z", newer.ID)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSItems(&buf, db, "", "", "", "", epic.ID, false, "", "created", 20, false)
	require.NoError(t, err)

	output := buf.String()
	newerIdx := strings.Index(output, "newer child")
	olderIdx := strings.Index(output, "older child")
	require.NotEqual(t, -1, newerIdx)
	require.NotEqual(t, -1, olderIdx)
	assert.Less(t, newerIdx, olderIdx)
}

func TestLSItemsDetailJSON(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Task title", "This is a description", "Important note", "component-x", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSItemDetail(&buf, db, "AC-1", true)
	require.NoError(t, err)

	var item backlog.Item
	err = json.Unmarshal(buf.Bytes(), &item)
	require.NoError(t, err)
	assert.Equal(t, "AC-1", item.ID)
	assert.Equal(t, "Task title", item.Title)
	assert.Equal(t, "This is a description", item.Description)
}

func TestLSItemsDetailPlain(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Task", "Description text", "Notes text", "auth", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSItemDetail(&buf, db, "AC-1", false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "AC-1")
	assert.Contains(t, output, "[P0]")
	assert.Contains(t, output, "acme-corp")
	assert.Contains(t, output, "Component: auth")
	assert.Contains(t, output, "Task")
	assert.Contains(t, output, "Description:")
	assert.Contains(t, output, "Notes:")
}

func TestLSItemsProjectNotFound(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runLSItems(&buf, db, "nonexistent", "", "", "", "", false, "", "", 20, false)
	require.NoError(t, err)

	output := strings.TrimSpace(buf.String())
	// Empty table (header only or minimal content)
	lines := strings.Split(output, "\n")
	assert.LessOrEqual(t, len(lines), 2)
}

func TestLSItemsLimit(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	for i := range 5 {
		_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", fmt.Sprintf("Item %d", i+1), "", "", "", "")
		require.NoError(t, err)
	}

	var buf bytes.Buffer
	err = runLSItems(&buf, db, "acme-corp", "open", "", "", "", false, "", "", 3, true)
	require.NoError(t, err)

	var items []backlog.Item
	require.NoError(t, json.Unmarshal(buf.Bytes(), &items))
	assert.Len(t, items, 3)
}
