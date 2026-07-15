package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/edges"
)

func TestRunBacklogGetSingle(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Critical bug", "desc here", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogGet(&buf, db, []string{"AC-1"}, false, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "[P0]")
	assert.Contains(t, output, "Critical bug")
	assert.Contains(t, output, "desc here")
	// Project name resolved (query.Get's IDs-mode zeroes ProjectID; the real
	// project id/name is looked up separately via resolveProjectName).
	assert.Contains(t, output, "acme-corp")
}

func TestRunBacklogGetVerboseNoEdges(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Item", "", "extra notes", "", "")
	require.NoError(t, err)
	_, err = backlog.UpdateItem(db, item.ID, map[string]string{"notes": "extra notes"})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogGet(&buf, db, []string{item.ID}, true, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "extra notes")
	assert.NotContains(t, output, "Edges:")
}

func TestRunBacklogGetVerboseIncludesEdges(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item1, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Item 1", "", "", "", "")
	require.NoError(t, err)
	item2, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Item 2", "", "", "", "")
	require.NoError(t, err)

	_, err = edges.Link(db, edges.TypeItem, item1.ID, "relates", edges.TypeItem, item2.ID, 0)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogGet(&buf, db, []string{item1.ID}, true, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Edges:")
	assert.Contains(t, output, "relates")
}

func TestRunBacklogGetJSON(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Item", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogGet(&buf, db, []string{"AC-1"}, false, true)
	require.NoError(t, err)

	var items []backlog.Item
	require.NoError(t, json.Unmarshal(buf.Bytes(), &items))
	require.Len(t, items, 1)
	assert.Equal(t, "Item", items[0].Title)
}

func TestRunBacklogGetMultiple(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "First", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Second", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogGet(&buf, db, []string{"AC-1", "AC-2"}, false, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "First")
	assert.Contains(t, output, "Second")
}

// TestRunBacklogGetMissingIDErrors: a lone missing id returns a "not found"
// error naming it, with no data written to out.
func TestRunBacklogGetMissingIDErrors(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runBacklogGet(&buf, db, []string{"AC-999"}, false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found: AC-999")
	assert.Empty(t, buf.String())
}

// TestRunBacklogGetMixedValidAndMissingID is the core CRITICAL-lesson
// regression guard: a missing id alongside a valid one must not corrupt
// stdout — the found item still renders cleanly, and the "not found"
// diagnostic travels solely via the returned error.
func TestRunBacklogGetMixedValidAndMissingID(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Found item", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogGet(&buf, db, []string{"AC-1", "AC-999"}, false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found: AC-999")

	output := buf.String()
	assert.Contains(t, output, "Found item")
	assert.NotContains(t, output, "not found")
}

// TestRunBacklogGetJSONMissing: the --json case of the same regression
// guard — out must still unmarshal as a clean JSON array containing only
// the found item, with the missing id surfacing solely via the error.
func TestRunBacklogGetJSONMissing(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Found item", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogGet(&buf, db, []string{"AC-1", "AC-999"}, false, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found: AC-999")

	var items []backlog.Item
	require.NoError(t, json.Unmarshal(buf.Bytes(), &items))
	require.Len(t, items, 1)
	assert.Equal(t, "Found item", items[0].Title)
}

// TestRunBacklogGetAllMissingJSON: every requested id missing still leaves
// out a clean, empty JSON array (never "null") alongside the error.
func TestRunBacklogGetAllMissingJSON(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runBacklogGet(&buf, db, []string{"AC-999"}, false, true)
	require.Error(t, err)

	var items []backlog.Item
	require.NoError(t, json.Unmarshal(buf.Bytes(), &items))
	assert.Empty(t, items)
}

func TestResolveProjectNameMissingItem(t *testing.T) {
	db := newTestDB(t)
	assert.Equal(t, "", resolveProjectName(db, "AC-999"))
}

// TestBacklogGetCmdRunEWiring exercises backlogGetCmd's RunE closure
// directly (flag parsing + withDB wiring), proving the cobra registration
// works end-to-end -- runBacklogGet's own behavior is already covered above.
func TestBacklogGetCmdRunEWiring(t *testing.T) {
	db := setupEdgesFileDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Wired item", "", "", "", "")
	require.NoError(t, err)

	backlogGetVerboseFlag = false
	backlogGetJSONFlag = false
	defer func() {
		backlogGetVerboseFlag = false
		backlogGetJSONFlag = false
	}()

	var buf bytes.Buffer
	backlogGetCmd.SetOut(&buf)
	err = backlogGetCmd.RunE(backlogGetCmd, []string{"AC-1"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Wired item")
}
