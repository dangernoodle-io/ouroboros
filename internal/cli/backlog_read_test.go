package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/query"
)

// TestMergeItemsByStatusDedup exercises mergeItemsByStatus's seen-map dedup
// branch directly: statuses are mutually exclusive per item via the
// underlying filter (so a genuine duplicate can't occur through
// `backlog list`'s own --status dedup), but the merge helper still guards
// against the same id surfacing from more than one per-status query.Get
// call. Passing the same status twice re-fetches the same result set for
// both iterations, forcing the second occurrence of every id through the
// dedup branch.
func TestMergeItemsByStatusDedup(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Item", "", "", "", "")
	require.NoError(t, err)

	merged, err := mergeItemsByStatus(db, query.Request{Domain: "backlog", Limit: 20}, []string{"open", "open"})
	require.NoError(t, err)
	require.Len(t, merged, 1)
	assert.Equal(t, "AC-1", merged[0].ID)
}

func TestMergeItemsByStatusError(t *testing.T) {
	db := newTestDB(t)
	_, err := mergeItemsByStatus(db, query.Request{Domain: "backlog", Sort: "bogus"}, []string{"open", "done"})
	require.Error(t, err)
}

func TestItemLessPriorityOrder(t *testing.T) {
	a := backlog.Item{ID: "A-1", Priority: "P0"}
	b := backlog.Item{ID: "A-2", Priority: "P1"}
	assert.True(t, itemLess(a, b, false))
	assert.False(t, itemLess(b, a, false))
}

func TestItemLessPriorityTieBreaksOnID(t *testing.T) {
	a := backlog.Item{ID: "A-1", Priority: "P0"}
	b := backlog.Item{ID: "A-2", Priority: "P0"}
	assert.True(t, itemLess(a, b, false))
}

func TestItemLessMalformedPrioritySortsLast(t *testing.T) {
	valid := backlog.Item{ID: "A-1", Priority: "P0"}
	malformed := backlog.Item{ID: "A-2", Priority: "bogus"}
	assert.True(t, itemLess(valid, malformed, false))
	assert.False(t, itemLess(malformed, valid, false))
}

func TestItemLessCreatedOrder(t *testing.T) {
	older := backlog.Item{ID: "A-1", Created: "2026-01-01T00:00:00Z"}
	newer := backlog.Item{ID: "A-2", Created: "2026-02-01T00:00:00Z"}
	assert.True(t, itemLess(newer, older, true))
	assert.False(t, itemLess(older, newer, true))
}

func TestItemLessCreatedTieBreaksOnID(t *testing.T) {
	a := backlog.Item{ID: "A-1", Created: "2026-01-01T00:00:00Z"}
	b := backlog.Item{ID: "A-2", Created: "2026-01-01T00:00:00Z"}
	assert.True(t, itemLess(a, b, true))
}

func TestDedupStrings(t *testing.T) {
	assert.Equal(t, []string{"open", "done"}, dedupStrings([]string{"open", "done", "open", ""}))
	assert.Empty(t, dedupStrings(nil))
}

func TestPriorityMinMax(t *testing.T) {
	lo, hi := priorityMinMax("P3")
	assert.Equal(t, "P3", lo)
	assert.Equal(t, "P3", hi)

	lo, hi = priorityMinMax("")
	assert.Equal(t, "", lo)
	assert.Equal(t, "", hi)

	lo, hi = priorityMinMax("bogus")
	assert.Equal(t, "", lo)
	assert.Equal(t, "", hi)

	lo, hi = priorityMinMax("P9")
	assert.Equal(t, "", lo)
	assert.Equal(t, "", hi)
}

func TestPrintBacklogItemTable(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Title", "", "", "backend", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = printBacklogItemTable(&buf, db, []backlog.Item{*item})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "acme-corp")
	assert.Contains(t, buf.String(), "backend")
}

func TestWriteEdgesEmpty(t *testing.T) {
	var buf bytes.Buffer
	writeEdges(&buf, nil)
	assert.Empty(t, buf.String())
}
