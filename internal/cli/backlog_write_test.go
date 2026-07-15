package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/edges"
)

// resetBacklogWriteFlags clears the package-scoped cobra flag vars AND each
// flag's Changed state (cmd.Flags().Changed(...) is what the RunE closures
// key off, not "non-empty string") so tests that invoke a *Cmd.RunE
// directly (rather than through a fresh cobra.Execute) don't leak state
// between cases -- mirrors edges_group_test.go's resetEdgesListFlags, plus
// the Changed reset backlog update's Changed-based semantics require.
func resetBacklogWriteFlags() {
	backlogCreateProjectFlag = ""
	backlogCreatePriorityFlag = ""
	backlogCreateTitleFlag = ""
	backlogCreateComponentFlag = ""
	backlogCreateEpicFlag = ""
	backlogCreateDescriptionFlag = ""
	backlogCreateNotesFlag = ""
	backlogCreateCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })

	backlogUpdatePriorityFlag = ""
	backlogUpdateTitleFlag = ""
	backlogUpdateStatusFlag = ""
	backlogUpdateComponentFlag = ""
	backlogUpdateEpicFlag = ""
	backlogUpdateDescriptionFlag = ""
	backlogUpdateNotesFlag = ""
	backlogUpdateAppendNotesFlag = false
	backlogUpdateCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
}

// --- backlog create: direct runBacklogCreate unit tests ---

func TestRunBacklogCreateHappy(t *testing.T) {
	db := newTestDB(t)
	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogCreate(&buf, db, backlogCreateRequest{
		Project:  "acme-corp",
		Priority: "p1",
		Title:    "New item",
	})
	require.NoError(t, err)

	var item backlog.Item
	require.NoError(t, json.Unmarshal(buf.Bytes(), &item))
	assert.Equal(t, "AC-1", item.ID)
	assert.Equal(t, "P1", item.Priority, "priority normalized to uppercase")
	assert.Equal(t, "open", item.Status)
}

func TestRunBacklogCreateBadPriority(t *testing.T) {
	db := newTestDB(t)
	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogCreate(&buf, db, backlogCreateRequest{Project: "acme-corp", Priority: "bogus", Title: "x"})
	assert.Error(t, err)
}

func TestRunBacklogCreateUnknownProject(t *testing.T) {
	db := newTestDB(t)
	var buf bytes.Buffer
	err := runBacklogCreate(&buf, db, backlogCreateRequest{Project: "nope", Priority: "P1", Title: "x"})
	assert.Error(t, err)
}

func TestRunBacklogCreateUnknownEpic(t *testing.T) {
	db := newTestDB(t)
	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogCreate(&buf, db, backlogCreateRequest{Project: "acme-corp", Priority: "P1", Title: "x", Epic: "AC-99"})
	assert.Error(t, err)
}

func TestRunBacklogCreateOverCapDescription(t *testing.T) {
	db := newTestDB(t)
	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogCreate(&buf, db, backlogCreateRequest{
		Project: "acme-corp", Priority: "P1", Title: "x",
		Description: strings.Repeat("a", 501),
	})
	assert.Error(t, err)
}

func TestRunBacklogCreateWithEpicAndComponent(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	epic, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "EPIC: big thing", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogCreate(&buf, db, backlogCreateRequest{
		Project: "acme-corp", Priority: "P2", Title: "child", Component: "widget", Epic: epic.ID,
	})
	require.NoError(t, err)

	var item backlog.Item
	require.NoError(t, json.Unmarshal(buf.Bytes(), &item))
	assert.Equal(t, epic.ID, item.Epic)
	assert.Equal(t, "widget", item.Component)
}

