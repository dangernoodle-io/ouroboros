package cli

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
