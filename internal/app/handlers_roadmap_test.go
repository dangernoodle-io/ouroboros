package app

import (
	"context"
	"database/sql"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/edges"
	"dangernoodle.io/ouroboros/internal/roadmap"
	"dangernoodle.io/ouroboros/internal/store"
)

// TestHandleRoadmap_Add verifies op=add appends an item and returns its id/section.
func TestHandleRoadmap_Add(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"op":             "add",
		"project":        "acme-corp",
		"section":        "now",
		"title":          "Ship the widget",
		"body":           "Body text",
		"component":      "widget",
		"why":            "priority shift",
		"resume_trigger": "n/a",
		"kb":             []interface{}{float64(1), float64(2)},
		"ticket":         []interface{}{"TK-1"},
		"blocked_by": []interface{}{
			map[string]interface{}{"project": "other-repo", "ref": "OR-9", "note": "waiting"},
		},
	})

	result, err := handleRoadmap(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var resp map[string]interface{}
	require.NoError(t, unmarshalResult(result, &resp))
	assert.Equal(t, float64(1), resp["id"])
	assert.Equal(t, "now", resp["section"])

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 1)
	item := rm.Sections.Now[0]
	assert.Equal(t, "Ship the widget", item.Title)
	assert.Equal(t, "widget", item.Component)
	assert.Equal(t, []int{1, 2}, item.KB)
	assert.Equal(t, []string{"TK-1"}, item.Ticket)
	require.Len(t, item.BlockedBy, 1)
	assert.Equal(t, "other-repo", item.BlockedBy[0].Project)
}

// TestHandleRoadmap_Add_InvalidSection verifies op=add rejects an unknown section.
func TestHandleRoadmap_Add_InvalidSection(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"op":      "add",
		"project": "acme-corp",
		"section": "bogus",
		"title":   "x",
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleRoadmap_Add_BlockedByMissingProject verifies op=add rejects a
// blocked_by entry with an empty project.
func TestHandleRoadmap_Add_BlockedByMissingProject(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "x",
		"blocked_by": []interface{}{
			map[string]interface{}{"project": "", "ref": "OR-9"},
		},
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.True(t, result.IsError)
	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "missing project")
}

// TestHandleRoadmap_Add_BlockedByMissingRef verifies op=add rejects a
// blocked_by entry with an empty ref.
func TestHandleRoadmap_Add_BlockedByMissingRef(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "x",
		"blocked_by": []interface{}{
			map[string]interface{}{"project": "other-repo", "ref": ""},
		},
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.True(t, result.IsError)
	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "missing ref")
}

// TestHandleRoadmap_Update_BlockedByMissingProject verifies op=update rejects
// a blocked_by entry with an empty project.
func TestHandleRoadmap_Update_BlockedByMissingProject(t *testing.T) {
	resetDB(t)

	addReq := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "x",
	})
	_, err := handleRoadmap(db, nil)(context.TODO(), addReq)
	require.NoError(t, err)

	updateReq := makeRequest(map[string]interface{}{
		"op": "update", "project": "acme-corp", "id": float64(1),
		"blocked_by": []interface{}{
			map[string]interface{}{"project": "", "ref": "OR-9"},
		},
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), updateReq)
	require.NoError(t, err)
	require.True(t, result.IsError)
	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "missing project")
}

// TestHandleRoadmap_Update verifies op=update patches only the provided fields.
func TestHandleRoadmap_Update(t *testing.T) {
	resetDB(t)

	addReq := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "Original", "body": "orig body",
	})
	_, err := handleRoadmap(db, nil)(context.TODO(), addReq)
	require.NoError(t, err)

	updateReq := makeRequest(map[string]interface{}{
		"op": "update", "project": "acme-corp", "id": float64(1), "title": "Updated title",
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), updateReq)
	require.NoError(t, err)
	require.False(t, result.IsError)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 1)
	assert.Equal(t, "Updated title", rm.Sections.Now[0].Title)
	assert.Equal(t, "orig body", rm.Sections.Now[0].Body, "body should be unchanged (nil patch field)")
}

