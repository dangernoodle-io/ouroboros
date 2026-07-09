package backlog_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/edges"
)

// TestAddItemTxCommits verifies AddItemTx participates correctly in a
// caller-owned transaction (the tx-aware counterpart used by handleBacklog
// to commit an item write atomically with its inline edges[]).
func TestAddItemTxCommits(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	tx, err := d.Begin()
	require.NoError(t, err)

	item, err := backlog.AddItemTx(tx, p.ID, "AC", "P1", "tx item", "desc", "", "", "")
	require.NoError(t, err)
	assert.Equal(t, "AC-1", item.ID)

	require.NoError(t, tx.Commit())

	fetched, err := backlog.GetItem(d, "AC-1")
	require.NoError(t, err)
	assert.Equal(t, "tx item", fetched.Title)
}

// TestAddItemTxRollback verifies a rolled-back AddItemTx leaves no trace —
// proving the caller's rollback-on-later-failure path (e.g. a bad edges[]
// target) actually undoes the item creation.
func TestAddItemTxRollback(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	tx, err := d.Begin()
	require.NoError(t, err)

	_, err = backlog.AddItemTx(tx, p.ID, "AC", "P1", "tx item", "desc", "", "", "")
	require.NoError(t, err)

	require.NoError(t, tx.Rollback())

	_, err = backlog.GetItem(d, "AC-1")
	assert.Error(t, err, "a rolled-back AddItemTx must not have created the item")
}

// TestUpdateItemTxCommits verifies UpdateItemTx participates correctly in a
// caller-owned transaction.
func TestUpdateItemTxCommits(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)
	item, err := backlog.AddItem(d, p.ID, "AC", "P1", "original", "", "", "", "")
	require.NoError(t, err)

	tx, err := d.Begin()
	require.NoError(t, err)

	updated, err := backlog.UpdateItemTx(tx, item.ID, map[string]string{"title": "updated"})
	require.NoError(t, err)
	assert.Equal(t, "updated", updated.Title)

	require.NoError(t, tx.Commit())

	fetched, err := backlog.GetItem(d, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "updated", fetched.Title)
}

// TestUpdateItemTxRollback verifies a rolled-back UpdateItemTx leaves the
// original field value in place.
func TestUpdateItemTxRollback(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)
	item, err := backlog.AddItem(d, p.ID, "AC", "P1", "original", "", "", "", "")
	require.NoError(t, err)

	tx, err := d.Begin()
	require.NoError(t, err)

	_, err = backlog.UpdateItemTx(tx, item.ID, map[string]string{"title": "should-not-stick"})
	require.NoError(t, err)

	require.NoError(t, tx.Rollback())

	fetched, err := backlog.GetItem(d, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "original", fetched.Title)
}

func TestDeleteItemsCascadesEdges(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	a, err := backlog.AddItem(d, p.ID, "AC", "P1", "item a", "", "", "", "")
	require.NoError(t, err)
	b, err := backlog.AddItem(d, p.ID, "AC", "P1", "item b", "", "", "", "")
	require.NoError(t, err)

	// a is a source of "blocks" and a target of "relates" — both directions
	// must be cleaned up.
	_, err = edges.Link(d, edges.TypeItem, a.ID, "blocks", edges.TypeItem, b.ID, p.ID)
	require.NoError(t, err)
	_, err = edges.Link(d, edges.TypeItem, b.ID, "relates", edges.TypeItem, a.ID, p.ID)
	require.NoError(t, err)

	affected, err := backlog.DeleteItems(d, []string{a.ID})
	require.NoError(t, err)
	assert.Equal(t, int64(1), affected)

	remaining, err := edges.ListEdges(d, "")
	require.NoError(t, err)
	assert.Empty(t, remaining, "deleting an item must cascade-remove every edge referencing it")
}

func TestDeleteItemsNoEdgesIsNoop(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	a, err := backlog.AddItem(d, p.ID, "AC", "P1", "item a", "", "", "", "")
	require.NoError(t, err)

	affected, err := backlog.DeleteItems(d, []string{a.ID})
	require.NoError(t, err)
	assert.Equal(t, int64(1), affected)
}

