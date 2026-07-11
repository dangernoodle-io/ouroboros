package app

import (
	"context"
	"strings"
	"testing"

	"github.com/dangernoodle-io/mcpkit/mcpx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/edges"
)

// backlogResult mirrors the {"id":..., "action":...} shape handleBacklogV2
// marshals for each entries[] result.
type backlogResult struct {
	ID     string `json:"id"`
	Action string `json:"action"`
}

// TestHandleBacklogV2BatchCreateAndUpdate mirrors
// TestHandleBacklogBatchCreateAndUpdate.
func TestHandleBacklogV2BatchCreateAndUpdate(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "test-project", "TP")
	require.NoError(t, err)
	item1, err := backlog.AddItem(db, proj.ID, "TP", "P0", "Task 1", "Initial", "", "", "")
	require.NoError(t, err)

	in := backlogInput{Entries: []backlogEntryInput{
		{ID: strPtrV2(item1.ID), Priority: strPtrV2("P2"), Title: strPtrV2("Task 1 Updated")},
		{Project: strPtrV2("test-project"), Priority: strPtrV2("P1"), Title: strPtrV2("Task 2 New")},
	}}

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, in)
	require.NoError(t, err)
	require.False(t, res.IsError)

	var resp []backlogResult
	require.NoError(t, jsonUnmarshalText(res, &resp))
	require.Len(t, resp, 2)
	assert.Equal(t, "update", resp[0].Action)
	assert.Equal(t, "create", resp[1].Action)

	updated, err := backlog.GetItem(db, item1.ID)
	require.NoError(t, err)
	assert.Equal(t, "Task 1 Updated", updated.Title)
	assert.Equal(t, "P2", updated.Priority)
}

// TestHandleBacklogV2DeleteNonexistent mirrors TestHandleBacklogDeleteNonexistent.
func TestHandleBacklogV2DeleteNonexistent(t *testing.T) {
	resetAllDB(t)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		DeleteIDs: []string{"NONEXISTENT"},
	})
	require.NoError(t, err)

	var resp map[string]interface{}
	require.NoError(t, jsonUnmarshalText(res, &resp))
	assert.Equal(t, float64(0), resp["deleted"])
}

// TestHandleBacklogV2DeleteMultiple mirrors TestHandleBacklogDeleteMultiple.
func TestHandleBacklogV2DeleteMultiple(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item1, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Delete me 1", "", "", "", "")
	require.NoError(t, err)
	item2, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P2", "Delete me 2", "", "", "", "")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		DeleteIDs: []string{item1.ID, item2.ID},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	var resp map[string]interface{}
	require.NoError(t, jsonUnmarshalText(res, &resp))
	assert.Equal(t, float64(2), resp["deleted"])
}

// TestHandleBacklogV2DeleteTakesPriorityOverEntries verifies delete_ids wins
// when both delete_ids and entries are present, matching handleBacklog's
// dispatch order (delete_ids checked first).
func TestHandleBacklogV2DeleteTakesPriorityOverEntries(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item1, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Delete me", "", "", "", "")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		DeleteIDs: []string{item1.ID},
		Entries: []backlogEntryInput{
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P1"), Title: strPtrV2("should not be created")},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	var resp map[string]interface{}
	require.NoError(t, jsonUnmarshalText(res, &resp))
	assert.Equal(t, float64(1), resp["deleted"])

	_, err = backlog.GetItem(db, "AC-2")
	assert.Error(t, err, "entries must not be processed when delete_ids is present")
}

// TestHandleBacklogV2Create mirrors TestHandleBacklogCreate.
func TestHandleBacklogV2Create(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P2"), Title: strPtrV2("New task")},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	var resp []backlogResult
	require.NoError(t, jsonUnmarshalText(res, &resp))
	require.Len(t, resp, 1)
	assert.Equal(t, "create", resp[0].Action)
	assert.Equal(t, "AC-1", resp[0].ID)
}

// TestHandleBacklogV2CreateWithNotesAndComponent mirrors
// TestHandleBacklogCreateWithNotesAndComponent.
func TestHandleBacklogV2CreateWithNotesAndComponent(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{
				Project:     strPtrV2("acme-corp"),
				Priority:    strPtrV2("P1"),
				Title:       strPtrV2("Task with extras"),
				Description: strPtrV2("A short description"),
				Notes:       strPtrV2("Detailed notes here"),
				Component:   strPtrV2("api"),
			},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	var resp []backlogResult
	require.NoError(t, jsonUnmarshalText(res, &resp))
	require.Len(t, resp, 1)
	assert.Equal(t, "create", resp[0].Action)

	item, err := backlog.GetItem(db, "AC-1")
	require.NoError(t, err)
	assert.Equal(t, "Detailed notes here", item.Notes)
	assert.Equal(t, "api", item.Component)
}

// TestHandleBacklogV2CreateWithEpic mirrors TestHandleBacklogCreateWithEpic.
func TestHandleBacklogV2CreateWithEpic(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	epic, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "EPIC: demo", "", "", "", "")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P1"), Title: strPtrV2("epic child"), Epic: strPtrV2(epic.ID)},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	item, err := backlog.GetItem(db, "AC-2")
	require.NoError(t, err)
	assert.Equal(t, epic.ID, item.Epic)
}