// TestHandleRoadmap_Update_SliceFields verifies op=update patches kb/ticket/blocked_by.
func TestHandleRoadmap_Update_SliceFields(t *testing.T) {
	resetDB(t)

	addReq := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "Original",
	})
	_, err := handleRoadmap(db, nil)(context.TODO(), addReq)
	require.NoError(t, err)

	updateReq := makeRequest(map[string]interface{}{
		"op": "update", "project": "acme-corp", "id": float64(1),
		"kb":     []interface{}{float64(7)},
		"ticket": []interface{}{"TK-9"},
		"blocked_by": []interface{}{
			map[string]interface{}{"project": "other-repo", "ref": "OR-1"},
		},
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), updateReq)
	require.NoError(t, err)
	require.False(t, result.IsError)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 1)
	item := rm.Sections.Now[0]
	assert.Equal(t, []int{7}, item.KB)
	assert.Equal(t, []string{"TK-9"}, item.Ticket)
	require.Len(t, item.BlockedBy, 1)
	assert.Equal(t, "other-repo", item.BlockedBy[0].Project)
}

// TestHandleRoadmap_Update_NotFound verifies op=update on an unknown id errors.
func TestHandleRoadmap_Update_NotFound(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"op": "update", "project": "acme-corp", "id": float64(999), "title": "x",
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleRoadmap_Move verifies op=move relocates an item to another section.
func TestHandleRoadmap_Move(t *testing.T) {
	resetDB(t)

	addReq := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "Move me",
	})
	_, err := handleRoadmap(db, nil)(context.TODO(), addReq)
	require.NoError(t, err)

	moveReq := makeRequest(map[string]interface{}{
		"op": "move", "project": "acme-corp", "id": float64(1), "to": "parked",
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), moveReq)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var resp map[string]interface{}
	require.NoError(t, unmarshalResult(result, &resp))
	assert.Equal(t, "parked", resp["section"])

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	assert.Empty(t, rm.Sections.Now)
	require.Len(t, rm.Sections.Parked, 1)
}

// TestHandleRoadmap_Move_InvalidSection verifies op=move rejects an unknown target section.
func TestHandleRoadmap_Move_InvalidSection(t *testing.T) {
	resetDB(t)

	addReq := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "x",
	})
	_, err := handleRoadmap(db, nil)(context.TODO(), addReq)
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"op": "move", "project": "acme-corp", "id": float64(1), "to": "bogus",
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleRoadmap_Done verifies op=done moves an item to the done section.
func TestHandleRoadmap_Done(t *testing.T) {
	resetDB(t)

	addReq := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "Finish me",
	})
	_, err := handleRoadmap(db, nil)(context.TODO(), addReq)
	require.NoError(t, err)

	doneReq := makeRequest(map[string]interface{}{
		"op": "done", "project": "acme-corp", "id": float64(1),
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), doneReq)
	require.NoError(t, err)
	require.False(t, result.IsError)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Done, 1)
}

// TestHandleRoadmap_Remove verifies op=remove deletes an item entirely.
func TestHandleRoadmap_Remove(t *testing.T) {
	resetDB(t)

	addReq := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "Delete me",
	})
	_, err := handleRoadmap(db, nil)(context.TODO(), addReq)
	require.NoError(t, err)

	removeReq := makeRequest(map[string]interface{}{
		"op": "remove", "project": "acme-corp", "id": float64(1),
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), removeReq)
	require.NoError(t, err)
	require.False(t, result.IsError)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	assert.Empty(t, rm.Sections.Now)
	assert.Empty(t, rm.Sections.Next)
	assert.Empty(t, rm.Sections.Parked)
	assert.Empty(t, rm.Sections.Done)
}

