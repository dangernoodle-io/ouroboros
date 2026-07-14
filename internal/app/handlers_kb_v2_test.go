package app

import (
	"context"
	"fmt"
	"testing"

	"github.com/dangernoodle-io/mcpkit/mcpx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/kb"
	"dangernoodle.io/ouroboros/internal/store"
)

func strPtrV2(s string) *string { return &s }

func tagsPtrV2(tags []string) *[]string { return &tags }

func metaPtrV2(m map[string]string) *map[string]string { return &m }

// TestHandleKBV2Batch mirrors TestHandleKBBatch: single-entry create.
func TestHandleKBV2Batch(t *testing.T) {
	resetDB(t)

	in := kbInput{Entries: []kbEntryInput{
		{
			Type:    strPtrV2("decision"),
			Project: strPtrV2("acme-corp"),
			Title:   strPtrV2("Use PostgreSQL"),
			Content: strPtrV2("Superior query performance for our use case"),
			Tags:    tagsPtrV2([]string{"database", "infrastructure"}),
		},
	}}

	res, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, in)
	require.NoError(t, err)
	require.NotNil(t, res)

	var resp []kb.PutResult
	require.NoError(t, jsonUnmarshalText(res, &resp))
	require.Len(t, resp, 1)
	assert.Equal(t, "created", resp[0].Action)
	assert.NotZero(t, resp[0].ID)
	assert.Equal(t, "Use PostgreSQL", resp[0].Title)
}

// TestHandleKBV2BatchMultiple mirrors TestHandleKBBatchMultiple.
func TestHandleKBV2BatchMultiple(t *testing.T) {
	resetDB(t)

	in := kbInput{Entries: []kbEntryInput{
		{Type: strPtrV2("decision"), Project: strPtrV2("acme-corp"), Title: strPtrV2("Use PostgreSQL"), Content: strPtrV2("Decision 1")},
		{Type: strPtrV2("fact"), Project: strPtrV2("acme-corp"), Title: strPtrV2("Database Version"), Content: strPtrV2("PostgreSQL 15")},
		{Type: strPtrV2("note"), Project: strPtrV2("acme-corp"), Title: strPtrV2("Schema Changes"), Content: strPtrV2("Need migration")},
	}}

	res, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, in)
	require.NoError(t, err)

	var resp []kb.PutResult
	require.NoError(t, jsonUnmarshalText(res, &resp))
	require.Len(t, resp, 3)
	for _, r := range resp {
		assert.Equal(t, "created", r.Action)
		assert.NotZero(t, r.ID)
		assert.NotEmpty(t, r.Title)
	}
}

// TestHandleKBV2BatchEmpty mirrors TestHandleKBBatchEmpty: verbatim message.
func TestHandleKBV2BatchEmpty(t *testing.T) {
	resetDB(t)

	res, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{})
	require.NoError(t, err)
	require.True(t, res.IsError)
	assert.Equal(t, errKBEntriesRequired, mcpx.ResultText(res))
}

// TestHandleKBV2UpdateByID_RetitleInPlace mirrors
// TestHandleKBUpdateByID_RetitleInPlace: an id-addressed update retitles in
// place instead of creating a duplicate row.
func TestHandleKBV2UpdateByID_RetitleInPlace(t *testing.T) {
	resetDB(t)

	createRes, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{Type: strPtrV2("decision"), Project: strPtrV2("acme-corp"), Title: strPtrV2("orig-title"), Content: strPtrV2("c1")},
	}})
	require.NoError(t, err)
	var createResp []kb.PutResult
	require.NoError(t, jsonUnmarshalText(createRes, &createResp))
	id := createResp[0].ID

	updateRes, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{ID: float64(id), Title: strPtrV2("renamed-title")},
	}})
	require.NoError(t, err)
	require.False(t, updateRes.IsError)

	var updateResp []kb.PutResult
	require.NoError(t, jsonUnmarshalText(updateRes, &updateResp))
	require.Len(t, updateResp, 1)
	assert.Equal(t, "updated", updateResp[0].Action)
	assert.Equal(t, "renamed-title", updateResp[0].Title)

	docs, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 50)
	require.NoError(t, err)
	require.Len(t, docs, 1, "retitle must update in place, not create a duplicate")
	assert.Equal(t, "renamed-title", docs[0].Title)
}

