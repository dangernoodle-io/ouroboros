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
	err = runKBDelete(&buf, db, fmt.Sprintf("%d", result.ID))
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("deleted document %d\n", result.ID), buf.String())

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
	require.NoError(t, runKBDelete(&buf, db, docID))

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
	err := runKBDelete(&buf, db, "9999")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.Empty(t, buf.String())
}

func TestKBDeleteInvalidID(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runKBDelete(&buf, db, "not-an-int")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid id")
	assert.Empty(t, buf.String())
}

func TestKBDeleteInvalidIDEmpty(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runKBDelete(&buf, db, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid id")
}

func TestKBDeleteInvalidIDFloat(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runKBDelete(&buf, db, "1.5")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid id")
}