// TestRunBacklogCreateAddItemBackendError injects a backend failure at the
// AddItem call specifically (not the earlier GetProjectByName lookup): a
// fully-closed *sql.DB can't isolate this branch, since GetProjectByName is
// the first db call runBacklogCreate makes and fails identically on a closed
// db (already covered by TestRunBacklogCreateUnknownProject) -- the closed-db
// error would never reach AddItem. Instead, a BEFORE INSERT trigger blocks
// only the INSERT (items table), leaving the projects-table SELECT that
// GetProjectByName issues unaffected.
func TestRunBacklogCreateAddItemBackendError(t *testing.T) {
	db := newTestDB(t)
	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TRIGGER block_item_insert BEFORE INSERT ON items BEGIN SELECT RAISE(ABORT, 'blocked for test'); END;`)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogCreate(&buf, db, backlogCreateRequest{Project: "acme-corp", Priority: "P1", Title: "x"})
	assert.Error(t, err)
}

// --- backlog create: RunE wiring test ---

func TestBacklogCreateCmdRunEWiring(t *testing.T) {
	db := setupEdgesFileDB(t)
	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	resetBacklogWriteFlags()
	backlogCreateProjectFlag = "acme-corp"
	backlogCreatePriorityFlag = "P1"
	backlogCreateTitleFlag = "wired item"

	var buf bytes.Buffer
	backlogCreateCmd.SetOut(&buf)
	require.NoError(t, backlogCreateCmd.RunE(backlogCreateCmd, nil))

	var item backlog.Item
	require.NoError(t, json.Unmarshal(buf.Bytes(), &item))
	assert.Equal(t, "wired item", item.Title)
}

func TestBacklogCreateCmdRunERequiredFields(t *testing.T) {
	resetBacklogWriteFlags()
	assert.Error(t, backlogCreateCmd.RunE(backlogCreateCmd, nil))

	backlogCreateProjectFlag = "acme-corp"
	assert.Error(t, backlogCreateCmd.RunE(backlogCreateCmd, nil))

	backlogCreatePriorityFlag = "P1"
	assert.Error(t, backlogCreateCmd.RunE(backlogCreateCmd, nil))

	resetBacklogWriteFlags()
}

// --- backlog update: direct runBacklogUpdate unit tests ---

func strPtr(s string) *string { return &s }

func TestRunBacklogUpdateHappyPerField(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "orig", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogUpdate(&buf, db, item.ID, backlogUpdateFields{
		Priority:  strPtr("p0"),
		Title:     strPtr("updated title"),
		Status:    strPtr("done"),
		Component: strPtr("widget"),
	})
	require.NoError(t, err)

	var updated backlog.Item
	require.NoError(t, json.Unmarshal(buf.Bytes(), &updated))
	assert.Equal(t, "P0", updated.Priority)
	assert.Equal(t, "updated title", updated.Title)
	assert.Equal(t, "done", updated.Status)
	assert.Equal(t, "widget", updated.Component)
}

func TestRunBacklogUpdateNoFieldsErrors(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "orig", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogUpdate(&buf, db, item.ID, backlogUpdateFields{})
	assert.Error(t, err)
}

func TestRunBacklogUpdateClearEpicDescriptionNotes(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	epic, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "EPIC: e", "", "", "", "")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "orig", "desc", "notes", "", epic.ID)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogUpdate(&buf, db, item.ID, backlogUpdateFields{
		Epic:        strPtr(""),
		Description: strPtr(""),
		Notes:       strPtr(""),
	})
	require.NoError(t, err)

	got, err := backlog.GetItem(db, item.ID)
	require.NoError(t, err)
	assert.Empty(t, got.Epic)
	assert.Empty(t, got.Description)
	assert.Empty(t, got.Notes)
}

func TestRunBacklogUpdateAppendNotesAppends(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "orig", "", "existing", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogUpdate(&buf, db, item.ID, backlogUpdateFields{
		Notes:       strPtr("more"),
		AppendNotes: true,
	})
	require.NoError(t, err)

	got, err := backlog.GetItem(db, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "existing\n\nmore", got.Notes)
}

func TestRunBacklogUpdateAppendNotesEmptyIsNoop(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "orig", "", "existing", "", "")
	require.NoError(t, err)

	// notes Changed but empty + append-notes: nothing to append -> only
	// updating notes contributes no field, so another field must be present
	// or this becomes the "no fields to update" error.
	var buf bytes.Buffer
	err = runBacklogUpdate(&buf, db, item.ID, backlogUpdateFields{
		Notes:       strPtr(""),
		AppendNotes: true,
	})
	assert.Error(t, err)

	got, err := backlog.GetItem(db, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "existing", got.Notes, "empty append-notes must not clear existing notes")
}

func TestRunBacklogUpdateEmptyTitleRejected(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "orig", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogUpdate(&buf, db, item.ID, backlogUpdateFields{Title: strPtr("")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "title cannot be empty")

	got, err := backlog.GetItem(db, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "orig", got.Title, "rejected empty title must not modify the item")
}

func TestRunBacklogUpdateAppendNotesOntoEmptyExisting(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "orig", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogUpdate(&buf, db, item.ID, backlogUpdateFields{
		Notes:       strPtr("first note"),
		AppendNotes: true,
	})
	require.NoError(t, err)

	got, err := backlog.GetItem(db, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "first note", got.Notes, "appending onto empty existing notes must not add a leading separator")
}

func TestRunBacklogUpdateBadStatus(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "orig", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogUpdate(&buf, db, item.ID, backlogUpdateFields{Status: strPtr("bogus")})
	assert.Error(t, err)
}

func TestRunBacklogUpdateBadPriority(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "orig", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogUpdate(&buf, db, item.ID, backlogUpdateFields{Priority: strPtr("bogus")})
	assert.Error(t, err)
}

func TestRunBacklogUpdateEpicValidation(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "orig", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogUpdate(&buf, db, item.ID, backlogUpdateFields{Epic: strPtr("AC-99")})
	assert.Error(t, err)
}

func TestRunBacklogUpdateOverCapDescription(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "orig", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogUpdate(&buf, db, item.ID, backlogUpdateFields{Description: strPtr(strings.Repeat("a", 501))})
	assert.Error(t, err)
}

// TestRunBacklogUpdateBackendErrorUpdateItem injects a closed *sql.DB: with
// only Priority changed, runBacklogUpdate makes zero db calls before
// backlog.UpdateItem, so the closed-db error surfaces there directly.
func TestRunBacklogUpdateBackendErrorUpdateItem(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "orig", "", "", "", "")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	var buf bytes.Buffer
	err = runBacklogUpdate(&buf, db, item.ID, backlogUpdateFields{Priority: strPtr("P1")})
	assert.Error(t, err)
}

// TestRunBacklogUpdateBackendErrorAppendNotesGetItem injects a closed
// *sql.DB: with only Notes+AppendNotes set, the first (and only) db call
// before reaching appendBacklogUpdateNotes's backlog.GetItem is that GetItem
// call itself, so the closed-db error propagates through it directly.
func TestRunBacklogUpdateBackendErrorAppendNotesGetItem(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "orig", "", "existing", "", "")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	var buf bytes.Buffer
	err = runBacklogUpdate(&buf, db, item.ID, backlogUpdateFields{Notes: strPtr("more"), AppendNotes: true})
	assert.Error(t, err)
}

// --- backlog update: RunE wiring test ---

func TestBacklogUpdateCmdRunEWiring(t *testing.T) {
	db := setupEdgesFileDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "orig", "", "", "", "")
	require.NoError(t, err)

	resetBacklogWriteFlags()
	require.NoError(t, backlogUpdateCmd.Flags().Set("title", "wired update"))

	var buf bytes.Buffer
	backlogUpdateCmd.SetOut(&buf)
	require.NoError(t, backlogUpdateCmd.RunE(backlogUpdateCmd, []string{item.ID}))

	var updated backlog.Item
	require.NoError(t, json.Unmarshal(buf.Bytes(), &updated))
	assert.Equal(t, "wired update", updated.Title)

	resetBacklogWriteFlags()
}

// TestBacklogUpdateCmdRunEEachChangedFlag drives backlogUpdateCmd.RunE
// directly (not runBacklogUpdate) once per flag so every
// cmd.Flags().Changed(...) branch in the RunE closure executes, not just
// "title" (the only flag TestBacklogUpdateCmdRunEWiring sets).
func TestBacklogUpdateCmdRunEEachChangedFlag(t *testing.T) {
	db := setupEdgesFileDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	epic, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "EPIC: e", "", "", "", "")
	require.NoError(t, err)

	cases := []struct {
		name  string
		flag  string
		value string
		check func(t *testing.T, item backlog.Item)
	}{
		{"priority", "priority", "p0", func(t *testing.T, item backlog.Item) {
			assert.Equal(t, "P0", item.Priority)
		}},
		{"status", "status", "done", func(t *testing.T, item backlog.Item) {
			assert.Equal(t, "done", item.Status)
		}},
		{"component", "component", "widget", func(t *testing.T, item backlog.Item) {
			assert.Equal(t, "widget", item.Component)
		}},
		{"epic", "epic", epic.ID, func(t *testing.T, item backlog.Item) {
			assert.Equal(t, epic.ID, item.Epic)
		}},
		{"description", "description", "new desc", func(t *testing.T, item backlog.Item) {
			assert.Equal(t, "new desc", item.Description)
		}},
		{"notes", "notes", "new notes", func(t *testing.T, item backlog.Item) {
			assert.Equal(t, "new notes", item.Notes)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "orig-"+tc.name, "", "", "", "")
			require.NoError(t, err)

			resetBacklogWriteFlags()
			require.NoError(t, backlogUpdateCmd.Flags().Set(tc.flag, tc.value))

			var buf bytes.Buffer
			backlogUpdateCmd.SetOut(&buf)
			require.NoError(t, backlogUpdateCmd.RunE(backlogUpdateCmd, []string{item.ID}))

			var updated backlog.Item
			require.NoError(t, json.Unmarshal(buf.Bytes(), &updated))
			tc.check(t, updated)

			resetBacklogWriteFlags()
		})
	}
}

// TestBacklogUpdateCmdRunEAppendNotes exercises the append-notes bool flag's
// Changed-independent wiring (append-notes has no Changed check in RunE — it
// is read unconditionally) via the notes+append-notes combo at RunE level.
func TestBacklogUpdateCmdRunEAppendNotes(t *testing.T) {
	db := setupEdgesFileDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "orig", "", "existing", "", "")
	require.NoError(t, err)

	resetBacklogWriteFlags()
	require.NoError(t, backlogUpdateCmd.Flags().Set("notes", "more"))
	require.NoError(t, backlogUpdateCmd.Flags().Set("append-notes", "true"))

	var buf bytes.Buffer
	backlogUpdateCmd.SetOut(&buf)
	require.NoError(t, backlogUpdateCmd.RunE(backlogUpdateCmd, []string{item.ID}))

	var updated backlog.Item
	require.NoError(t, json.Unmarshal(buf.Bytes(), &updated))
	assert.Equal(t, "existing\n\nmore", updated.Notes)

	resetBacklogWriteFlags()
}

func TestBacklogUpdateCmdRunENoFieldsChanged(t *testing.T) {
	db := setupEdgesFileDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "orig", "", "", "", "")
	require.NoError(t, err)

	resetBacklogWriteFlags()
	var buf bytes.Buffer
	backlogUpdateCmd.SetOut(&buf)
	err = backlogUpdateCmd.RunE(backlogUpdateCmd, []string{item.ID})
	assert.Error(t, err)
}

// --- backlog delete: direct runBacklogDelete unit tests ---

func TestRunBacklogDeleteSingle(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "orig", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogDelete(&buf, db, []string{item.ID})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "deleted 1 item(s)")

	_, err = backlog.GetItem(db, item.ID)
	assert.Error(t, err)
}

func TestRunBacklogDeleteMultiAndDedup(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	a, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "a", "", "", "", "")
	require.NoError(t, err)
	b, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "b", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogDelete(&buf, db, []string{a.ID, b.ID, a.ID})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "deleted 2 item(s)")

	_, err = backlog.GetItem(db, a.ID)
	assert.Error(t, err)
	_, err = backlog.GetItem(db, b.ID)
	assert.Error(t, err)
}

func TestRunBacklogDeleteOneMissingAbortsNothingDeleted(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	a, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "a", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogDelete(&buf, db, []string{a.ID, "AC-99"})
	assert.Error(t, err)

	survivor, err := backlog.GetItem(db, a.ID)
	require.NoError(t, err, "survivor must not be deleted when the batch aborts")
	assert.Equal(t, a.ID, survivor.ID)
}

func TestRunBacklogDeleteCascadesEdge(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	a, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "a", "", "", "", "")
	require.NoError(t, err)
	b, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "b", "", "", "", "")
	require.NoError(t, err)

	_, err = edges.Link(db, edges.TypeItem, a.ID, "relates", edges.TypeItem, b.ID, proj.ID)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, runBacklogDelete(&buf, db, []string{a.ID}))

	remaining, err := edges.EdgesFor(db, edges.TypeItem, b.ID)
	require.NoError(t, err)
	assert.Empty(t, remaining, "deleting a backlog item must cascade-remove edges referencing it")
}

// TestRunBacklogDeleteBackendErrorGetItemsByIDs injects a closed *sql.DB:
// GetItemsByIDs is runBacklogDelete's first db call, so the closed-db error
// surfaces there directly.
func TestRunBacklogDeleteBackendErrorGetItemsByIDs(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "orig", "", "", "", "")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	var buf bytes.Buffer
	err = runBacklogDelete(&buf, db, []string{item.ID})
	assert.Error(t, err)
}

// TestRunBacklogDeleteBackendErrorDeleteItems injects a backend failure at
// the DeleteItems call specifically (not the earlier GetItemsByIDs
// pre-check): a fully-closed *sql.DB can't isolate this branch, since
// GetItemsByIDs is the first db call runBacklogDelete makes and fails
// identically on a closed db (same failure mode as the GetItemsByIDs test
// above) -- the closed-db error would never reach DeleteItems. Instead, a
// BEFORE DELETE trigger blocks only the DELETE, leaving the preceding SELECT
// (which finds the item and lets the existence pre-check pass) unaffected.
func TestRunBacklogDeleteBackendErrorDeleteItems(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "orig", "", "", "", "")
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TRIGGER block_item_delete BEFORE DELETE ON items BEGIN SELECT RAISE(ABORT, 'blocked for test'); END;`)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runBacklogDelete(&buf, db, []string{item.ID})
	assert.Error(t, err)
}

// --- backlog delete: RunE wiring test ---

func TestBacklogDeleteCmdRunEWiring(t *testing.T) {
	db := setupEdgesFileDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "orig", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	backlogDeleteCmd.SetOut(&buf)
	require.NoError(t, backlogDeleteCmd.RunE(backlogDeleteCmd, []string{item.ID}))
	assert.Contains(t, buf.String(), "deleted 1 item(s)")

	_, err = backlog.GetItem(db, item.ID)
	assert.Error(t, err)
}

// --- group wiring ---

func TestBacklogCmdGroupWiringIncludesWriteVerbs(t *testing.T) {
	names := make([]string, 0)
	for _, c := range backlogCmd.Commands() {
		names = append(names, c.Name())
	}
	assert.Contains(t, names, "create")
	assert.Contains(t, names, "update")
	assert.Contains(t, names, "delete")
}
