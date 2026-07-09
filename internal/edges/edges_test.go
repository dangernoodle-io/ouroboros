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

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	_, err = db.Exec("PRAGMA foreign_keys=ON")
	require.NoError(t, err)
	require.NoError(t, store.ApplySchema(db))
	t.Cleanup(func() { db.Close() })
	return db
}

// seedProjectAndItems creates a project and two backlog items directly
// (bypassing internal/backlog to avoid an import cycle in this test package).
func seedProjectAndItems(t *testing.T, db *sql.DB) (projectID int64, itemA, itemB string) {
	t.Helper()
	res, err := db.Exec("INSERT INTO projects (name, prefix, created) VALUES (?, ?, ?)", "acme-corp", "AC", "2026-01-01T00:00:00Z")
	require.NoError(t, err)
	projectID, err = res.LastInsertId()
	require.NoError(t, err)

	_, err = db.Exec(
		"INSERT INTO items (id, project_id, seq, priority, title, status, created, updated) VALUES (?, ?, ?, ?, ?, 'open', ?, ?)",
		"AC-1", projectID, 1, "P1", "item a", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z",
	)
	require.NoError(t, err)
	_, err = db.Exec(
		"INSERT INTO items (id, project_id, seq, priority, title, status, created, updated) VALUES (?, ?, ?, ?, ?, 'open', ?, ?)",
		"AC-2", projectID, 2, "P1", "item b", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z",
	)
	require.NoError(t, err)

	return projectID, "AC-1", "AC-2"
}

func TestMigrationV13AppliesOnPreV13DB(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	require.NoError(t, store.ApplySchema(db))

	var name string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='edges'").Scan(&name)
	require.NoError(t, err)
	assert.Equal(t, "edges", name)

	var maxVersion int
	require.NoError(t, db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&maxVersion))
	assert.Equal(t, 13, maxVersion)

	// Idempotent: re-applying is a no-op, not an error.
	require.NoError(t, store.ApplySchema(db))
}

func TestValidLabelAndType(t *testing.T) {
	assert.True(t, edges.ValidLabel("blocks"))
	assert.True(t, edges.ValidLabel("relates"))
	assert.True(t, edges.ValidLabel("explains"))
	assert.False(t, edges.ValidLabel("part-of"))
	assert.False(t, edges.ValidLabel(""))

	assert.True(t, edges.ValidType("item"))
	assert.True(t, edges.ValidType("kb"))
	assert.False(t, edges.ValidType("doc"))
}

func TestLinkRejectsBadLabel(t *testing.T) {
	db := testDB(t)
	_, itemA, itemB := seedProjectAndItems(t, db)

	_, err := edges.Link(db, edges.TypeItem, itemA, "part-of", edges.TypeItem, itemB, 0)
	assert.Error(t, err)
}

func TestLinkRejectsBadType(t *testing.T) {
	db := testDB(t)
	_, itemA, itemB := seedProjectAndItems(t, db)

	_, err := edges.Link(db, "doc", itemA, "blocks", edges.TypeItem, itemB, 0)
	assert.Error(t, err)
}

func TestLinkUnlinkEdgesFor(t *testing.T) {
	db := testDB(t)
	projectID, itemA, itemB := seedProjectAndItems(t, db)

	edge, err := edges.Link(db, edges.TypeItem, itemA, "blocks", edges.TypeItem, itemB, projectID)
	require.NoError(t, err)
	assert.Equal(t, itemA, edge.SourceID)
	assert.Equal(t, itemB, edge.TargetID)
	assert.Equal(t, "blocks", edge.Label)
	assert.NotEmpty(t, edge.CreatedAt)

	list, err := edges.EdgesFor(db, edges.TypeItem, itemA)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, itemA, list[0].SourceID)

	// The target side also sees the edge (both directions).
	list, err = edges.EdgesFor(db, edges.TypeItem, itemB)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, itemB, list[0].TargetID)

	affected, err := edges.Unlink(db, edges.TypeItem, itemA, "blocks", edges.TypeItem, itemB)
	require.NoError(t, err)
	assert.Equal(t, int64(1), affected)

	list, err = edges.EdgesFor(db, edges.TypeItem, itemA)
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestUnlinkRejectsBadLabelAndType(t *testing.T) {
	db := testDB(t)
	_, itemA, itemB := seedProjectAndItems(t, db)

	_, err := edges.Unlink(db, edges.TypeItem, itemA, "part-of", edges.TypeItem, itemB)
	assert.Error(t, err)

	_, err = edges.Unlink(db, "doc", itemA, "blocks", edges.TypeItem, itemB)
	assert.Error(t, err)
}