// TestHandleKBV2UpdateByID_AllFields verifies every update field applies.
func TestHandleKBV2UpdateByID_AllFields(t *testing.T) {
	resetDB(t)

	createRes, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{Type: strPtrV2("fact"), Project: strPtrV2("acme-corp"), Title: strPtrV2("orig"), Content: strPtrV2("c1")},
	}})
	require.NoError(t, err)
	var createResp []kb.PutResult
	require.NoError(t, jsonUnmarshalText(createRes, &createResp))
	id := createResp[0].ID

	updateRes, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{
			ID:       float64(id),
			Type:     strPtrV2("decision"),
			Category: strPtrV2("architecture"),
			Title:    strPtrV2("new-title"),
			Content:  strPtrV2("new-content"),
			Notes:    strPtrV2("new-notes"),
			Tags:     tagsPtrV2([]string{"tag-a"}),
			Metadata: metaPtrV2(map[string]string{"k": "v"}),
		},
	}})
	require.NoError(t, err)
	require.False(t, updateRes.IsError)

	docs, err := store.GetDocuments(db, []int64{id})
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "decision", docs[0].Type)
	assert.Equal(t, "architecture", docs[0].Category)
	assert.Equal(t, "new-title", docs[0].Title)
	assert.Equal(t, "new-content", docs[0].Content)
	assert.Equal(t, "new-notes", docs[0].Notes)
	assert.Equal(t, []string{"tag-a"}, docs[0].Tags)
	assert.Equal(t, map[string]string{"k": "v"}, docs[0].Metadata)
}

// TestHandleKBV2UpdateByID_NonexistentID_Errors mirrors
// TestHandleKBUpdateByID_NonexistentID_Errors.
func TestHandleKBV2UpdateByID_NonexistentID_Errors(t *testing.T) {
	resetDB(t)

	res, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{ID: float64(999999), Title: strPtrV2("does not matter")},
	}})
	require.NoError(t, err)
	assert.True(t, res.IsError)
}

// TestHandleKBV2UpdateByID_NonexistentID_AppendNotesErrors is a regression
// test for a nil-deref panic: append_notes against a nonexistent id must
// return a clean error result, not panic, and must not write anything.
func TestHandleKBV2UpdateByID_NonexistentID_AppendNotesErrors(t *testing.T) {
	resetDB(t)

	res, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{ID: float64(999999), Notes: strPtrV2("acme-corp addendum"), AppendNotes: true},
	}})
	require.NoError(t, err)
	assert.True(t, res.IsError)

	docs, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 50)
	require.NoError(t, err)
	assert.Empty(t, docs)
}

// TestHandleKBV2UpdateByID_MixedBatch mirrors TestHandleKBUpdateByID_MixedBatch:
// a single entries[] call mixes a create and an update.
func TestHandleKBV2UpdateByID_MixedBatch(t *testing.T) {
	resetDB(t)

	seedRes, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{Type: strPtrV2("fact"), Project: strPtrV2("acme-corp"), Title: strPtrV2("existing"), Content: strPtrV2("c1")},
	}})
	require.NoError(t, err)
	var seedResp []kb.PutResult
	require.NoError(t, jsonUnmarshalText(seedRes, &seedResp))
	id := seedResp[0].ID

	mixedRes, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{ID: float64(id), Title: strPtrV2("existing renamed")},
		{Type: strPtrV2("note"), Project: strPtrV2("acme-corp"), Title: strPtrV2("brand new"), Content: strPtrV2("c2")},
	}})
	require.NoError(t, err)
	require.False(t, mixedRes.IsError)

	var mixedResp []kb.PutResult
	require.NoError(t, jsonUnmarshalText(mixedRes, &mixedResp))
	require.Len(t, mixedResp, 2)

	docs, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 50)
	require.NoError(t, err)
	require.Len(t, docs, 2)
}