func TestRenameProjectPrefixCascadesEdges(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	a, err := backlog.AddItem(d, p.ID, "AC", "P1", "item a", "", "", "", "")
	require.NoError(t, err)
	b, err := backlog.AddItem(d, p.ID, "AC", "P1", "item b", "", "", "", "")
	require.NoError(t, err)

	_, err = edges.Link(d, edges.TypeItem, a.ID, "blocks", edges.TypeItem, b.ID, p.ID)
	require.NoError(t, err)

	_, err = backlog.RenameProject(d, "acme-corp", "", "ZZ")
	require.NoError(t, err)

	list, err := edges.EdgesFor(d, edges.TypeItem, "ZZ-1")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "ZZ-1", list[0].SourceID)
	assert.Equal(t, "ZZ-2", list[0].TargetID)

	// The old id still resolves via item_id_aliases.
	viaOldID, err := edges.EdgesFor(d, edges.TypeItem, a.ID)
	require.NoError(t, err)
	require.Len(t, viaOldID, 1)
}

// TestRenameProjectPrefixEdgeCascadeCollisionLeavesNoStaleRow verifies that
// when a renamed item's edge collides (UNIQUE) with an edge already held by
// the new id, the UPDATE-OR-IGNORE'd old-id row is explicitly deleted
// rather than left behind as a stale, unreachable edge.
func TestRenameProjectPrefixEdgeCascadeCollisionLeavesNoStaleRow(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	a, err := backlog.AddItem(d, p.ID, "AC", "P1", "item a", "", "", "", "")
	require.NoError(t, err)
	b, err := backlog.AddItem(d, p.ID, "AC", "P1", "item b", "", "", "", "")
	require.NoError(t, err)

	// The rename will turn AC-1 -> ZZ-1, AC-2 -> ZZ-2. Seed an edge under the
	// OLD id (AC-1 blocks AC-2) and, to force a collision, also seed the
	// identical edge already expressed under the NEW id (ZZ-1 blocks ZZ-2)
	// directly against the edges table (bypassing item existence checks,
	// since ZZ-1 doesn't exist yet — edges have no FK on item ids).
	_, err = edges.Link(d, edges.TypeItem, a.ID, "blocks", edges.TypeItem, b.ID, p.ID)
	require.NoError(t, err)
	_, err = d.Exec(
		"INSERT INTO edges (source_type, source_id, target_type, target_id, label, project_id, created_at) VALUES ('item', 'ZZ-1', 'item', 'ZZ-2', 'blocks', ?, '2026-01-01T00:00:00Z')",
		p.ID,
	)
	require.NoError(t, err)

	var beforeCount int
	require.NoError(t, d.QueryRow("SELECT COUNT(*) FROM edges").Scan(&beforeCount))
	require.Equal(t, 2, beforeCount)

	_, err = backlog.RenameProject(d, "acme-corp", "", "ZZ")
	require.NoError(t, err)

	var afterCount int
	require.NoError(t, d.QueryRow("SELECT COUNT(*) FROM edges").Scan(&afterCount))
	assert.Equal(t, 1, afterCount, "the collided old-id edge must not survive as a stale, unreachable row")

	list, err := edges.EdgesFor(d, edges.TypeItem, "ZZ-1")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "ZZ-1", list[0].SourceID)
	assert.Equal(t, "ZZ-2", list[0].TargetID)

	// No leftover row still carrying the old id.
	var staleCount int
	require.NoError(t, d.QueryRow("SELECT COUNT(*) FROM edges WHERE source_id = ? OR target_id = ?", a.ID, a.ID).Scan(&staleCount))
	assert.Equal(t, 0, staleCount)
}

func TestDeleteProjectForceCascadesEdges(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	a, err := backlog.AddItem(d, p.ID, "AC", "P1", "item a", "", "", "", "")
	require.NoError(t, err)
	b, err := backlog.AddItem(d, p.ID, "AC", "P1", "item b", "", "", "", "")
	require.NoError(t, err)
	_, err = edges.Link(d, edges.TypeItem, a.ID, "blocks", edges.TypeItem, b.ID, p.ID)
	require.NoError(t, err)

	err = backlog.DeleteProject(d, "acme-corp", true, "")
	require.NoError(t, err, "force-deleting a project with edges must not FK-violate")

	remaining, err := edges.ListEdges(d, "")
	require.NoError(t, err)
	assert.Empty(t, remaining, "force-deleting a project must cascade-remove its edges")
}

