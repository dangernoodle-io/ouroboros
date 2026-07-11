package app

import (
	"context"
	"database/sql"
	"testing"

	"github.com/dangernoodle-io/mcpkit/mcpx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/roadmap"
	"dangernoodle.io/ouroboros/internal/store"
)

func intPtrV2(i int) *int             { return &i }
func float64PtrV2(f float64) *float64 { return &f }

// TestHandleRoadmapV2_Add mirrors TestHandleRoadmap_Add.
func TestHandleRoadmapV2_Add(t *testing.T) {
	resetDB(t)

	in := roadmapInput{
		Op:            "add",
		Project:       "acme-corp",
		Section:       "now",
		Title:         strPtrV2("Ship the widget"),
		Body:          strPtrV2("Body text"),
		Component:     strPtrV2("widget"),
		Why:           strPtrV2("priority shift"),
		ResumeTrigger: strPtrV2("n/a"),
		KB:            &[]int{1, 2},
		Ticket:        &[]string{"TK-1"},
		BlockedBy:     &[]blockerInput{{Project: "other-repo", Ref: "OR-9", Note: "waiting"}},
	}

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, in)
	require.NoError(t, err)
	require.False(t, res.IsError)

	var resp map[string]interface{}
	require.NoError(t, jsonUnmarshalText(res, &resp))
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

// TestHandleRoadmapV2_Add_InvalidSection mirrors TestHandleRoadmap_Add_InvalidSection.
func TestHandleRoadmapV2_Add_InvalidSection(t *testing.T) {
	resetDB(t)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "add", Project: "acme-corp", Section: "bogus", Title: strPtrV2("x"),
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)
}

// TestHandleRoadmapV2_Add_BlockedByMissingProject mirrors
// TestHandleRoadmap_Add_BlockedByMissingProject.
func TestHandleRoadmapV2_Add_BlockedByMissingProject(t *testing.T) {
	resetDB(t)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "add", Project: "acme-corp", Section: "now", Title: strPtrV2("x"),
		BlockedBy: &[]blockerInput{{Project: "", Ref: "OR-9"}},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	assert.Contains(t, mcpx.ResultText(res), "missing project")
}

// TestHandleRoadmapV2_Add_BlockedByMissingRef mirrors
// TestHandleRoadmap_Add_BlockedByMissingRef.
func TestHandleRoadmapV2_Add_BlockedByMissingRef(t *testing.T) {
	resetDB(t)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "add", Project: "acme-corp", Section: "now", Title: strPtrV2("x"),
		BlockedBy: &[]blockerInput{{Project: "other-repo", Ref: ""}},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	assert.Contains(t, mcpx.ResultText(res), "missing ref")
}

// TestHandleRoadmapV2_Update_BlockedByMissingProject mirrors
// TestHandleRoadmap_Update_BlockedByMissingProject.
func TestHandleRoadmapV2_Update_BlockedByMissingProject(t *testing.T) {
	resetDB(t)

	_, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "add", Project: "acme-corp", Section: "now", Title: strPtrV2("x"),
	})
	require.NoError(t, err)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "update", Project: "acme-corp", ID: intPtrV2(1),
		BlockedBy: &[]blockerInput{{Project: "", Ref: "OR-9"}},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	assert.Contains(t, mcpx.ResultText(res), "missing project")
}

// TestHandleRoadmapV2_Update mirrors TestHandleRoadmap_Update.
func TestHandleRoadmapV2_Update(t *testing.T) {
	resetDB(t)

	_, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "add", Project: "acme-corp", Section: "now", Title: strPtrV2("Original"), Body: strPtrV2("orig body"),
	})
	require.NoError(t, err)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "update", Project: "acme-corp", ID: intPtrV2(1), Title: strPtrV2("Updated title"),
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 1)
	assert.Equal(t, "Updated title", rm.Sections.Now[0].Title)
	assert.Equal(t, "orig body", rm.Sections.Now[0].Body, "body should be unchanged (nil patch field)")
}

// TestHandleRoadmapV2_Update_SliceFields mirrors TestHandleRoadmap_Update_SliceFields.
func TestHandleRoadmapV2_Update_SliceFields(t *testing.T) {
	resetDB(t)

	_, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "add", Project: "acme-corp", Section: "now", Title: strPtrV2("Original"),
	})
	require.NoError(t, err)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "update", Project: "acme-corp", ID: intPtrV2(1),
		KB:        &[]int{7},
		Ticket:    &[]string{"TK-9"},
		BlockedBy: &[]blockerInput{{Project: "other-repo", Ref: "OR-1"}},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 1)
	item := rm.Sections.Now[0]
	assert.Equal(t, []int{7}, item.KB)
	assert.Equal(t, []string{"TK-9"}, item.Ticket)
	require.Len(t, item.BlockedBy, 1)
	assert.Equal(t, "other-repo", item.BlockedBy[0].Project)
}

