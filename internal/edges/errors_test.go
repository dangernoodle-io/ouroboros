package edges_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/edges"
	"dangernoodle.io/ouroboros/internal/store"
	_ "modernc.org/sqlite"
)

// closedDB returns a DB with schema applied, then closed — forces every
// subsequent Exec/Query/QueryRow to error, exercising each function's error
// path without real fault injection. Pattern mirrors
// internal/store/store_test.go's TestUpsertDocument_ExecError.
func closedDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	require.NoError(t, store.ApplySchema(db))
	require.NoError(t, db.Close())
	return db
}

func TestValidateEndpointsInvalidTargetType(t *testing.T) {
	db := testDB(t)
	_, itemA, itemB := seedProjectAndItems(t, db)

	// Valid source type, invalid target type — exercises the dstType branch
	// of validateEndpoints specifically (src passes, dst fails).
	_, err := edges.Link(db, edges.TypeItem, itemA, "blocks", "doc", itemB, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target type")
}

func TestLinkInsertErrorOnNonexistentProjectID(t *testing.T) {
	db := testDB(t)
	_, itemA, itemB := seedProjectAndItems(t, db)

	// projects.id 999999 doesn't exist — trips the edges.project_id FK
	// (foreign_keys=ON in this test DB), exercising Link's INSERT error path.
	_, err := edges.Link(db, edges.TypeItem, itemA, "blocks", edges.TypeItem, itemB, 999999)
	require.Error(t, err)
}

func TestUnlinkErrorOnClosedDB(t *testing.T) {
	db := closedDB(t)
	_, err := edges.Unlink(db, edges.TypeItem, "AC-1", "blocks", edges.TypeItem, "AC-2")
	assert.Error(t, err)
}

func TestEdgesForErrorOnClosedDB(t *testing.T) {
	db := closedDB(t)
	_, err := edges.EdgesFor(db, edges.TypeItem, "AC-1")
	assert.Error(t, err)
}

func TestItemsByEdgeErrorOnClosedDB(t *testing.T) {
	db := closedDB(t)
	_, err := edges.ItemsByEdge(db, edges.TypeItem, "AC-1", "blocks")
	assert.Error(t, err)
}

func TestListEdgesErrorOnClosedDB(t *testing.T) {
	db := closedDB(t)
	_, err := edges.ListEdges(db, "")
	assert.Error(t, err)

	_, err = edges.ListEdges(db, "blocks")
	assert.Error(t, err)
}

func TestCascadeDeleteErrorOnClosedDB(t *testing.T) {
	db := closedDB(t)
	_, err := edges.CascadeDelete(db, edges.TypeItem, "AC-1")
	assert.Error(t, err)
}

func TestDeleteAutolinksErrorOnClosedDB(t *testing.T) {
	db := closedDB(t)
	_, err := edges.DeleteAutolinks(db, edges.TypeKB, "1", "explains")
	assert.Error(t, err)
}

func TestAutolinkKBErrorOnClosedDB(t *testing.T) {
	db := closedDB(t)
	// DeleteAutolinks (the first call inside AutolinkKB) fails immediately
	// on a closed DB.
	_, err := edges.AutolinkKB(db, 1, "acme-corp", "see [[Some Title]]")
	assert.Error(t, err)
}