// TestHandleKBV2UpdateByID_MixedBatchPartialFailure_Atomic mirrors
// TestHandleKBUpdateByID_MixedBatchPartialFailure_Atomic: a create alongside
// an update targeting a nonexistent id must roll back entirely -- asserted
// via a side-channel store.QueryDocuments re-query.
func TestHandleKBV2UpdateByID_MixedBatchPartialFailure_Atomic(t *testing.T) {
	resetDB(t)

	res, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{Type: strPtrV2("fact"), Project: strPtrV2("acme-corp"), Title: strPtrV2("should-not-persist"), Content: strPtrV2("c1")},
		{ID: float64(999999), Title: strPtrV2("does not matter")},
	}})
	require.NoError(t, err)
	assert.True(t, res.IsError, "batch with a bad update id must fail")

	docs, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 50)
	require.NoError(t, err)
	assert.Empty(t, docs, "create in the same failed batch must not persist")
}

// TestHandleKBV2UpdateByID_StringID mirrors TestHandleKBUpdateByID_StringID:
// a JSON string id still routes to update.
func TestHandleKBV2UpdateByID_StringID(t *testing.T) {
	resetDB(t)

	createRes, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{Type: strPtrV2("fact"), Project: strPtrV2("acme-corp"), Title: strPtrV2("string-id-orig"), Content: strPtrV2("v1")},
	}})
	require.NoError(t, err)
	var createResp []kb.PutResult
	require.NoError(t, jsonUnmarshalText(createRes, &createResp))
	id := createResp[0].ID

	updateRes, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{ID: fmt.Sprintf("%d", id), Title: strPtrV2("string-id-renamed")},
	}})
	require.NoError(t, err)
	require.False(t, updateRes.IsError)

	docs, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 50)
	require.NoError(t, err)
	require.Len(t, docs, 1, "string id must update the existing row, not create a new one")
	assert.Equal(t, "string-id-renamed", docs[0].Title)
}

// TestHandleKBV2UpdateByID_InvalidStringID_Errors mirrors
// TestHandleKBUpdateByID_InvalidStringID_Errors.
func TestHandleKBV2UpdateByID_InvalidStringID_Errors(t *testing.T) {
	resetDB(t)

	res, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{ID: "abc", Title: strPtrV2("does not matter")},
	}})
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Equal(t, "invalid id: abc", mcpx.ResultText(res))

	docs, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 50)
	require.NoError(t, err)
	assert.Empty(t, docs, "invalid id must not be silently misrouted to create")
}

// TestHandleKBV2UpdateByID_ZeroID_Errors mirrors
// TestHandleKBUpdateByID_ZeroID_Errors: id:0 is a hard error, not an
// absent-id sentinel.
func TestHandleKBV2UpdateByID_ZeroID_Errors(t *testing.T) {
	resetDB(t)

	res, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{ID: float64(0), Title: strPtrV2("does not matter")},
	}})
	require.NoError(t, err)
	assert.True(t, res.IsError)
}

// TestHandleKBV2UpdateByID_NegativeID_Errors covers a negative id (not
// exercised by the old test suite's exact-name matrix, but the same
// parseKBEntryID <=0 branch as zero).
func TestHandleKBV2UpdateByID_NegativeID_Errors(t *testing.T) {
	resetDB(t)

	res, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{ID: float64(-5), Title: strPtrV2("does not matter")},
	}})
	require.NoError(t, err)
	assert.True(t, res.IsError)
}