func TestItemsByEdgeRejectsBadTypeAndLabel(t *testing.T) {
	db := testDB(t)

	_, err := edges.ItemsByEdge(db, "doc", "1", "blocks")
	assert.Error(t, err)

	_, err = edges.ItemsByEdge(db, edges.TypeItem, "1", "part-of")
	assert.Error(t, err)
}

func TestListEdgesRejectsBadLabel(t *testing.T) {
	db := testDB(t)

	_, err := edges.ListEdges(db, "part-of")
	assert.Error(t, err)
}

func TestCascadeDeleteRejectsBadType(t *testing.T) {
	db := testDB(t)

	_, err := edges.CascadeDelete(db, "doc", "1")
	assert.Error(t, err)
}

func TestLinkRejectsEmptyIDs(t *testing.T) {
	db := testDB(t)
	_, itemA, _ := seedProjectAndItems(t, db)

	_, err := edges.Link(db, edges.TypeItem, itemA, "blocks", edges.TypeItem, "", 0)
	assert.Error(t, err)

	_, err = edges.Link(db, edges.TypeItem, "", "blocks", edges.TypeItem, itemA, 0)
	assert.Error(t, err)
}

func TestUnlinkNoMatchIsZeroNotError(t *testing.T) {
	db := testDB(t)
	_, itemA, itemB := seedProjectAndItems(t, db)

	affected, err := edges.Unlink(db, edges.TypeItem, itemA, "blocks", edges.TypeItem, itemB)
	require.NoError(t, err)
	assert.Equal(t, int64(0), affected)
}

