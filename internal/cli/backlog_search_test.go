package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
)

func TestRunBacklogSearchBasic(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Database migration", "Move to PostgreSQL", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Unrelated task", "Other work", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogSearch(&buf, db, "PostgreSQL", backlogSearchRequestFlags{Limit: 20})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Database migration")
	assert.NotContains(t, output, "Unrelated task")
}

func TestRunBacklogSearchJSON(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Widget feature", "widget details", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogSearch(&buf, db, "widget", backlogSearchRequestFlags{Limit: 20, JSON: true})
	require.NoError(t, err)

	var items []backlog.Item
	require.NoError(t, json.Unmarshal(buf.Bytes(), &items))
	require.Len(t, items, 1)
	assert.Equal(t, "Widget feature", items[0].Title)
}

func TestRunBacklogSearchProjectFilter(t *testing.T) {
	db := newTestDB(t)
	proj1, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	proj2, err := backlog.CreateProject(db, "other-project", "OP")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj1.ID, proj1.Prefix, "P0", "ACME widget", "widget stuff", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj2.ID, proj2.Prefix, "P0", "OTHER widget", "widget stuff", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogSearch(&buf, db, "widget", backlogSearchRequestFlags{Projects: []string{"acme-corp"}, Limit: 20})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "ACME widget")
	assert.NotContains(t, output, "OTHER widget")
}

func TestRunBacklogSearchStatusFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Open widget", "widget stuff", "", "", "")
	require.NoError(t, err)
	doneItem, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Done widget", "widget stuff", "", "", "")
	require.NoError(t, err)
	_, err = backlog.UpdateItem(db, doneItem.ID, map[string]string{"status": "done"})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogSearch(&buf, db, "widget", backlogSearchRequestFlags{Status: "done", Limit: 20})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Done widget")
	assert.NotContains(t, output, "Open widget")
}

func TestRunBacklogSearchPriorityFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Critical widget", "widget stuff", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "Low widget", "widget stuff", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogSearch(&buf, db, "widget", backlogSearchRequestFlags{Priority: "P0", Limit: 20})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Critical widget")
	assert.NotContains(t, output, "Low widget")
}

func TestRunBacklogSearchComponentFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Frontend widget", "widget stuff", "", "frontend", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Backend widget", "widget stuff", "", "backend", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogSearch(&buf, db, "widget", backlogSearchRequestFlags{Component: "frontend", Limit: 20})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Frontend widget")
	assert.NotContains(t, output, "Backend widget")
}

func TestRunBacklogSearchEpicsOnlyFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "EPIC: widget rollout", "widget stuff", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "widget subtask", "widget stuff", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogSearch(&buf, db, "widget", backlogSearchRequestFlags{EpicsOnly: true, Limit: 20})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "EPIC: widget rollout")
	assert.NotContains(t, output, "widget subtask")
}

func TestRunBacklogSearchSortCreated(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	older, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "First widget", "widget stuff", "", "", "")
	require.NoError(t, err)
	_, err = db.Exec("UPDATE items SET created = ? WHERE id = ?", "2025-01-01T00:00:00Z", older.ID)
	require.NoError(t, err)
	newer, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Second widget", "widget stuff", "", "", "")
	require.NoError(t, err)
	_, err = db.Exec("UPDATE items SET created = ? WHERE id = ?", "2026-01-01T00:00:00Z", newer.ID)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogSearch(&buf, db, "widget", backlogSearchRequestFlags{Sort: "created", Limit: 20, JSON: true})
	require.NoError(t, err)

	var items []backlog.Item
	require.NoError(t, json.Unmarshal(buf.Bytes(), &items))
	require.Len(t, items, 2)
	assert.Equal(t, "AC-2", items[0].ID)
}

func TestRunBacklogSearchSinceFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "widget item", "widget stuff", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogSearch(&buf, db, "widget", backlogSearchRequestFlags{Since: "24h", Limit: 20})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "widget item")
}

func TestRunBacklogSearchEmptyTextError(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runBacklogSearch(&buf, db, "", backlogSearchRequestFlags{Limit: 20})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "text is required")
}

func TestRunBacklogSearchInvalidSortErrors(t *testing.T) {
	db := newTestDB(t)
	var buf bytes.Buffer
	err := runBacklogSearch(&buf, db, "widget", backlogSearchRequestFlags{Sort: "bogus", Limit: 20})
	require.Error(t, err)
}

func TestRunBacklogSearchEpicFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	epic, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "EPIC: widget rollout", "widget stuff", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "widget child", "widget stuff", "", "", epic.ID)
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "widget unrelated", "widget stuff", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogSearch(&buf, db, "widget", backlogSearchRequestFlags{Epic: epic.ID, Limit: 20})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "widget child")
	assert.NotContains(t, output, "widget unrelated")
}

// TestBacklogSearchCmdRunEWiring exercises backlogSearchCmd's RunE closure
// directly (flag parsing + withDB wiring), proving the cobra registration
// works end-to-end -- runBacklogSearch's own behavior is already covered
// above.
func TestBacklogSearchCmdRunEWiring(t *testing.T) {
	db := setupEdgesFileDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Wired widget", "widget stuff", "", "", "")
	require.NoError(t, err)

	backlogSearchProjectFlag = nil
	backlogSearchStatusFlag = ""
	backlogSearchPriorityFlag = ""
	backlogSearchComponentFlag = ""
	backlogSearchEpicFlag = ""
	backlogSearchEpicsFlag = false
	backlogSearchSinceFlag = ""
	backlogSearchSortFlag = ""
	backlogSearchLimitFlag = 20
	backlogSearchJSONFlag = false
	defer func() {
		backlogSearchProjectFlag = nil
		backlogSearchStatusFlag = ""
		backlogSearchPriorityFlag = ""
		backlogSearchComponentFlag = ""
		backlogSearchEpicFlag = ""
		backlogSearchEpicsFlag = false
		backlogSearchSinceFlag = ""
		backlogSearchSortFlag = ""
		backlogSearchLimitFlag = 20
		backlogSearchJSONFlag = false
	}()

	var buf bytes.Buffer
	backlogSearchCmd.SetOut(&buf)
	err = backlogSearchCmd.RunE(backlogSearchCmd, []string{"widget"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Wired widget")
}