// TestHandleKBV2UpdateByID_NonScalarID_Errors mirrors
// TestHandleKBUpdateByID_NonScalarID_Errors: an id that's neither a JSON
// number nor a string is a hard error.
func TestHandleKBV2UpdateByID_NonScalarID_Errors(t *testing.T) {
	resetDB(t)

	res, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{ID: true, Title: strPtrV2("does not matter")},
	}})
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Equal(t, "invalid id: true", mcpx.ResultText(res))
}

// TestHandleKBV2_IDAbsent_StillUpserts mirrors TestHandleKB_IDAbsent_StillUpserts:
// id-absent entries keep the existing upsert-by-natural-key behavior.
func TestHandleKBV2_IDAbsent_StillUpserts(t *testing.T) {
	resetDB(t)

	first, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{Type: strPtrV2("fact"), Project: strPtrV2("acme-corp"), Category: strPtrV2("cat"), Title: strPtrV2("t"), Content: strPtrV2("v1")},
	}})
	require.NoError(t, err)
	var firstResp []kb.PutResult
	require.NoError(t, jsonUnmarshalText(first, &firstResp))
	require.Equal(t, "created", firstResp[0].Action)

	second, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{Type: strPtrV2("fact"), Project: strPtrV2("acme-corp"), Category: strPtrV2("cat"), Title: strPtrV2("t"), Content: strPtrV2("v2")},
	}})
	require.NoError(t, err)
	var secondResp []kb.PutResult
	require.NoError(t, jsonUnmarshalText(second, &secondResp))
	require.Equal(t, firstResp[0].ID, secondResp[0].ID, "same natural key upserts the same row")

	docs, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 50)
	require.NoError(t, err)
	require.Len(t, docs, 1)
}

// TestHandleKBV2ValidationAbortsEntireBatch mirrors
// TestHandleKBValidationAbortsEntireBatch: a validation failure anywhere in
// the batch prevents every write in that batch, including earlier valid
// entries.
func TestHandleKBV2ValidationAbortsEntireBatch(t *testing.T) {
	resetDB(t)

	res, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{Type: strPtrV2("decision"), Project: strPtrV2("acme-corp"), Title: strPtrV2("Valid entry"), Content: strPtrV2("Content 1")},
		{Project: strPtrV2("acme-corp"), Title: strPtrV2("Invalid entry"), Content: strPtrV2("Content 2")}, // missing required type
	}})
	require.NoError(t, err)
	assert.True(t, res.IsError, "should return error due to invalid entry")

	docs, err := store.QueryDocuments(db, []string{"decision"}, nil, nil, "", nil, 50)
	require.NoError(t, err)
	assert.Empty(t, docs, "batch validation failure should prevent all writes")
}

// TestHandleKBV2Batch50Entries mirrors TestHandleKBBatch50Entries.
func TestHandleKBV2Batch50Entries(t *testing.T) {
	resetDB(t)

	entries := make([]kbEntryInput, 50)
	for i := 0; i < 50; i++ {
		title := fmt.Sprintf("Entry %02d", i+1)
		entries[i] = kbEntryInput{
			Type:    strPtrV2("note"),
			Project: strPtrV2("acme-corp"),
			Title:   strPtrV2(title),
			Content: strPtrV2("Content for entry"),
		}
	}

	res, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: entries})
	require.NoError(t, err)

	var resp []kb.PutResult
	require.NoError(t, jsonUnmarshalText(res, &resp))
	require.Len(t, resp, 50)
	for _, r := range resp {
		assert.Equal(t, "created", r.Action)
		assert.NotZero(t, r.ID)
	}
}