// TestHandleBacklogV2CreateEpicNotFound_Errors mirrors
// TestHandleBacklogCreateEpicNotFound_Errors.
func TestHandleBacklogV2CreateEpicNotFound_Errors(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P1"), Title: strPtrV2("orphan child"), Epic: strPtrV2("AC-999")},
		},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	assert.Contains(t, mcpx.ResultText(res), `epic item "AC-999" not found`)

	_, err = backlog.GetItem(db, "AC-1")
	assert.Error(t, err)
}

// TestHandleBacklogV2BatchMidBatchError_RollsBackWholeBatch mirrors
// TestHandleBacklogBatchMidBatchError_RollsBackWholeBatch -- the core
// single-transaction guarantee (#132): asserts via side-channel
// backlog.GetItem that NEITHER item persists after a mid-batch failure.
func TestHandleBacklogV2BatchMidBatchError_RollsBackWholeBatch(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P1"), Title: strPtrV2("would-be first item")},
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P1"), Title: strPtrV2("orphan child"), Epic: strPtrV2("AC-999")},
		},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	assert.Contains(t, mcpx.ResultText(res), `epic item "AC-999" not found`)

	// Side-channel assertion: nothing persisted for the WHOLE batch,
	// including the first, otherwise-valid entry.
	_, err = backlog.GetItem(db, "AC-1")
	assert.Error(t, err)
	_, err = backlog.GetItem(db, "AC-2")
	assert.Error(t, err)
}

// TestHandleBacklogV2CreateWithAliasResolvedEpic mirrors
// TestHandleBacklogCreateWithAliasResolvedEpic.
func TestHandleBacklogV2CreateWithAliasResolvedEpic(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	epic, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "EPIC: renamed", "", "", "", "")
	require.NoError(t, err)

	// old_id must be distinct from handlers_backlog_test.go's
	// TestHandleBacklogCreateWithAliasResolvedEpic ("OLD-1") -- resetAllDB
	// does not clear item_id_aliases, and both tests share the same
	// in-memory db within this package.
	_, err = db.Exec("INSERT INTO item_id_aliases (old_id, new_id, renamed_at) VALUES (?, ?, ?)",
		"OLD-V2-1", epic.ID, "2024-01-01T00:00:00Z")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P1"), Title: strPtrV2("epic child via old id"), Epic: strPtrV2("OLD-V2-1")},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
}

// TestHandleBacklogV2UpdateComponent mirrors TestHandleBacklogUpdateComponent.
func TestHandleBacklogV2UpdateComponent(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "task", "", "", "", "")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{ID: strPtrV2(item.ID), Component: strPtrV2("widget")},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	updated, err := backlog.GetItem(db, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "widget", updated.Component)
}