// TestHandleRoadmapV2_Update_ClearFields proves presence-tracked pointer
// fields distinguish absent (nil, untouched) from present-empty (non-nil
// zero value, clears) -- the KEY PATTERN this build spec calls out for
// UpdateItem's patch semantics.
func TestHandleRoadmapV2_Update_ClearFields(t *testing.T) {
	resetDB(t)

	_, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "add", Project: "acme-corp", Section: "now", Title: strPtrV2("Original"),
		Body:      strPtrV2("orig body"),
		Component: strPtrV2("widget"),
		KB:        &[]int{1, 2},
		Ticket:    &[]string{"TK-1"},
	})
	require.NoError(t, err)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "update", Project: "acme-corp", ID: intPtrV2(1),
		Body:      strPtrV2(""),
		Component: strPtrV2(""),
		KB:        &[]int{},
		Ticket:    &[]string{},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 1)
	item := rm.Sections.Now[0]
	assert.Equal(t, "Original", item.Title, "title untouched (nil patch field)")
	assert.Empty(t, item.Body, "body cleared (present-empty patch field)")
	assert.Empty(t, item.Component, "component cleared (present-empty patch field)")
	assert.Empty(t, item.KB, "kb cleared (present-empty patch field)")
	assert.Empty(t, item.Ticket, "ticket cleared (present-empty patch field)")
}

// TestHandleRoadmapV2_Update_NotFound mirrors TestHandleRoadmap_Update_NotFound.
func TestHandleRoadmapV2_Update_NotFound(t *testing.T) {
	resetDB(t)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "update", Project: "acme-corp", ID: intPtrV2(999), Title: strPtrV2("x"),
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)
}

// TestHandleRoadmapV2_Move mirrors TestHandleRoadmap_Move.
func TestHandleRoadmapV2_Move(t *testing.T) {
	resetDB(t)

	_, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "add", Project: "acme-corp", Section: "now", Title: strPtrV2("Move me"),
	})
	require.NoError(t, err)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "move", Project: "acme-corp", ID: intPtrV2(1), To: "parked",
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	var resp map[string]interface{}
	require.NoError(t, jsonUnmarshalText(res, &resp))
	assert.Equal(t, "parked", resp["section"])

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	assert.Empty(t, rm.Sections.Now)
	require.Len(t, rm.Sections.Parked, 1)
}

// TestHandleRoadmapV2_Move_InvalidSection mirrors TestHandleRoadmap_Move_InvalidSection.
func TestHandleRoadmapV2_Move_InvalidSection(t *testing.T) {
	resetDB(t)

	_, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "add", Project: "acme-corp", Section: "now", Title: strPtrV2("x"),
	})
	require.NoError(t, err)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "move", Project: "acme-corp", ID: intPtrV2(1), To: "bogus",
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)
}

// TestHandleRoadmapV2_Move_WithPosition mirrors TestHandleRoadmap_Move_WithPosition.
func TestHandleRoadmapV2_Move_WithPosition(t *testing.T) {
	resetDB(t)

	_, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "add", Project: "acme-corp", Section: "now", Title: strPtrV2("Move me"),
	})
	require.NoError(t, err)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "move", Project: "acme-corp", ID: intPtrV2(1), To: "deferred", Position: float64PtrV2(6),
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Deferred, 1)
	assert.Equal(t, 1, rm.Sections.Deferred[0].Position)
}

// TestHandleRoadmapV2_Move_NotFound mirrors TestHandleRoadmap_Move_NotFound.
func TestHandleRoadmapV2_Move_NotFound(t *testing.T) {
	resetDB(t)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "move", Project: "acme-corp", ID: intPtrV2(9999), To: "parked",
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)
}

// TestHandleRoadmapV2_Done mirrors TestHandleRoadmap_Done.
func TestHandleRoadmapV2_Done(t *testing.T) {
	resetDB(t)

	_, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "add", Project: "acme-corp", Section: "now", Title: strPtrV2("Finish me"),
	})
	require.NoError(t, err)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "done", Project: "acme-corp", ID: intPtrV2(1),
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Done, 1)
}

// TestHandleRoadmapV2_Done_NotFound mirrors TestHandleRoadmap_Done_NotFound.
func TestHandleRoadmapV2_Done_NotFound(t *testing.T) {
	resetDB(t)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "done", Project: "acme-corp", ID: intPtrV2(9999),
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)
}