// TestHandleKBV2Batch_CategoryNotesMetadata mirrors
// TestHandleKBBatch_CategoryNotesMetadata.
func TestHandleKBV2Batch_CategoryNotesMetadata(t *testing.T) {
	resetDB(t)

	res, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{
			Type:     strPtrV2("decision"),
			Project:  strPtrV2("acme-corp"),
			Title:    strPtrV2("Entry with extras"),
			Content:  strPtrV2("Content"),
			Category: strPtrV2("architecture"),
			Notes:    strPtrV2("Some notes here"),
			Metadata: metaPtrV2(map[string]string{"source": "meeting", "owner": "team-a"}),
		},
	}})
	require.NoError(t, err)
	require.False(t, res.IsError)

	var resp []kb.PutResult
	require.NoError(t, jsonUnmarshalText(res, &resp))
	require.Len(t, resp, 1)
	assert.Equal(t, "created", resp[0].Action)
}

// TestHandleKBV2Batch_ConflictDetected mirrors TestHandleKBBatch_ConflictDetected.
func TestHandleKBV2Batch_ConflictDetected(t *testing.T) {
	resetDB(t)

	_, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{Type: strPtrV2("decision"), Project: strPtrV2("acme-corp"), Title: strPtrV2("conflict-entry"), Content: strPtrV2("original content")},
	}})
	require.NoError(t, err)

	res, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{Type: strPtrV2("decision"), Project: strPtrV2("acme-corp"), Title: strPtrV2("conflict-entry"), Content: strPtrV2("changed content")},
	}})
	require.NoError(t, err)
	require.False(t, res.IsError)

	var resp []kb.PutResult
	require.NoError(t, jsonUnmarshalText(res, &resp))
	require.Len(t, resp, 1)
	assert.True(t, resp[0].Conflict)
	assert.NotEmpty(t, resp[0].PreviousExcerpt)
}

// TestHandleKBV2UpdateByID_EmptyContentRejected covers ValidateEntryUpdate's
// explicit-empty rejection on the update path (not exercised by the old
// suite's exact-name matrix, but a distinct branch in kb.WriteAndUpdateBatch
// this port must preserve): an explicit content="" on update is an error,
// not a silent no-op.
func TestHandleKBV2UpdateByID_EmptyContentRejected(t *testing.T) {
	resetDB(t)

	createRes, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{Type: strPtrV2("fact"), Project: strPtrV2("acme-corp"), Title: strPtrV2("t"), Content: strPtrV2("v1")},
	}})
	require.NoError(t, err)
	var createResp []kb.PutResult
	require.NoError(t, jsonUnmarshalText(createRes, &createResp))
	id := createResp[0].ID

	res, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{ID: float64(id), Content: strPtrV2("")},
	}})
	require.NoError(t, err)
	assert.True(t, res.IsError)
}

// TestHandleKBV2UpdateAppendNotes verifies append_notes concatenates onto
// existing non-empty notes, \n\n-separated (OU-299), on an id-addressed
// update.
func TestHandleKBV2UpdateAppendNotes(t *testing.T) {
	resetDB(t)

	createRes, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{Type: strPtrV2("fact"), Project: strPtrV2("acme-corp"), Title: strPtrV2("t"), Content: strPtrV2("v1"), Notes: strPtrV2("existing note")},
	}})
	require.NoError(t, err)
	var createResp []kb.PutResult
	require.NoError(t, jsonUnmarshalText(createRes, &createResp))
	id := createResp[0].ID

	res, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{ID: float64(id), Notes: strPtrV2("new note"), AppendNotes: true},
	}})
	require.NoError(t, err)
	require.False(t, res.IsError)

	docs, err := store.GetDocuments(db, []int64{id})
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "existing note\n\nnew note", docs[0].Notes)
}

