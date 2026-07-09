package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/roadmap"
)

// ── parsing helpers ──────────────────────────────────────────────────────────

func TestParseIntIDs(t *testing.T) {
	ids, err := parseIntIDs([]string{"1", "2", " 3"})
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3}, ids)
}

func TestParseIntIDsEmpty(t *testing.T) {
	ids, err := parseIntIDs(nil)
	require.NoError(t, err)
	assert.Nil(t, ids)
}

func TestParseIntIDsInvalid(t *testing.T) {
	_, err := parseIntIDs([]string{"1", "x", "3"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid id "x"`)
}

func TestParseBlockers(t *testing.T) {
	blockers, err := parseBlockers([]string{"acme-corp:PR-1:waiting on review, with a comma", "other-proj:issue-2"})
	require.NoError(t, err)
	require.Len(t, blockers, 2)
	assert.Equal(t, roadmap.Blocker{Project: "acme-corp", Ref: "PR-1", Note: "waiting on review, with a comma"}, blockers[0])
	assert.Equal(t, roadmap.Blocker{Project: "other-proj", Ref: "issue-2"}, blockers[1])
}

func TestParseBlockersEmpty(t *testing.T) {
	blockers, err := parseBlockers(nil)
	require.NoError(t, err)
	assert.Nil(t, blockers)
}

func TestParseBlockersMissingRef(t *testing.T) {
	_, err := parseBlockers([]string{"acme-corp"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --blocked-by")
}

func TestParseBlockersMissingProject(t *testing.T) {
	_, err := parseBlockers([]string{":PR-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing project")
}

func TestParseBlockersMissingRefEmpty(t *testing.T) {
	_, err := parseBlockers([]string{"acme-corp:"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing ref")
}

func TestParseBlockersNoteWithColon(t *testing.T) {
	blockers, err := parseBlockers([]string{"acme-corp:PR-1:waiting on 10:30 review"})
	require.NoError(t, err)
	require.Len(t, blockers, 1)
	assert.Equal(t, "waiting on 10:30 review", blockers[0].Note)
}

func TestParseSectionInvalid(t *testing.T) {
	_, err := parseSection("bogus")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid section")
}

// ── show ─────────────────────────────────────────────────────────────────────

func TestRunRoadmapShowEmpty(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runRoadmapShow(&buf, db, "acme-corp", "", "", "")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "# Roadmap")
	assert.Contains(t, buf.String(), "_none_")
}

func TestRunRoadmapShowPopulated(t *testing.T) {
	db := newTestDB(t)

	item := roadmap.Item{Title: "ship widget", Body: "build the widget"}
	err := runRoadmapAdd(&bytes.Buffer{}, db, "acme-corp", roadmap.SectionNow, item)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runRoadmapShow(&buf, db, "acme-corp", "", "", "")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "ship widget")
	assert.Contains(t, buf.String(), "build the widget")
}

func TestRunRoadmapShowByEpicResolvesLabelAndFilters(t *testing.T) {
	db := newTestDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	epicItem, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "EPIC: WiFi map", "", "", "", "")
	require.NoError(t, err)

	require.NoError(t, runRoadmapAdd(&bytes.Buffer{}, db, "acme-corp", roadmap.SectionNow, roadmap.Item{
		Title: "in the epic", Component: "widget", Epic: epicItem.ID,
	}))
	require.NoError(t, runRoadmapAdd(&bytes.Buffer{}, db, "acme-corp", roadmap.SectionNow, roadmap.Item{
		Title: "not in the epic", Component: "widget",
	}))

	var buf bytes.Buffer
	err = runRoadmapShow(&buf, db, "acme-corp", "epic", "", "")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "#### epic: WiFi map")
	assert.Contains(t, buf.String(), "in the epic")
	assert.Contains(t, buf.String(), "not in the epic")

	buf.Reset()
	err = runRoadmapShow(&buf, db, "acme-corp", "epic", "", epicItem.ID)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "in the epic")
	assert.NotContains(t, buf.String(), "not in the epic")
}

// ── add ──────────────────────────────────────────────────────────────────────

func TestRunRoadmapAdd(t *testing.T) {
	db := newTestDB(t)

	item := roadmap.Item{
		Title:     "ship widget",
		Body:      "build the widget",
		Component: "widget",
		KB:        []int{1, 2},
		Ticket:    []string{"tk-test-000"},
		BlockedBy: []roadmap.Blocker{{Project: "other-proj", Ref: "PR-9"}},
	}

	var buf bytes.Buffer
	err := runRoadmapAdd(&buf, db, "acme-corp", roadmap.SectionNow, item)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "added item 1 to acme-corp/now")

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 1)
	assert.Equal(t, "ship widget", rm.Sections.Now[0].Title)
	assert.Equal(t, []int{1, 2}, rm.Sections.Now[0].KB)
}

// ── update ───────────────────────────────────────────────────────────────────

func TestRunRoadmapUpdate(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, runRoadmapAdd(&bytes.Buffer{}, db, "acme-corp", roadmap.SectionNow, roadmap.Item{Title: "old title"}))

	newTitle := "new title"
	patch := roadmap.Patch{Title: &newTitle}

	var buf bytes.Buffer
	err := runRoadmapUpdate(&buf, db, "acme-corp", 1, patch, false, "")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "updated item 1")

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 1)
	assert.Equal(t, "new title", rm.Sections.Now[0].Title)
}

func TestRunRoadmapUpdateWithSectionMove(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, runRoadmapAdd(&bytes.Buffer{}, db, "acme-corp", roadmap.SectionNow, roadmap.Item{Title: "item"}))

	var buf bytes.Buffer
	err := runRoadmapUpdate(&buf, db, "acme-corp", 1, roadmap.Patch{}, true, roadmap.SectionNext)
	require.NoError(t, err)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	assert.Empty(t, rm.Sections.Now)
	require.Len(t, rm.Sections.Next, 1)
}

func TestRunRoadmapUpdateNotFound(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runRoadmapUpdate(&buf, db, "acme-corp", 9999, roadmap.Patch{}, false, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "roadmap:")
}

// ── move ─────────────────────────────────────────────────────────────────────

func TestRunRoadmapMove(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, runRoadmapAdd(&bytes.Buffer{}, db, "acme-corp", roadmap.SectionNow, roadmap.Item{Title: "item"}))

	var buf bytes.Buffer
	err := runRoadmapMove(&buf, db, "acme-corp", 1, roadmap.SectionParked)
	require.NoError(t, err)
	assert.True(t, strings.Contains(buf.String(), "moved item 1 to parked"))

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	assert.Empty(t, rm.Sections.Now)
	require.Len(t, rm.Sections.Parked, 1)
}

func TestRunRoadmapMoveNotFound(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runRoadmapMove(&buf, db, "acme-corp", 9999, roadmap.SectionParked)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "roadmap:")
}

// ── done ─────────────────────────────────────────────────────────────────────

func TestRunRoadmapDone(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, runRoadmapAdd(&bytes.Buffer{}, db, "acme-corp", roadmap.SectionNow, roadmap.Item{Title: "item"}))

	var buf bytes.Buffer
	err := runRoadmapDone(&buf, db, "acme-corp", 1)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "marked item 1 done")

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Done, 1)
}

func TestRunRoadmapDoneNotFound(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runRoadmapDone(&buf, db, "acme-corp", 9999)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "roadmap:")
}

// ── remove ───────────────────────────────────────────────────────────────────

func TestRunRoadmapRemove(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, runRoadmapAdd(&bytes.Buffer{}, db, "acme-corp", roadmap.SectionNow, roadmap.Item{Title: "item"}))

	var buf bytes.Buffer
	err := runRoadmapRemove(&buf, db, "acme-corp", 1)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "removed item 1 from acme-corp")

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	assert.Empty(t, rm.Sections.Now)
}

func TestRunRoadmapRemoveNotFound(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runRoadmapRemove(&buf, db, "acme-corp", 9999)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "roadmap:")
}

// ── reorder ──────────────────────────────────────────────────────────────────

func TestRunRoadmapReorder(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, runRoadmapAdd(&bytes.Buffer{}, db, "acme-corp", roadmap.SectionNow, roadmap.Item{Title: "item"}))

	var buf bytes.Buffer
	err := runRoadmapReorder(&buf, db, "acme-corp", 1, 5)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "reordered item 1 to position 5")

	// A single item in the section: any target index clamps to 0, so its
	// densely-renumbered Position is always 1.
	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 1)
	assert.Equal(t, 1, rm.Sections.Now[0].Position)
}

func TestRunRoadmapReorderNotFound(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runRoadmapReorder(&buf, db, "acme-corp", 9999, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "roadmap:")
}

// ── epic / component / position / new sections ──────────────────────────────

// TestRunRoadmapAddWithEpicAndPosition verifies runRoadmapAdd's variadic
// position: on a NON-empty section, index 0 splices the new item to the
// front (item.Position itself is never an ordering input).
func TestRunRoadmapAddWithEpicAndPosition(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, runRoadmapAdd(&bytes.Buffer{}, db, "acme-corp", roadmap.SectionNow, roadmap.Item{Title: "existing"}))

	item := roadmap.Item{Title: "item", Epic: "BB-707"}
	var buf bytes.Buffer
	err := runRoadmapAdd(&buf, db, "acme-corp", roadmap.SectionNow, item, 0)
	require.NoError(t, err)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 2)
	assert.Equal(t, "item", rm.Sections.Now[0].Title, "position 0 splices the new item to the front")
	assert.Equal(t, "BB-707", rm.Sections.Now[0].Epic)
	assert.Equal(t, 1, rm.Sections.Now[0].Position)
	assert.Equal(t, "existing", rm.Sections.Now[1].Title)
	assert.Equal(t, 2, rm.Sections.Now[1].Position)
}

// TestRunRoadmapAddNoPositionAppendsLast verifies runRoadmapAdd with no
// position given plain-appends.
func TestRunRoadmapAddNoPositionAppendsLast(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, runRoadmapAdd(&bytes.Buffer{}, db, "acme-corp", roadmap.SectionNow, roadmap.Item{Title: "first"}))
	require.NoError(t, runRoadmapAdd(&bytes.Buffer{}, db, "acme-corp", roadmap.SectionNow, roadmap.Item{Title: "second"}))

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 2)
	assert.Equal(t, "first", rm.Sections.Now[0].Title)
	assert.Equal(t, "second", rm.Sections.Now[1].Title)
	assert.Zero(t, rm.Sections.Now[0].Position, "section stays legacy — no position was ever given")
}

func TestRunRoadmapUpdateEpic(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, runRoadmapAdd(&bytes.Buffer{}, db, "acme-corp", roadmap.SectionNow, roadmap.Item{Title: "item"}))

	epic := "BB-707"
	patch := roadmap.Patch{Epic: &epic}

	var buf bytes.Buffer
	err := runRoadmapUpdate(&buf, db, "acme-corp", 1, patch, false, "")
	require.NoError(t, err)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 1)
	assert.Equal(t, "BB-707", rm.Sections.Now[0].Epic)
}

func TestRunRoadmapMoveWithPosition(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, runRoadmapAdd(&bytes.Buffer{}, db, "acme-corp", roadmap.SectionNow, roadmap.Item{Title: "item"}))

	var buf bytes.Buffer
	err := runRoadmapMove(&buf, db, "acme-corp", 1, roadmap.SectionDeferred, 9)
	require.NoError(t, err)

	// A lone item landing in an empty target section clamps to index 0,
	// densely renumbered to Position 1.
	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Deferred, 1)
	assert.Equal(t, 1, rm.Sections.Deferred[0].Position)
}

func TestParseSectionDeferredAndDropped(t *testing.T) {
	section, err := parseSection("deferred")
	require.NoError(t, err)
	assert.Equal(t, roadmap.SectionDeferred, section)

	section, err = parseSection("dropped")
	require.NoError(t, err)
	assert.Equal(t, roadmap.SectionDropped, section)
}