// TestHandleBacklogV2UpdateEpic mirrors TestHandleBacklogUpdateEpic.
func TestHandleBacklogV2UpdateEpic(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	epic, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "EPIC: demo", "", "", "", "")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "task", "", "", "", "")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{ID: strPtrV2(item.ID), Epic: strPtrV2(epic.ID)},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	updated, err := backlog.GetItem(db, item.ID)
	require.NoError(t, err)
	assert.Equal(t, epic.ID, updated.Epic)
}

// TestHandleBacklogV2UpdateEpicNotFound_Errors mirrors
// TestHandleBacklogUpdateEpicNotFound_Errors.
func TestHandleBacklogV2UpdateEpicNotFound_Errors(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "task", "", "", "", "")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{ID: strPtrV2(item.ID), Priority: strPtrV2("P3"), Epic: strPtrV2("AC-999")},
		},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	assert.Contains(t, mcpx.ResultText(res), `epic item "AC-999" not found`)

	unchanged, err := backlog.GetItem(db, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "P1", unchanged.Priority)
	assert.Empty(t, unchanged.Epic)
}

// TestHandleBacklogV2UpdateEpicClear mirrors TestHandleBacklogUpdateEpicClear:
// passing an empty epic on update is allowed (no validation on empty --
// clearing stays a no-op given epic is only set when non-empty, matching the
// component field's convention).
func TestHandleBacklogV2UpdateEpicClear(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "task", "", "", "", "")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{ID: strPtrV2(item.ID), Priority: strPtrV2("P2"), Epic: strPtrV2("")},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	updated, err := backlog.GetItem(db, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "P2", updated.Priority)
	assert.Empty(t, updated.Epic)
}

// TestHandleBacklogV2InvalidPriority mirrors TestHandleBacklogInvalidPriority.
func TestHandleBacklogV2InvalidPriority(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P9"), Title: strPtrV2("Bad priority task")},
		},
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)
}

// TestHandleBacklogV2NonexistentProject mirrors TestHandleBacklogNonexistentProject.
func TestHandleBacklogV2NonexistentProject(t *testing.T) {
	resetAllDB(t)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{Project: strPtrV2("nonexistent-project"), Priority: strPtrV2("P1"), Title: strPtrV2("Orphan task")},
		},
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)
}

// TestHandleBacklogV2DescriptionTooLong mirrors TestHandleBacklogDescriptionTooLong.
func TestHandleBacklogV2DescriptionTooLong(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	longDesc := strings.Repeat("a", 502)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P1"), Title: strPtrV2("Long desc task"), Description: strPtrV2(longDesc)},
		},
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Contains(t, mcpx.ResultText(res), "description exceeds 500 char hard cap")
}

// TestHandleBacklogV2UpdateInvalidPriority mirrors TestHandleBacklogUpdateInvalidPriority.
func TestHandleBacklogV2UpdateInvalidPriority(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Original task", "", "", "", "")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{ID: strPtrV2(item.ID), Priority: strPtrV2("INVALID")},
		},
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)
}

// TestHandleBacklogV2UpdateNonexistentID_Errors verifies UpdateItemTx's
// error (item not found) propagates and rolls back the batch -- covers the
// update path's UpdateItemTx/GetItem error branch, which no other test here
// exercises (a bad epic/priority errors earlier in the same branch).
func TestHandleBacklogV2UpdateNonexistentID_Errors(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{ID: strPtrV2("AC-999"), Priority: strPtrV2("P1")},
		},
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)
}

// TestHandleBacklogV2UpdateEpicBackrefMalformed_Errors verifies a malformed
// "$N" epic back-reference errors in the UPDATE path specifically (the
// existing malformed-backref tests all use the create path) -- covers the
// update branch's resolveEpicRef error.
func TestHandleBacklogV2UpdateEpicBackrefMalformed_Errors(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "task", "", "", "", "")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{ID: strPtrV2(item.ID), Epic: strPtrV2("$x")},
		},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	assert.Contains(t, mcpx.ResultText(res), "invalid epic back-reference")
}