// TestDeleteProjectForceCascadesNullProjectIDEdges is the case
// TestDeleteProjectForceCascadesEdges doesn't cover: an edge created the
// way CLI `link` and every [[Title]] autolink actually create edges — with
// projectID=0 (stored NULL). The edges.project_id FK's ON DELETE CASCADE
// never fires for a NULL project_id, so this exercises the real
// endpoint-based bulk delete in DeleteProject's force path, for both an
// item<->item edge and a kb<->kb (doc<->doc) edge.
func TestDeleteProjectForceCascadesNullProjectIDEdges(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	a, err := backlog.AddItem(d, p.ID, "AC", "P1", "item a", "", "", "", "")
	require.NoError(t, err)
	b, err := backlog.AddItem(d, p.ID, "AC", "P1", "item b", "", "", "", "")
	require.NoError(t, err)

	// item<->item edge with projectID=0 (NULL), as CLI `link` creates.
	_, err = edges.Link(d, edges.TypeItem, a.ID, "blocks", edges.TypeItem, b.ID, 0)
	require.NoError(t, err)

	// Two documents in the same project, plus a doc->doc "explains" edge
	// with projectID=0 (NULL), as a [[Title]] autolink creates.
	_, err = d.Exec("INSERT INTO documents (type, project, title, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		"note", "acme-corp", "Target Doc", "target content", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z")
	require.NoError(t, err)
	_, err = d.Exec("INSERT INTO documents (type, project, title, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		"note", "acme-corp", "Source Doc", "see [[Target Doc]]", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z")
	require.NoError(t, err)
	var targetDocID, sourceDocID int64
	require.NoError(t, d.QueryRow("SELECT id FROM documents WHERE title = 'Target Doc'").Scan(&targetDocID))
	require.NoError(t, d.QueryRow("SELECT id FROM documents WHERE title = 'Source Doc'").Scan(&sourceDocID))
	_, err = edges.Link(d,
		edges.TypeKB, strconv.FormatInt(sourceDocID, 10),
		"explains",
		edges.TypeKB, strconv.FormatInt(targetDocID, 10),
		0,
	)
	require.NoError(t, err)

	// Sanity: both edges exist and neither carries a project_id.
	before, err := edges.ListEdges(d, "")
	require.NoError(t, err)
	require.Len(t, before, 2)
	for _, e := range before {
		assert.Equal(t, int64(0), e.ProjectID, "edges created via link/autolink must have NULL project_id")
	}

	err = backlog.DeleteProject(d, "acme-corp", true, "")
	require.NoError(t, err)

	remaining, err := edges.ListEdges(d, "")
	require.NoError(t, err)
	assert.Empty(t, remaining, "force-delete must cascade NULL-project_id edges by endpoint, not just via the project_id FK")
}

func TestDeleteProjectReassignRepointsEdges(t *testing.T) {
	d := testDB(t)
	src, err := backlog.CreateProject(d, "src-proj", "SP")
	require.NoError(t, err)
	dst, err := backlog.CreateProject(d, "dst-proj", "DP")
	require.NoError(t, err)

	a, err := backlog.AddItem(d, src.ID, "SP", "P1", "item a", "", "", "", "")
	require.NoError(t, err)
	b, err := backlog.AddItem(d, src.ID, "SP", "P1", "item b", "", "", "", "")
	require.NoError(t, err)
	_, err = edges.Link(d, edges.TypeItem, a.ID, "blocks", edges.TypeItem, b.ID, src.ID)
	require.NoError(t, err)

	err = backlog.DeleteProject(d, "src-proj", false, "dst-proj")
	require.NoError(t, err, "reassigning a project with edges must succeed")

	// Reassign moves the items/edges, it never deletes them — unlike
	// force-delete, the edge count must be unchanged.
	list, err := edges.ListEdges(d, "")
	require.NoError(t, err)
	require.Len(t, list, 1, "reassign must not drop edges")
	assert.Equal(t, dst.ID, list[0].ProjectID, "the edge's project_id must be repointed to the reassignment target")
}