// TestHandleKBV2UpdateAppendNotes_OmittedOrEmpty_Unchanged verifies
// append_notes with no notes text (omitted or "") is a no-op.
func TestHandleKBV2UpdateAppendNotes_OmittedOrEmpty_Unchanged(t *testing.T) {
	resetDB(t)

	createRes, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{Type: strPtrV2("fact"), Project: strPtrV2("acme-corp"), Title: strPtrV2("t"), Content: strPtrV2("v1"), Notes: strPtrV2("keep me")},
	}})
	require.NoError(t, err)
	var createResp []kb.PutResult
	require.NoError(t, jsonUnmarshalText(createRes, &createResp))
	id := createResp[0].ID

	res, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{ID: float64(id), AppendNotes: true, Title: strPtrV2("touch other field")},
	}})
	require.NoError(t, err)
	require.False(t, res.IsError)
	docs, err := store.GetDocuments(db, []int64{id})
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "keep me", docs[0].Notes, "append_notes with omitted notes must preserve")

	res, _, err = handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{ID: float64(id), Notes: strPtrV2(""), AppendNotes: true},
	}})
	require.NoError(t, err)
	require.False(t, res.IsError)
	docs, err = store.GetDocuments(db, []int64{id})
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "keep me", docs[0].Notes, "append_notes with empty notes must preserve, not append-of-empty")
}

// TestHandleKBV2UpdateNotesDefaultReplace verifies the default
// (append_notes absent) behavior is unchanged: notes replaces in place.
func TestHandleKBV2UpdateNotesDefaultReplace(t *testing.T) {
	resetDB(t)

	createRes, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{Type: strPtrV2("fact"), Project: strPtrV2("acme-corp"), Title: strPtrV2("t"), Content: strPtrV2("v1"), Notes: strPtrV2("old note")},
	}})
	require.NoError(t, err)
	var createResp []kb.PutResult
	require.NoError(t, jsonUnmarshalText(createRes, &createResp))
	id := createResp[0].ID

	res, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{ID: float64(id), Notes: strPtrV2("replaced note")},
	}})
	require.NoError(t, err)
	require.False(t, res.IsError)

	docs, err := store.GetDocuments(db, []int64{id})
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "replaced note", docs[0].Notes)
}

// TestHandleKBV2UpdateClearNotes_Regression is the OU-299 regression test:
// kb's pointer-nil clear semantics for notes must still work after the
// append_notes addition (empty string clears, omitted preserves).
func TestHandleKBV2UpdateClearNotes_Regression(t *testing.T) {
	resetDB(t)

	createRes, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{Type: strPtrV2("fact"), Project: strPtrV2("acme-corp"), Title: strPtrV2("t"), Content: strPtrV2("v1"), Notes: strPtrV2("some notes")},
	}})
	require.NoError(t, err)
	var createResp []kb.PutResult
	require.NoError(t, jsonUnmarshalText(createRes, &createResp))
	id := createResp[0].ID

	res, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{ID: float64(id), Notes: strPtrV2("")},
	}})
	require.NoError(t, err)
	require.False(t, res.IsError)

	docs, err := store.GetDocuments(db, []int64{id})
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Empty(t, docs[0].Notes, "empty notes must still clear")
}

// TestHandleKBV2Create_AppendNotesIgnored verifies append_notes has no
// effect on the create/natural-key-upsert path: notes is set as provided,
// with no crash and no attempted fetch of a nonexistent row.
func TestHandleKBV2Create_AppendNotesIgnored(t *testing.T) {
	resetDB(t)

	res, _, err := handleKBV2(&serverState{db: db})(context.Background(), &mcpx.CallToolRequest{}, kbInput{Entries: []kbEntryInput{
		{Type: strPtrV2("fact"), Project: strPtrV2("acme-corp"), Title: strPtrV2("t"), Content: strPtrV2("v1"), Notes: strPtrV2("fresh note"), AppendNotes: true},
	}})
	require.NoError(t, err)
	require.False(t, res.IsError)

	var resp []kb.PutResult
	require.NoError(t, jsonUnmarshalText(res, &resp))
	require.Len(t, resp, 1)
	assert.Equal(t, "created", resp[0].Action)

	docs, err := store.GetDocuments(db, []int64{resp[0].ID})
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "fresh note", docs[0].Notes)
}