// TestHandleBacklogV2UpdateInlineEdgeMissingTarget_Errors mirrors
// TestHandleBacklogUpdateInlineEdgeMissingTarget_Errors: a failing edges[]
// target on an UPDATE entry rolls back the field update too, not just the
// (nonexistent) item create -- covers the update path's linkEdgesTx error
// branch.
func TestHandleBacklogV2UpdateInlineEdgeMissingTarget_Errors(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	a, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item a", "", "", "", "")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{ID: strPtrV2(a.ID), Priority: strPtrV2("P0"), Edges: []edgeInput{{Label: "blocks", Target: "TM-999"}}},
		},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)

	unchanged, err := backlog.GetItem(db, a.ID)
	require.NoError(t, err)
	assert.Equal(t, "P1", unchanged.Priority, "a failing edges[] target must roll back the field update too")
}

// TestHandleBacklogV2BatchCreate mirrors TestHandleBacklogBatchCreate.
func TestHandleBacklogV2BatchCreate(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P0"), Title: strPtrV2("Batch task 1")},
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P1"), Title: strPtrV2("Batch task 2")},
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P2"), Title: strPtrV2("Batch task 3")},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	var resp []backlogResult
	require.NoError(t, jsonUnmarshalText(res, &resp))
	require.Len(t, resp, 3)
	for _, r := range resp {
		assert.Equal(t, "create", r.Action)
	}
}

// TestHandleBacklogV2BatchCreateSequentialIDs mirrors
// TestHandleBacklogBatchCreateSequentialIDs.
func TestHandleBacklogV2BatchCreateSequentialIDs(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P0"), Title: strPtrV2("first")},
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P1"), Title: strPtrV2("second")},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	var resp []backlogResult
	require.NoError(t, jsonUnmarshalText(res, &resp))
	require.Len(t, resp, 2)
	assert.Equal(t, "AC-1", resp[0].ID)
	assert.Equal(t, "AC-2", resp[1].ID)
}

// ============ Silent-skip semantics ============

// TestHandleBacklogV2Create_MissingRequiredFields_SilentlySkipped verifies a
// create entry missing project/priority/title is silently skipped (no error,
// no result entry) -- matches handlers_backlog.go's create gate.
func TestHandleBacklogV2Create_MissingRequiredFields_SilentlySkipped(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{}, // no id, no project/priority/title -- silently skipped
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P1"), Title: strPtrV2("real one")},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	var resp []backlogResult
	require.NoError(t, jsonUnmarshalText(res, &resp))
	require.Len(t, resp, 1)
	assert.Equal(t, "create", resp[0].Action)
}

// TestHandleBacklogV2Update_NoFieldsNoEdges_SilentlySkipped verifies an
// update entry (id present) with no fields and no edges is silently skipped
// (no error, no result entry) -- matches handlers_backlog.go's update gate.
func TestHandleBacklogV2Update_NoFieldsNoEdges_SilentlySkipped(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "task", "", "", "", "")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{ID: strPtrV2(item.ID)},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	var resp []backlogResult
	require.NoError(t, jsonUnmarshalText(res, &resp))
	assert.Empty(t, resp)
}

// ============ Inline edges ============

// TestHandleBacklogV2CreateWithInlineEdges mirrors
// TestHandleBacklogCreateWithInlineEdges.
func TestHandleBacklogV2CreateWithInlineEdges(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	target, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "target", "", "", "", "")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{
				Project:  strPtrV2("acme-corp"),
				Priority: strPtrV2("P1"),
				Title:    strPtrV2("source"),
				Edges:    []edgeInput{{Label: "blocks", Target: target.ID}},
			},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	list, err := edges.EdgesFor(db, edges.TypeItem, "AC-2")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "blocks", list[0].Label)
	assert.Equal(t, target.ID, list[0].TargetID)
}

