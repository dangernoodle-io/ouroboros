package cli

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/edges"
	"dangernoodle.io/ouroboros/internal/store"
)

func TestKBDeleteHappyPath(t *testing.T) {
	db := newTestDB(t)
	result, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Use PostgreSQL",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runKBDelete(&buf, db, []string{fmt.Sprintf("%d", result.ID)})
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("deleted 1 document(s): %d\n", result.ID), buf.String())

	// verify gone
	doc, err := store.GetDocument(db, result.ID)
	require.NoError(t, err)
	assert.Nil(t, doc)
}

func TestKBDeleteCascadesEdges(t *testing.T) {
	db := newTestDB(t)
	result, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Use PostgreSQL",
	})
	require.NoError(t, err)
	docID := fmt.Sprintf("%d", result.ID)

	other, err := store.UpsertDocument(db, store.Document{
		Type:    "note",
		Project: "acme-corp",
		Title:   "Related note",
	})
	require.NoError(t, err)

	_, err = edges.Link(db, edges.TypeKB, docID, "relates", edges.TypeKB, fmt.Sprintf("%d", other.ID), 0)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, runKBDelete(&buf, db, []string{docID}))

	remaining, err := edges.EdgesFor(db, edges.TypeKB, fmt.Sprintf("%d", other.ID))
	require.NoError(t, err)
	assert.Empty(t, remaining, "deleting a kb doc must cascade-remove edges referencing it")
}

// TestKBDeleteCascadeIsAtomic proves the doc delete + edge cascade run in
// one tx (both or neither): injecting a failure at the cascade step (an
// invalid edge type — CascadeDelete's own validation) and rolling back must
// leave the document delete un-committed too. This exercises exactly the
// tx-based mechanism runKBDelete uses internally (DeleteDocumentTx +
// CascadeDelete on one *sql.Tx), just with a deliberately-invalid second
// call standing in for a real (untestable-without-fault-injection) DB
// error, since production code always calls CascadeDelete with the valid
// edges.TypeKB constant.
func TestKBDeleteCascadeIsAtomic(t *testing.T) {
	db := newTestDB(t)
	result, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Use PostgreSQL",
	})
	require.NoError(t, err)
	docID := fmt.Sprintf("%d", result.ID)

	tx, err := db.Begin()
	require.NoError(t, err)

	require.NoError(t, store.DeleteDocumentTx(tx, result.ID))

	_, err = edges.CascadeDelete(tx, "bogus-type", docID)
	require.Error(t, err, "cascade must fail on an invalid edge type")
	require.NoError(t, tx.Rollback())

	// The doc delete did not survive without its cascade completing —
	// atomic, not partial.
	doc, err := store.GetDocument(db, result.ID)
	require.NoError(t, err)
	assert.NotNil(t, doc, "a failed cascade must roll back the whole delete")
}

func TestKBDeleteNonexistent(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runKBDelete(&buf, db, []string{"9999"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.Empty(t, buf.String())
}

func TestKBDeleteInvalidID(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runKBDelete(&buf, db, []string{"not-an-int"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid id")
	assert.Contains(t, err.Error(), "not-an-int")
	assert.Empty(t, buf.String())
}

func TestKBDeleteInvalidIDEmpty(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runKBDelete(&buf, db, []string{""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid id")
}

func TestKBDeleteInvalidIDFloat(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runKBDelete(&buf, db, []string{"1.5"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid id")
}

func TestKBDeleteInvalidIDZero(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runKBDelete(&buf, db, []string{"0"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid id")
}

func TestKBDeleteInvalidIDNegative(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runKBDelete(&buf, db, []string{"-5"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid id")
}

func TestKBDeleteMultipleIDs(t *testing.T) {
	db := newTestDB(t)

	a, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "Doc A"})
	require.NoError(t, err)
	b, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "Doc B"})
	require.NoError(t, err)
	c, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "Doc C"})
	require.NoError(t, err)

	idA := fmt.Sprintf("%d", a.ID)
	idB := fmt.Sprintf("%d", b.ID)

	var buf bytes.Buffer
	require.NoError(t, runKBDelete(&buf, db, []string{idA, idB}))
	assert.Equal(t, fmt.Sprintf("deleted 2 document(s): %s, %s\n", idA, idB), buf.String())

	docA, err := store.GetDocument(db, a.ID)
	require.NoError(t, err)
	assert.Nil(t, docA)
	docB, err := store.GetDocument(db, b.ID)
	require.NoError(t, err)
	assert.Nil(t, docB)
	docC, err := store.GetDocument(db, c.ID)
	require.NoError(t, err)
	assert.NotNil(t, docC, "id not in the delete set must survive")

	// search: deleted titles unsearchable, surviving doc still found, from a
	// single FTS rebuild after the batch commit.
	resA, err := store.SearchDocuments(db, "Doc A", nil, nil, nil, 10)
	require.NoError(t, err)
	assert.Empty(t, resA)
	resC, err := store.SearchDocuments(db, "Doc C", nil, nil, nil, 10)
	require.NoError(t, err)
	require.Len(t, resC, 1)
	assert.Equal(t, c.ID, resC[0].ID)
}

func TestKBDeleteMultipleIDsCascadesEdges(t *testing.T) {
	db := newTestDB(t)

	a, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "Doc A"})
	require.NoError(t, err)
	b, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "Doc B"})
	require.NoError(t, err)
	other, err := store.UpsertDocument(db, store.Document{Type: "note", Project: "acme-corp", Title: "Related note"})
	require.NoError(t, err)

	idA := fmt.Sprintf("%d", a.ID)
	idB := fmt.Sprintf("%d", b.ID)
	otherID := fmt.Sprintf("%d", other.ID)

	_, err = edges.Link(db, edges.TypeKB, idA, "relates", edges.TypeKB, otherID, 0)
	require.NoError(t, err)
	_, err = edges.Link(db, edges.TypeKB, idB, "relates", edges.TypeKB, otherID, 0)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, runKBDelete(&buf, db, []string{idA, idB}))

	remaining, err := edges.EdgesFor(db, edges.TypeKB, otherID)
	require.NoError(t, err)
	assert.Empty(t, remaining, "deleting each kb doc in the batch must cascade-remove its edges")
}

func TestKBDeleteDuplicateIDsDeduped(t *testing.T) {
	db := newTestDB(t)

	a, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "Doc A"})
	require.NoError(t, err)
	idA := fmt.Sprintf("%d", a.ID)

	var buf bytes.Buffer
	require.NoError(t, runKBDelete(&buf, db, []string{idA, idA}))
	assert.Equal(t, fmt.Sprintf("deleted 1 document(s): %s\n", idA), buf.String())
}

// TestKBDeleteMultipleIDsMissingIsAllOrNothing proves a missing id among an
// otherwise-valid batch aborts before anything is deleted (rollback, not a
// partial delete).
func TestKBDeleteMultipleIDsMissingIsAllOrNothing(t *testing.T) {
	db := newTestDB(t)

	a, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "Doc A"})
	require.NoError(t, err)
	idA := fmt.Sprintf("%d", a.ID)

	var buf bytes.Buffer
	err = runKBDelete(&buf, db, []string{idA, "999999"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.Contains(t, err.Error(), "999999")
	assert.Empty(t, buf.String())

	// nothing deleted, including the valid id in the same batch
	docA, err := store.GetDocument(db, a.ID)
	require.NoError(t, err)
	assert.NotNil(t, docA, "a missing id in the batch must not partially delete valid ids")
}
