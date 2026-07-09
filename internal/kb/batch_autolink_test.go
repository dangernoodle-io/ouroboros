package kb_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/edges"
	"dangernoodle.io/ouroboros/internal/kb"
)

func itoaHelper(id int64) string {
	return strconv.FormatInt(id, 10)
}

func TestWriteBatchAutolinksKBTitle(t *testing.T) {
	db := testDB(t)

	results, err := kb.WriteBatch(db, []kb.Entry{
		{Type: "note", Project: "acme-corp", Title: "Target Doc", Content: "target content"},
	}, "")
	require.NoError(t, err)
	targetID := results[0].ID

	results, err = kb.WriteBatch(db, []kb.Entry{
		{Type: "note", Project: "acme-corp", Title: "Source Doc", Content: "see [[Target Doc]] for details"},
	}, "")
	require.NoError(t, err)
	sourceID := results[0].ID

	list, err := edges.EdgesFor(db, edges.TypeKB, itoaHelper(sourceID))
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "explains", list[0].Label)
	assert.Equal(t, itoaHelper(targetID), list[0].TargetID)
}

func TestWriteBatchAutolinkUnresolvedTitleSkippedSilently(t *testing.T) {
	db := testDB(t)

	results, err := kb.WriteBatch(db, []kb.Entry{
		{Type: "note", Project: "acme-corp", Title: "Source Doc", Content: "see [[Nowhere]]"},
	}, "")
	require.NoError(t, err, "an unresolved [[title]] must not fail the write")

	list, err := edges.EdgesFor(db, edges.TypeKB, itoaHelper(results[0].ID))
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestWriteBatchAutolinkIdempotentOnRewrite(t *testing.T) {
	db := testDB(t)

	_, err := kb.WriteBatch(db, []kb.Entry{
		{Type: "note", Project: "acme-corp", Title: "Target Doc", Content: "target content"},
	}, "")
	require.NoError(t, err)

	results, err := kb.WriteBatch(db, []kb.Entry{
		{Type: "note", Project: "acme-corp", Title: "Source Doc", Content: "see [[Target Doc]]"},
	}, "")
	require.NoError(t, err)
	sourceID := results[0].ID

	// Re-write (upsert, same natural key) with identical content: no dup edge.
	_, err = kb.WriteBatch(db, []kb.Entry{
		{Type: "note", Project: "acme-corp", Title: "Source Doc", Content: "see [[Target Doc]]"},
	}, "")
	require.NoError(t, err)

	list, err := edges.EdgesFor(db, edges.TypeKB, itoaHelper(sourceID))
	require.NoError(t, err)
	assert.Len(t, list, 1)
}