// TestHandleRoadmap_MissingProject_Errors verifies project is required for every op.
func TestHandleRoadmap_MissingProject_Errors(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{"op": "add", "section": "now", "title": "x"})
	result, err := handleRoadmap(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleRoadmap_InvalidOp_Errors verifies an unrecognized op errors.
func TestHandleRoadmap_InvalidOp_Errors(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{"op": "bogus", "project": "acme-corp"})
	result, err := handleRoadmap(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleRoadmap_MissingID_Errors verifies update/move/done/remove require id.
func TestHandleRoadmap_MissingID_Errors(t *testing.T) {
	resetDB(t)

	for _, op := range []string{"update", "move", "done", "remove"} {
		req := makeRequest(map[string]interface{}{"op": op, "project": "acme-corp"})
		result, err := handleRoadmap(db, nil)(context.TODO(), req)
		require.NoError(t, err)
		assert.True(t, result.IsError, "op=%s should require id", op)
	}
}

// TestHandleRoadmap_Singleton verifies two adds to the same project produce one documents row.
func TestHandleRoadmap_Singleton(t *testing.T) {
	resetDB(t)

	for i := 0; i < 2; i++ {
		req := makeRequest(map[string]interface{}{
			"op": "add", "project": "acme-corp", "section": "now", "title": "item",
		})
		_, err := handleRoadmap(db, nil)(context.TODO(), req)
		require.NoError(t, err)
	}

	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM documents WHERE type='roadmap' AND project='acme-corp'`).Scan(&count))
	assert.Equal(t, 1, count)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	assert.Len(t, rm.Sections.Now, 2)
}

// TestHandleRoadmap_Add_BlockedByNotObject verifies op=add rejects a
// blocked_by entry that isn't an object.
func TestHandleRoadmap_Add_BlockedByNotObject(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "x",
		"blocked_by": []interface{}{"not-an-object"},
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.True(t, result.IsError)
	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "must be an object")
}

// TestHandleRoadmap_Add_MutateError verifies op=add surfaces a Mutate failure
// (e.g. a broken store) as a tool error rather than a Go error.
func TestHandleRoadmap_Add_MutateError(t *testing.T) {
	brokenDB, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer brokenDB.Close()
	require.NoError(t, store.ApplySchema(brokenDB))
	_, err = brokenDB.Exec("DROP TABLE documents")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "x",
	})
	result, callErr := handleRoadmap(brokenDB, nil)(context.TODO(), req)
	require.NoError(t, callErr)
	assert.True(t, result.IsError)
}

// TestHandleRoadmap_Move_NotFound verifies op=move on an unknown id errors.
func TestHandleRoadmap_Move_NotFound(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"op": "move", "project": "acme-corp", "id": float64(9999), "to": "parked",
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleRoadmap_Done_NotFound verifies op=done on an unknown id errors.
func TestHandleRoadmap_Done_NotFound(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"op": "done", "project": "acme-corp", "id": float64(9999),
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleRoadmap_Reorder verifies op=reorder physically splices the item
// to the target index and densely renumbers its section.
func TestHandleRoadmap_Reorder(t *testing.T) {
	resetDB(t)

	addReq1 := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "stays first",
	})
	_, err := handleRoadmap(db, nil)(context.TODO(), addReq1)
	require.NoError(t, err)
	addReq2 := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "reorder me",
	})
	_, err = handleRoadmap(db, nil)(context.TODO(), addReq2)
	require.NoError(t, err)

	// Target index 4 clamps to the end (index 1, after removal) of a
	// 2-item section, so "reorder me" moves to last, densely renumbered.
	reorderReq := makeRequest(map[string]interface{}{
		"op": "reorder", "project": "acme-corp", "id": float64(2), "position": float64(4),
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), reorderReq)
	require.NoError(t, err)
	require.False(t, result.IsError)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 2)
	assert.Equal(t, "stays first", rm.Sections.Now[0].Title)
	assert.Equal(t, 1, rm.Sections.Now[0].Position)
	assert.Equal(t, "reorder me", rm.Sections.Now[1].Title)
	assert.Equal(t, 2, rm.Sections.Now[1].Position)
}

// TestHandleRoadmap_Reorder_MissingPosition verifies op=reorder requires position.
func TestHandleRoadmap_Reorder_MissingPosition(t *testing.T) {
	resetDB(t)

	addReq := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "x",
	})
	_, err := handleRoadmap(db, nil)(context.TODO(), addReq)
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"op": "reorder", "project": "acme-corp", "id": float64(1),
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleRoadmap_Reorder_MissingID verifies op=reorder requires id.
func TestHandleRoadmap_Reorder_MissingID(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"op": "reorder", "project": "acme-corp", "position": float64(1),
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleRoadmap_Reorder_NotFound verifies op=reorder on an unknown id errors.
func TestHandleRoadmap_Reorder_NotFound(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"op": "reorder", "project": "acme-corp", "id": float64(9999), "position": float64(1),
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleRoadmap_Add_EpicAndPosition verifies op=add wires epic/position
// onto the item: position 0 into a NON-empty section splices the new item
// to the front, mirroring op=reorder's index semantics.
func TestHandleRoadmap_Add_EpicAndPosition(t *testing.T) {
	resetDB(t)

	existingReq := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "existing",
	})
	_, err := handleRoadmap(db, nil)(context.TODO(), existingReq)
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "x",
		"epic": "BB-707", "position": float64(0),
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 2)
	assert.Equal(t, "x", rm.Sections.Now[0].Title, "position 0 splices the new item to the front")
	assert.Equal(t, "BB-707", rm.Sections.Now[0].Epic)
	assert.Equal(t, 1, rm.Sections.Now[0].Position)
	assert.Equal(t, "existing", rm.Sections.Now[1].Title)
	assert.Equal(t, 2, rm.Sections.Now[1].Position)
}

// TestHandleRoadmap_Add_NoPosition_AppendsLast verifies op=add with no
// position given plain-appends (item.Position is never an ordering input).
func TestHandleRoadmap_Add_NoPosition_AppendsLast(t *testing.T) {
	resetDB(t)

	firstReq := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "first",
	})
	_, err := handleRoadmap(db, nil)(context.TODO(), firstReq)
	require.NoError(t, err)

	secondReq := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "second",
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), secondReq)
	require.NoError(t, err)
	require.False(t, result.IsError)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 2)
	assert.Equal(t, "first", rm.Sections.Now[0].Title)
	assert.Equal(t, "second", rm.Sections.Now[1].Title)
	assert.Zero(t, rm.Sections.Now[0].Position, "section stays legacy — no position was ever given")
}

// TestHandleRoadmap_Add_ComponentNotScalar_Errors verifies op=add rejects a
// non-string component (single-valued enforcement).
func TestHandleRoadmap_Add_ComponentNotScalar_Errors(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "x",
		"component": []interface{}{"a", "b"},
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.True(t, result.IsError)
	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "single-valued")
}

// TestHandleRoadmap_Add_EpicNotScalar_Errors verifies op=add rejects a
// non-string epic (single-valued enforcement).
func TestHandleRoadmap_Add_EpicNotScalar_Errors(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "x",
		"epic": []interface{}{"BB-1", "BB-2"},
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.True(t, result.IsError)
	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "single-valued")
}

// TestHandleRoadmap_Update_Epic verifies op=update patches epic.
func TestHandleRoadmap_Update_Epic(t *testing.T) {
	resetDB(t)

	addReq := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "x",
	})
	_, err := handleRoadmap(db, nil)(context.TODO(), addReq)
	require.NoError(t, err)

	updateReq := makeRequest(map[string]interface{}{
		"op": "update", "project": "acme-corp", "id": float64(1), "epic": "BB-707",
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), updateReq)
	require.NoError(t, err)
	require.False(t, result.IsError)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 1)
	assert.Equal(t, "BB-707", rm.Sections.Now[0].Epic)
}

// TestHandleRoadmap_Update_EpicNotScalar_Errors verifies op=update rejects a
// non-string epic (single-valued enforcement).
func TestHandleRoadmap_Update_EpicNotScalar_Errors(t *testing.T) {
	resetDB(t)

	addReq := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "x",
	})
	_, err := handleRoadmap(db, nil)(context.TODO(), addReq)
	require.NoError(t, err)

	updateReq := makeRequest(map[string]interface{}{
		"op": "update", "project": "acme-corp", "id": float64(1),
		"epic": []interface{}{"BB-1", "BB-2"},
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), updateReq)
	require.NoError(t, err)
	require.True(t, result.IsError)
	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "single-valued")
}

// TestHandleRoadmap_Move_WithPosition verifies op=move honors an optional
// position: a lone item landing in an empty target section always clamps
// to index 0, densely renumbered to Position 1.
func TestHandleRoadmap_Move_WithPosition(t *testing.T) {
	resetDB(t)

	addReq := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "Move me",
	})
	_, err := handleRoadmap(db, nil)(context.TODO(), addReq)
	require.NoError(t, err)

	moveReq := makeRequest(map[string]interface{}{
		"op": "move", "project": "acme-corp", "id": float64(1), "to": "deferred", "position": float64(6),
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), moveReq)
	require.NoError(t, err)
	require.False(t, result.IsError)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Deferred, 1)
	assert.Equal(t, 1, rm.Sections.Deferred[0].Position)
}

// TestHandleRoadmap_Add_DeferredAndDropped verifies op=add accepts the new sections.
func TestHandleRoadmap_Add_DeferredAndDropped(t *testing.T) {
	resetDB(t)

	for _, section := range []string{"deferred", "dropped"} {
		req := makeRequest(map[string]interface{}{
			"op": "add", "project": "acme-corp", "section": section, "title": "item " + section,
		})
		result, err := handleRoadmap(db, nil)(context.TODO(), req)
		require.NoError(t, err)
		require.False(t, result.IsError, "section=%s", section)
	}

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	assert.Len(t, rm.Sections.Deferred, 1)
	assert.Len(t, rm.Sections.Dropped, 1)
}

// TestHandleRoadmap_Remove_NotFound verifies op=remove on an unknown id errors.
func TestHandleRoadmap_Remove_NotFound(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"op": "remove", "project": "acme-corp", "id": float64(9999),
	})
	result, err := handleRoadmap(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// ---- get/search domain=roadmap ----

// TestHandleGet_DomainRoadmap_Structured verifies get domain=roadmap returns the
// structured Roadmap JSON by default.
func TestHandleGet_DomainRoadmap_Structured(t *testing.T) {
	resetDB(t)

	addReq := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "Structured item",
	})
	_, err := handleRoadmap(db, nil)(context.TODO(), addReq)
	require.NoError(t, err)

	getReq := makeRequest(map[string]interface{}{
		"domain": "roadmap", "projects": []interface{}{"acme-corp"},
	})
	result, err := handleGet(db)(context.TODO(), getReq)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var rm roadmap.Roadmap
	require.NoError(t, unmarshalResult(result, &rm))
	require.Len(t, rm.Sections.Now, 1)
	assert.Equal(t, "Structured item", rm.Sections.Now[0].Title)
}

// TestHandleGet_DomainRoadmap_Markdown verifies get domain=roadmap format=md renders Markdown.
func TestHandleGet_DomainRoadmap_Markdown(t *testing.T) {
	resetDB(t)

	addReq := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "MD item",
	})
	_, err := handleRoadmap(db, nil)(context.TODO(), addReq)
	require.NoError(t, err)

	getReq := makeRequest(map[string]interface{}{
		"domain": "roadmap", "projects": []interface{}{"acme-corp"}, "format": "md",
	})
	result, err := handleGet(db)(context.TODO(), getReq)
	require.NoError(t, err)
	require.False(t, result.IsError)

	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "# Roadmap")
	assert.Contains(t, textContent.Text, "MD item")
}

// TestHandleGet_DomainRoadmap_ByEpicResolvesLabel verifies get domain=roadmap
// format=md by=epic groups by epic and resolves the epic's backlog title,
// stripping a leading "EPIC:" prefix.
func TestHandleGet_DomainRoadmap_ByEpicResolvesLabel(t *testing.T) {
	resetDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	epicItem, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "EPIC: WiFi map", "", "", "", "")
	require.NoError(t, err)

	addReq := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "epic item",
		"epic": epicItem.ID,
	})
	_, err = handleRoadmap(db, nil)(context.TODO(), addReq)
	require.NoError(t, err)

	getReq := makeRequest(map[string]interface{}{
		"domain": "roadmap", "projects": []interface{}{"acme-corp"}, "format": "md", "by": "epic",
	})
	result, err := handleGet(db)(context.TODO(), getReq)
	require.NoError(t, err)
	require.False(t, result.IsError)

	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "#### epic: WiFi map")
}

// TestHandleGet_DomainRoadmap_HTML verifies get domain=roadmap format=html
// returns a self-contained HTML fragment.
func TestHandleGet_DomainRoadmap_HTML(t *testing.T) {
	resetDB(t)

	addReq := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "HTML item",
		"component": "widget",
	})
	_, err := handleRoadmap(db, nil)(context.TODO(), addReq)
	require.NoError(t, err)

	getReq := makeRequest(map[string]interface{}{
		"domain": "roadmap", "projects": []interface{}{"acme-corp"}, "format": "html",
	})
	result, err := handleGet(db)(context.TODO(), getReq)
	require.NoError(t, err)
	require.False(t, result.IsError)

	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "<style>")
	assert.Contains(t, textContent.Text, "HTML item")
	assert.Contains(t, textContent.Text, "component: widget")
}

// TestHandleGet_DomainRoadmap_HTMLByEpicMarksBlockedHeader verifies
// format=html&by=epic resolves the epic's backlog title and marks the
// group header blocked when an incoming "blocks" edge targets the epic
// item, computed from internal/edges.
func TestHandleGet_DomainRoadmap_HTMLByEpicMarksBlockedHeader(t *testing.T) {
	resetDB(t)

	proj, err := backlog.GetProjectByName(db, "acme-corp")
	require.NoError(t, err)
	if proj == nil {
		proj, err = backlog.CreateProject(db, "acme-corp", "AC")
		require.NoError(t, err)
	}
	epicItem, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "EPIC: WiFi map", "", "", "", "")
	require.NoError(t, err)
	blocker, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "blocking item", "", "", "", "")
	require.NoError(t, err)
	_, err = edges.Link(db, edges.TypeItem, blocker.ID, "blocks", edges.TypeItem, epicItem.ID, proj.ID)
	require.NoError(t, err)

	addReq := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "epic item",
		"epic": epicItem.ID,
	})
	_, err = handleRoadmap(db, nil)(context.TODO(), addReq)
	require.NoError(t, err)

	getReq := makeRequest(map[string]interface{}{
		"domain": "roadmap", "projects": []interface{}{"acme-corp"}, "format": "html", "by": "epic",
	})
	result, err := handleGet(db)(context.TODO(), getReq)
	require.NoError(t, err)
	require.False(t, result.IsError)

	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "epic: WiFi map")
	assert.Contains(t, textContent.Text, `class="blocked-marker"`)
}

// TestHandleGet_DomainRoadmap_HTMLFiltersApply verifies component/epic
// filters still apply under format=html.
func TestHandleGet_DomainRoadmap_HTMLFiltersApply(t *testing.T) {
	resetDB(t)

	addReq1 := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "matches",
		"component": "widget",
	})
	_, err := handleRoadmap(db, nil)(context.TODO(), addReq1)
	require.NoError(t, err)

	addReq2 := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "other",
		"component": "gadget",
	})
	_, err = handleRoadmap(db, nil)(context.TODO(), addReq2)
	require.NoError(t, err)

	getReq := makeRequest(map[string]interface{}{
		"domain": "roadmap", "projects": []interface{}{"acme-corp"}, "format": "html", "component": "widget",
	})
	result, err := handleGet(db)(context.TODO(), getReq)
	require.NoError(t, err)
	require.False(t, result.IsError)

	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "matches")
	assert.NotContains(t, textContent.Text, "other")
}

// TestHandleGet_DomainRoadmap_ComponentAndEpicFilters verifies get
// domain=roadmap filters items by component and epic (structured output).
func TestHandleGet_DomainRoadmap_ComponentAndEpicFilters(t *testing.T) {
	resetDB(t)

	addReq1 := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "matches",
		"component": "widget", "epic": "BB-707",
	})
	_, err := handleRoadmap(db, nil)(context.TODO(), addReq1)
	require.NoError(t, err)

	addReq2 := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "other",
		"component": "gadget",
	})
	_, err = handleRoadmap(db, nil)(context.TODO(), addReq2)
	require.NoError(t, err)

	getReq := makeRequest(map[string]interface{}{
		"domain": "roadmap", "projects": []interface{}{"acme-corp"}, "component": "widget", "epic": "BB-707",
	})
	result, err := handleGet(db)(context.TODO(), getReq)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var rm roadmap.Roadmap
	require.NoError(t, unmarshalResult(result, &rm))
	require.Len(t, rm.Sections.Now, 1)
	assert.Equal(t, "matches", rm.Sections.Now[0].Title)
}

// TestHandleGet_DomainRoadmap_MissingProject_Errors verifies get domain=roadmap requires projects[0].
func TestHandleGet_DomainRoadmap_MissingProject_Errors(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{"domain": "roadmap"})
	result, err := handleGet(db)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleSearch_DomainRoadmap verifies search domain=roadmap finds roadmap docs by FTS.
func TestHandleSearch_DomainRoadmap(t *testing.T) {
	resetDB(t)

	addReq := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "Unique searchable widget",
	})
	_, err := handleRoadmap(db, nil)(context.TODO(), addReq)
	require.NoError(t, err)

	searchReq := makeRequest(map[string]interface{}{
		"domain": "roadmap", "query": "widget",
	})
	result, err := handleSearch(db)(context.TODO(), searchReq)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var resp []map[string]interface{}
	require.NoError(t, unmarshalResult(result, &resp))
	require.Len(t, resp, 1)
	assert.Equal(t, "roadmap", resp[0]["type"])
}

// TestHandleSearch_DomainRoadmap_MissingQuery_Errors verifies search domain=roadmap requires a query.
func TestHandleSearch_DomainRoadmap_MissingQuery_Errors(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{"domain": "roadmap"})
	result, err := handleSearch(db)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleGet_DomainRoadmap_LoadError verifies get domain=roadmap surfaces
// a roadmap.Load failure (corrupt metadata) as a tool error.
func TestHandleGet_DomainRoadmap_LoadError(t *testing.T) {
	resetDB(t)

	_, err := store.UpsertDocument(db, store.Document{
		Type:     "roadmap",
		Project:  "acme-corp",
		Title:    "roadmap",
		Content:  "# Roadmap",
		Metadata: map[string]string{"data": "not json"},
	})
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"domain": "roadmap", "projects": []interface{}{"acme-corp"},
	})
	result, err := handleGet(db)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleSearch_DomainRoadmap_WithLimit verifies search domain=roadmap honors an explicit limit.
func TestHandleSearch_DomainRoadmap_WithLimit(t *testing.T) {
	resetDB(t)

	addReq := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "Limited searchable widget",
	})
	_, err := handleRoadmap(db, nil)(context.TODO(), addReq)
	require.NoError(t, err)

	searchReq := makeRequest(map[string]interface{}{
		"domain": "roadmap", "query": "widget", "limit": float64(5),
	})
	result, err := handleSearch(db)(context.TODO(), searchReq)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var resp []map[string]interface{}
	require.NoError(t, unmarshalResult(result, &resp))
	require.Len(t, resp, 1)
}

// TestHandleSearch_DomainRoadmap_Error verifies search domain=roadmap surfaces
// a store.SearchDocuments failure (broken FTS index) as a tool error.
func TestHandleSearch_DomainRoadmap_Error(t *testing.T) {
	brokenDB, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer brokenDB.Close()
	require.NoError(t, store.ApplySchema(brokenDB))

	addReq := makeRequest(map[string]interface{}{
		"op": "add", "project": "acme-corp", "section": "now", "title": "widget",
	})
	_, err = handleRoadmap(brokenDB, nil)(context.TODO(), addReq)
	require.NoError(t, err)

	_, err = brokenDB.Exec("DROP TABLE documents_fts")
	require.NoError(t, err)

	searchReq := makeRequest(map[string]interface{}{
		"domain": "roadmap", "query": "widget",
	})
	result, err := handleSearch(brokenDB)(context.TODO(), searchReq)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}
