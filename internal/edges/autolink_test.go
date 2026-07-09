package edges_test

import (
	"database/sql"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/edges"
	"dangernoodle.io/ouroboros/internal/store"
)

func seedDoc(t *testing.T, db *sql.DB, title, content string) int64 {
	t.Helper()
	result, err := store.UpsertDocument(db, store.Document{
		Type: "note", Project: "acme-corp", Title: title, Content: content,
	})
	require.NoError(t, err)
	return result.ID
}

func TestAutolinkKBResolvesKBTitle(t *testing.T) {
	db := testDB(t)
	targetID := seedDoc(t, db, "Target Doc", "target content")
	sourceID := seedDoc(t, db, "Source Doc", "see [[Target Doc]] for details")

	count, err := edges.AutolinkKB(db, sourceID, "acme-corp", "see [[Target Doc]] for details")
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	list, err := edges.EdgesFor(db, edges.TypeKB, itoa(sourceID))
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "explains", list[0].Label)
	assert.Equal(t, edges.TypeKB, list[0].TargetType)
	assert.Equal(t, itoa(targetID), list[0].TargetID)
}

func TestAutolinkKBResolvesItemID(t *testing.T) {
	db := testDB(t)
	_, itemA, _ := seedProjectAndItems(t, db)
	sourceID := seedDoc(t, db, "Source Doc", "blocked on [[AC-1]]")

	count, err := edges.AutolinkKB(db, sourceID, "acme-corp", "blocked on [[AC-1]]")
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	list, err := edges.EdgesFor(db, edges.TypeKB, itoa(sourceID))
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, edges.TypeItem, list[0].TargetType)
	assert.Equal(t, itemA, list[0].TargetID)
}

func TestAutolinkKBSkipsUnresolvedSilently(t *testing.T) {
	db := testDB(t)
	sourceID := seedDoc(t, db, "Source Doc", "see [[Nonexistent Title]]")

	count, err := edges.AutolinkKB(db, sourceID, "acme-corp", "see [[Nonexistent Title]]")
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	list, err := edges.EdgesFor(db, edges.TypeKB, itoa(sourceID))
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestAutolinkKBNoBrackets(t *testing.T) {
	db := testDB(t)
	sourceID := seedDoc(t, db, "Source Doc", "plain content, no links")

	count, err := edges.AutolinkKB(db, sourceID, "acme-corp", "plain content, no links")
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestAutolinkKBIsIdempotentOnRewrite(t *testing.T) {
	db := testDB(t)
	targetID := seedDoc(t, db, "Target Doc", "target content")
	sourceID := seedDoc(t, db, "Source Doc", "see [[Target Doc]]")

	_, err := edges.AutolinkKB(db, sourceID, "acme-corp", "see [[Target Doc]]")
	require.NoError(t, err)
	// Re-write with the same content: no duplicate edge.
	count, err := edges.AutolinkKB(db, sourceID, "acme-corp", "see [[Target Doc]]")
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	list, err := edges.EdgesFor(db, edges.TypeKB, itoa(sourceID))
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, itoa(targetID), list[0].TargetID)

	// Re-write with different content: stale link is dropped.
	other := seedDoc(t, db, "Other Doc", "other content")
	count, err = edges.AutolinkKB(db, sourceID, "acme-corp", "see [[Other Doc]]")
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	list, err = edges.EdgesFor(db, edges.TypeKB, itoa(sourceID))
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, itoa(other), list[0].TargetID)
}

func TestAutolinkKBSkipsSelfReference(t *testing.T) {
	db := testDB(t)
	sourceID := seedDoc(t, db, "Self Doc", "see [[Self Doc]]")

	count, err := edges.AutolinkKB(db, sourceID, "acme-corp", "see [[Self Doc]]")
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestAutolinkKBDedupesRepeatedTitleInSameContent(t *testing.T) {
	db := testDB(t)
	seedDoc(t, db, "Target Doc", "target content")
	sourceID := seedDoc(t, db, "Source Doc", "see [[Target Doc]] and again [[Target Doc]]")

	count, err := edges.AutolinkKB(db, sourceID, "acme-corp", "see [[Target Doc]] and again [[Target Doc]]")
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func itoa(id int64) string {
	return strconv.FormatInt(id, 10)
}