// TestHandleRoadmapV2_Remove mirrors TestHandleRoadmap_Remove.
func TestHandleRoadmapV2_Remove(t *testing.T) {
	resetDB(t)

	_, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "add", Project: "acme-corp", Section: "now", Title: strPtrV2("Delete me"),
	})
	require.NoError(t, err)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "remove", Project: "acme-corp", ID: intPtrV2(1),
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	assert.Empty(t, rm.Sections.Now)
	assert.Empty(t, rm.Sections.Next)
	assert.Empty(t, rm.Sections.Parked)
	assert.Empty(t, rm.Sections.Done)
}

// TestHandleRoadmapV2_Remove_NotFound mirrors TestHandleRoadmap_Remove_NotFound.
func TestHandleRoadmapV2_Remove_NotFound(t *testing.T) {
	resetDB(t)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "remove", Project: "acme-corp", ID: intPtrV2(9999),
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)
}

// TestHandleRoadmapV2_MissingProject_Errors mirrors TestHandleRoadmap_MissingProject_Errors.
func TestHandleRoadmapV2_MissingProject_Errors(t *testing.T) {
	resetDB(t)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "add", Section: "now", Title: strPtrV2("x"),
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Equal(t, errRoadmapProjectRequired, mcpx.ResultText(res))
}

// TestHandleRoadmapV2_InvalidOp_Errors mirrors TestHandleRoadmap_InvalidOp_Errors.
func TestHandleRoadmapV2_InvalidOp_Errors(t *testing.T) {
	resetDB(t)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "bogus", Project: "acme-corp",
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Equal(t, errRoadmapOpRequired, mcpx.ResultText(res))
}

// TestHandleRoadmapV2_MissingID_Errors mirrors TestHandleRoadmap_MissingID_Errors.
func TestHandleRoadmapV2_MissingID_Errors(t *testing.T) {
	resetDB(t)

	for _, op := range []string{"update", "move", "done", "remove"} {
		res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
			Op: op, Project: "acme-corp",
		})
		require.NoError(t, err)
		assert.True(t, res.IsError, "op=%s should require id", op)
		assert.Equal(t, errRoadmapIDRequired, mcpx.ResultText(res), "op=%s", op)
	}
}