// TestHandleBacklogV2UpdateWithInlineEdges mirrors
// TestHandleBacklogUpdateWithInlineEdges.
func TestHandleBacklogV2UpdateWithInlineEdges(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	a, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "a", "", "", "", "")
	require.NoError(t, err)
	b, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "b", "", "", "", "")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{ID: strPtrV2(a.ID), Edges: []edgeInput{{Label: "relates", Target: b.ID}}},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	list, err := edges.EdgesFor(db, edges.TypeItem, a.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "relates", list[0].Label)
}

// TestHandleBacklogV2InlineEdgeMissingLabelOrTarget_Errors mirrors
// parseEdgeSpecs' missing-label/target hard-error, ported to buildEdgeSpecsV2.
func TestHandleBacklogV2InlineEdgeMissingLabelOrTarget_Errors(t *testing.T) {
	_, err := buildEdgeSpecsV2([]edgeInput{{Label: "blocks"}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "edges[] entry requires label and target")

	_, err = buildEdgeSpecsV2([]edgeInput{{Target: "AC-1"}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "edges[] entry requires label and target")
}

// TestHandleBacklogV2InlineEdgeNoEdgesKey verifies an absent/empty Edges
// slice returns (nil, nil), mirroring TestParseEdgeSpecsNoEdgesKey.
func TestHandleBacklogV2InlineEdgeNoEdgesKey(t *testing.T) {
	specs, err := buildEdgeSpecsV2(nil)
	require.NoError(t, err)
	assert.Nil(t, specs)
}

// TestHandleBacklogV2InlineEdgeInvalidLabel_Errors mirrors
// TestHandleBacklogInlineEdgeInvalidLabel_Errors.
func TestHandleBacklogV2InlineEdgeInvalidLabel_Errors(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{
				Project:  strPtrV2("acme-corp"),
				Priority: strPtrV2("P1"),
				Title:    strPtrV2("x"),
				Edges:    []edgeInput{{Label: "part-of", Target: "AC-1"}},
			},
		},
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Contains(t, mcpx.ResultText(res), "invalid edge label")
}

// TestHandleBacklogV2InlineEdgeMissingTarget_Errors mirrors
// TestHandleBacklogInlineEdgeMissingTarget_Errors: a typo'd/missing
// edges[].target rolls back the whole entry, including the item create.
func TestHandleBacklogV2InlineEdgeMissingTarget_Errors(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{
				Project:  strPtrV2("acme-corp"),
				Priority: strPtrV2("P1"),
				Title:    strPtrV2("blocked item"),
				Edges:    []edgeInput{{Label: "blocks", Target: "AC-999"}},
			},
		},
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Contains(t, mcpx.ResultText(res), `edge target item "AC-999" not found`)

	items, err := backlog.ListItems(db, backlog.ItemFilter{})
	require.NoError(t, err)
	assert.Empty(t, items, "a failing edges[] target must roll back the whole entry, including the item create")
}

// ============ $N intra-batch epic back-reference ============

// TestHandleBacklogV2BatchEpicBackref_SameBatchCreate mirrors
// TestHandleBacklogBatchEpicBackref_SameBatchCreate.
func TestHandleBacklogV2BatchEpicBackref_SameBatchCreate(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P1"), Title: strPtrV2("EPIC: X")},
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P2"), Title: strPtrV2("child 1"), Epic: strPtrV2("$0")},
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P2"), Title: strPtrV2("child 2"), Epic: strPtrV2("$0")},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	var resp []backlogResult
	require.NoError(t, jsonUnmarshalText(res, &resp))
	require.Len(t, resp, 3)
	epicID := resp[0].ID

	child1, err := backlog.GetItem(db, resp[1].ID)
	require.NoError(t, err)
	assert.Equal(t, epicID, child1.Epic)

	child2, err := backlog.GetItem(db, resp[2].ID)
	require.NoError(t, err)
	assert.Equal(t, epicID, child2.Epic)
}