func TestLinkIsUpsertSafe(t *testing.T) {
	db := testDB(t)
	projectID, itemA, itemB := seedProjectAndItems(t, db)

	first, err := edges.Link(db, edges.TypeItem, itemA, "blocks", edges.TypeItem, itemB, projectID)
	require.NoError(t, err)
	second, err := edges.Link(db, edges.TypeItem, itemA, "blocks", edges.TypeItem, itemB, projectID)
	require.NoError(t, err)
	assert.Equal(t, first.ID, second.ID)

	list, err := edges.EdgesFor(db, edges.TypeItem, itemA)
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestEdgesForEmpty(t *testing.T) {
	db := testDB(t)
	_, itemA, _ := seedProjectAndItems(t, db)

	list, err := edges.EdgesFor(db, edges.TypeItem, itemA)
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestEdgesForInvalidType(t *testing.T) {
	db := testDB(t)
	_, err := edges.EdgesFor(db, "doc", "1")
	assert.Error(t, err)
}

func TestItemsByEdge(t *testing.T) {
	db := testDB(t)
	projectID, itemA, itemB := seedProjectAndItems(t, db)

	_, err := edges.Link(db, edges.TypeItem, itemA, "blocks", edges.TypeItem, itemB, projectID)
	require.NoError(t, err)

	ids, err := edges.ItemsByEdge(db, edges.TypeItem, itemB, "blocks")
	require.NoError(t, err)
	assert.Equal(t, []string{itemA}, ids)

	// No match for a different label.
	ids, err = edges.ItemsByEdge(db, edges.TypeItem, itemB, "relates")
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestListEdgesFilterByLabel(t *testing.T) {
	db := testDB(t)
	projectID, itemA, itemB := seedProjectAndItems(t, db)

	_, err := edges.Link(db, edges.TypeItem, itemA, "blocks", edges.TypeItem, itemB, projectID)
	require.NoError(t, err)
	_, err = edges.Link(db, edges.TypeItem, itemB, "relates", edges.TypeItem, itemA, projectID)
	require.NoError(t, err)

	all, err := edges.ListEdges(db, "")
	require.NoError(t, err)
	assert.Len(t, all, 2)

	blocks, err := edges.ListEdges(db, "blocks")
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	assert.Equal(t, "blocks", blocks[0].Label)
}

func TestCascadeDeleteRemovesSourceAndTargetEdges(t *testing.T) {
	db := testDB(t)
	projectID, itemA, itemB := seedProjectAndItems(t, db)

	// itemA is the source of a "blocks" edge and the target of a "relates" edge.
	_, err := edges.Link(db, edges.TypeItem, itemA, "blocks", edges.TypeItem, itemB, projectID)
	require.NoError(t, err)
	_, err = edges.Link(db, edges.TypeItem, itemB, "relates", edges.TypeItem, itemA, projectID)
	require.NoError(t, err)

	affected, err := edges.CascadeDelete(db, edges.TypeItem, itemA)
	require.NoError(t, err)
	assert.Equal(t, int64(2), affected)

	list, err := edges.ListEdges(db, "")
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestAliasResolutionOnRenamedEndpoint(t *testing.T) {
	db := testDB(t)
	projectID, itemA, itemB := seedProjectAndItems(t, db)

	_, err := edges.Link(db, edges.TypeItem, itemA, "blocks", edges.TypeItem, itemB, projectID)
	require.NoError(t, err)

	// Rename itemA -> AC-9 (mirrors backlog.RenameProject's renamePrefixInTx:
	// update items.id, cascade-update any edge referencing the old id, and
	// record the alias).
	_, err = db.Exec("UPDATE items SET id = 'AC-9' WHERE id = ?", itemA)
	require.NoError(t, err)
	_, err = db.Exec("UPDATE edges SET source_id = 'AC-9' WHERE source_type = 'item' AND source_id = ?", itemA)
	require.NoError(t, err)
	_, err = db.Exec("UPDATE edges SET target_id = 'AC-9' WHERE target_type = 'item' AND target_id = ?", itemA)
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO item_id_aliases (old_id, new_id, renamed_at) VALUES (?, ?, ?)", itemA, "AC-9", "2026-01-01T00:00:00Z")
	require.NoError(t, err)

	// EdgesFor(old id) still resolves via the alias.
	list, err := edges.EdgesFor(db, edges.TypeItem, itemA)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "AC-9", list[0].SourceID)

	// Link(old id) also resolves before writing, so a retrofit link against
	// a stale id lands on the current row.
	_, err = edges.Link(db, edges.TypeItem, itemA, "relates", edges.TypeItem, itemB, projectID)
	require.NoError(t, err)
	all, err := edges.EdgesFor(db, edges.TypeItem, "AC-9")
	require.NoError(t, err)
	assert.Len(t, all, 2)

	// Unlink(old id) resolves too.
	affected, err := edges.Unlink(db, edges.TypeItem, itemA, "blocks", edges.TypeItem, itemB)
	require.NoError(t, err)
	assert.Equal(t, int64(1), affected)
}

func TestDeleteAutolinks(t *testing.T) {
	db := testDB(t)
	projectID, itemA, itemB := seedProjectAndItems(t, db)

	_, err := edges.Link(db, edges.TypeKB, "1", "explains", edges.TypeItem, itemA, projectID)
	require.NoError(t, err)
	_, err = edges.Link(db, edges.TypeItem, itemB, "blocks", edges.TypeItem, itemA, projectID)
	require.NoError(t, err)

	affected, err := edges.DeleteAutolinks(db, edges.TypeKB, "1", "explains")
	require.NoError(t, err)
	assert.Equal(t, int64(1), affected)

	// The unrelated item->item edge survives.
	list, err := edges.ListEdges(db, "")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "blocks", list[0].Label)
}