// TestHandleRoadmapV2_Singleton mirrors TestHandleRoadmap_Singleton.
func TestHandleRoadmapV2_Singleton(t *testing.T) {
	resetDB(t)

	for i := 0; i < 2; i++ {
		_, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
			Op: "add", Project: "acme-corp", Section: "now", Title: strPtrV2("item"),
		})
		require.NoError(t, err)
	}

	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM documents WHERE type='roadmap' AND project='acme-corp'`).Scan(&count))
	assert.Equal(t, 1, count)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	assert.Len(t, rm.Sections.Now, 2)
}

// TestHandleRoadmapV2_Add_MutateError mirrors TestHandleRoadmap_Add_MutateError.
func TestHandleRoadmapV2_Add_MutateError(t *testing.T) {
	brokenDB, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer brokenDB.Close()
	require.NoError(t, store.ApplySchema(brokenDB))
	_, err = brokenDB.Exec("DROP TABLE documents")
	require.NoError(t, err)

	res, _, callErr := handleRoadmapV2(&serverState{db: brokenDB})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "add", Project: "acme-corp", Section: "now", Title: strPtrV2("x"),
	})
	require.NoError(t, callErr)
	assert.True(t, res.IsError)
}

// TestHandleRoadmapV2_Reorder mirrors TestHandleRoadmap_Reorder.
func TestHandleRoadmapV2_Reorder(t *testing.T) {
	resetDB(t)

	_, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "add", Project: "acme-corp", Section: "now", Title: strPtrV2("stays first"),
	})
	require.NoError(t, err)
	_, _, err = handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "add", Project: "acme-corp", Section: "now", Title: strPtrV2("reorder me"),
	})
	require.NoError(t, err)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "reorder", Project: "acme-corp", ID: intPtrV2(2), Position: float64PtrV2(4),
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 2)
	assert.Equal(t, "stays first", rm.Sections.Now[0].Title)
	assert.Equal(t, 1, rm.Sections.Now[0].Position)
	assert.Equal(t, "reorder me", rm.Sections.Now[1].Title)
	assert.Equal(t, 2, rm.Sections.Now[1].Position)
}

// TestHandleRoadmapV2_Reorder_MissingPosition mirrors
// TestHandleRoadmap_Reorder_MissingPosition.
func TestHandleRoadmapV2_Reorder_MissingPosition(t *testing.T) {
	resetDB(t)

	_, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "add", Project: "acme-corp", Section: "now", Title: strPtrV2("x"),
	})
	require.NoError(t, err)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "reorder", Project: "acme-corp", ID: intPtrV2(1),
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Equal(t, errRoadmapPositionRequired, mcpx.ResultText(res))
}

// TestHandleRoadmapV2_Reorder_MissingID mirrors TestHandleRoadmap_Reorder_MissingID.
func TestHandleRoadmapV2_Reorder_MissingID(t *testing.T) {
	resetDB(t)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "reorder", Project: "acme-corp", Position: float64PtrV2(1),
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Equal(t, errRoadmapIDRequired, mcpx.ResultText(res))
}

// TestHandleRoadmapV2_Reorder_NotFound mirrors TestHandleRoadmap_Reorder_NotFound.
func TestHandleRoadmapV2_Reorder_NotFound(t *testing.T) {
	resetDB(t)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "reorder", Project: "acme-corp", ID: intPtrV2(9999), Position: float64PtrV2(1),
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)
}

// TestHandleRoadmapV2_Add_EpicAndPosition mirrors TestHandleRoadmap_Add_EpicAndPosition.
func TestHandleRoadmapV2_Add_EpicAndPosition(t *testing.T) {
	resetDB(t)

	_, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "add", Project: "acme-corp", Section: "now", Title: strPtrV2("existing"),
	})
	require.NoError(t, err)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "add", Project: "acme-corp", Section: "now", Title: strPtrV2("x"),
		Epic: strPtrV2("BB-707"), Position: float64PtrV2(0),
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 2)
	assert.Equal(t, "x", rm.Sections.Now[0].Title, "position 0 splices the new item to the front")
	assert.Equal(t, "BB-707", rm.Sections.Now[0].Epic)
	assert.Equal(t, 1, rm.Sections.Now[0].Position)
	assert.Equal(t, "existing", rm.Sections.Now[1].Title)
	assert.Equal(t, 2, rm.Sections.Now[1].Position)
}

// TestHandleRoadmapV2_Add_NoPosition_AppendsLast mirrors
// TestHandleRoadmap_Add_NoPosition_AppendsLast.
func TestHandleRoadmapV2_Add_NoPosition_AppendsLast(t *testing.T) {
	resetDB(t)

	_, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "add", Project: "acme-corp", Section: "now", Title: strPtrV2("first"),
	})
	require.NoError(t, err)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "add", Project: "acme-corp", Section: "now", Title: strPtrV2("second"),
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 2)
	assert.Equal(t, "first", rm.Sections.Now[0].Title)
	assert.Equal(t, "second", rm.Sections.Now[1].Title)
	assert.Zero(t, rm.Sections.Now[0].Position, "section stays legacy — no position was ever given")
}

// TestHandleRoadmapV2_Update_Epic mirrors TestHandleRoadmap_Update_Epic.
func TestHandleRoadmapV2_Update_Epic(t *testing.T) {
	resetDB(t)

	_, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "add", Project: "acme-corp", Section: "now", Title: strPtrV2("x"),
	})
	require.NoError(t, err)

	res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
		Op: "update", Project: "acme-corp", ID: intPtrV2(1), Epic: strPtrV2("BB-707"),
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 1)
	assert.Equal(t, "BB-707", rm.Sections.Now[0].Epic)
}

// TestHandleRoadmapV2_Add_DeferredAndDropped mirrors
// TestHandleRoadmap_Add_DeferredAndDropped.
func TestHandleRoadmapV2_Add_DeferredAndDropped(t *testing.T) {
	resetDB(t)

	for _, section := range []string{"deferred", "dropped"} {
		res, _, err := handleRoadmapV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, roadmapInput{
			Op: "add", Project: "acme-corp", Section: section, Title: strPtrV2("item " + section),
		})
		require.NoError(t, err)
		require.False(t, res.IsError, "section=%s", section)
	}

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	assert.Len(t, rm.Sections.Deferred, 1)
	assert.Len(t, rm.Sections.Dropped, 1)
}

// TestHandleRoadmapV2_BlockersFromInput_NilAndEmpty verifies
// blockersFromInputV2's not-present (nil) and present-but-empty cases both
// return (nil, nil), mirroring parseBlockers' absent-key and
// present-but-empty behavior (an empty slice never marshals differently
// from nil given roadmap.Item.BlockedBy's omitempty tag).
func TestHandleRoadmapV2_BlockersFromInput_NilAndEmpty(t *testing.T) {
	v, err := blockersFromInputV2(nil)
	require.NoError(t, err)
	assert.Nil(t, v)

	v, err = blockersFromInputV2([]blockerInput{})
	require.NoError(t, err)
	assert.Nil(t, v)
}
