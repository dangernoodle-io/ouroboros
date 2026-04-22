package cli

import (
	"bytes"
	"encoding/json"
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
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Critical bug", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "Feature", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSItems(&buf, db, "", "", "", "", false)
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
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Task", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSItems(&buf, db, "", "", "", "", true)
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

	_, err = backlog.AddItem(db, proj1.ID, proj1.Prefix, "P0", "AC item", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj2.ID, proj2.Prefix, "P0", "OP item", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSItems(&buf, db, "acme-corp", "", "", "", false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "AC-1")
	assert.NotContains(t, output, "OP-1")
}

func TestLSItemsStatusFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Open", "", "", "")
	require.NoError(t, err)
	item2, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Done", "", "", "")
	require.NoError(t, err)

	_, err = backlog.UpdateItem(db, item2.ID, map[string]string{"status": "done"})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSItems(&buf, db, "", "done", "", "", false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "AC-2")
	assert.NotContains(t, output, "AC-1")
}

func TestLSItemsPriorityFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Critical", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "Medium", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSItems(&buf, db, "", "", "P0", "", false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "AC-1")
	assert.NotContains(t, output, "AC-2")
}

func TestLSItemsComponentFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Auth task", "", "", "auth")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "DB task", "", "", "db")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSItems(&buf, db, "", "", "", "auth", false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "AC-1")
	assert.NotContains(t, output, "AC-2")
}

func TestLSItemsDetailJSON(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Task title", "This is a description", "Important note", "component-x")
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

	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Task", "Description text", "Notes text", "auth")
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
	err := runLSItems(&buf, db, "nonexistent", "", "", "", false)
	require.NoError(t, err)

	output := strings.TrimSpace(buf.String())
	// Empty table (header only or minimal content)
	lines := strings.Split(output, "\n")
	assert.LessOrEqual(t, len(lines), 2)
}