// TestHandleBacklogV2BatchEpicBackref_UpdateReparent mirrors
// TestHandleBacklogBatchEpicBackref_UpdateReparent.
func TestHandleBacklogV2BatchEpicBackref_UpdateReparent(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	existing, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P2", "pre-existing task", "", "", "", "")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P1"), Title: strPtrV2("EPIC: Y")},
			{ID: strPtrV2(existing.ID), Epic: strPtrV2("$0")},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	var resp []backlogResult
	require.NoError(t, jsonUnmarshalText(res, &resp))
	require.Len(t, resp, 2)
	epicID := resp[0].ID

	reparented, err := backlog.GetItem(db, existing.ID)
	require.NoError(t, err)
	assert.Equal(t, epicID, reparented.Epic)
}

// TestHandleBacklogV2BatchEpicBackref_ForwardRef_Errors mirrors
// TestHandleBacklogBatchEpicBackref_ForwardRef_Errors.
func TestHandleBacklogV2BatchEpicBackref_ForwardRef_Errors(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P2"), Title: strPtrV2("child (forward ref)"), Epic: strPtrV2("$1")},
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P1"), Title: strPtrV2("EPIC: later")},
		},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	assert.Contains(t, mcpx.ResultText(res), "must point to an earlier entry")

	_, err = backlog.GetItem(db, "AC-1")
	assert.Error(t, err)
	_, err = backlog.GetItem(db, "AC-2")
	assert.Error(t, err)
}

// TestHandleBacklogV2BatchEpicBackref_SelfRef_Errors mirrors
// TestHandleBacklogBatchEpicBackref_SelfRef_Errors.
func TestHandleBacklogV2BatchEpicBackref_SelfRef_Errors(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P1"), Title: strPtrV2("self-ref"), Epic: strPtrV2("$0")},
		},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	assert.Contains(t, mcpx.ResultText(res), "must point to an earlier entry")
}

// TestHandleBacklogV2BatchEpicBackref_OutOfRange_Errors mirrors
// TestHandleBacklogBatchEpicBackref_OutOfRange_Errors.
func TestHandleBacklogV2BatchEpicBackref_OutOfRange_Errors(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P1"), Title: strPtrV2("EPIC: A")},
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P2"), Title: strPtrV2("out of range ref"), Epic: strPtrV2("$5")},
		},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	assert.Contains(t, mcpx.ResultText(res), "out of range")
}

// TestHandleBacklogV2BatchEpicBackref_Malformed_Errors mirrors
// TestHandleBacklogBatchEpicBackref_Malformed_Errors.
func TestHandleBacklogV2BatchEpicBackref_Malformed_Errors(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	for _, bad := range []string{"$", "$x"} {
		res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
			Entries: []backlogEntryInput{
				{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P1"), Title: strPtrV2("malformed ref " + bad), Epic: strPtrV2(bad)},
			},
		})
		require.NoError(t, err)
		require.True(t, res.IsError)
		assert.Contains(t, mcpx.ResultText(res), "invalid epic back-reference")
	}
}

// TestHandleBacklogV2BatchEpicBackref_UnresolvedPosition_Errors mirrors
// TestHandleBacklogBatchEpicBackref_UnresolvedPosition_Errors: a "$N" that's
// in-range and precedes the referencing entry, but whose target entry
// produced no item (silently skipped), is rejected.
func TestHandleBacklogV2BatchEpicBackref_UnresolvedPosition_Errors(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{
		Entries: []backlogEntryInput{
			{}, // malformed: no id, no project/priority/title -- silently skipped
			{Project: strPtrV2("acme-corp"), Priority: strPtrV2("P2"), Title: strPtrV2("dangling ref"), Epic: strPtrV2("$0")},
		},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	assert.Contains(t, mcpx.ResultText(res), "refers to an entry that produced no item")
}

// TestHandleBacklogV2NoEntriesOrDeleteIDs_Errors mirrors
// TestHandleBacklogNoEntriesOrDeleteIDs_Errors.
func TestHandleBacklogV2NoEntriesOrDeleteIDs_Errors(t *testing.T) {
	resetAllDB(t)

	res, _, err := handleBacklogV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, backlogInput{})
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Equal(t, errBacklogEntriesOrDeleteIDsRequired, mcpx.ResultText(res))
}
