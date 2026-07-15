package roadmap_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/roadmap"
	"dangernoodle.io/ouroboros/internal/store"
)

func testDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	require.NoError(t, store.ApplySchema(db))
	t.Cleanup(func() { db.Close() })
	return db
}

func TestLoadEmpty(t *testing.T) {
	db := testDB(t)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.NotNil(t, rm)
	assert.Empty(t, rm.Sections.Now)
	assert.Empty(t, rm.Sections.Next)
	assert.Empty(t, rm.Sections.Parked)
	assert.Empty(t, rm.Sections.Done)
	assert.Zero(t, rm.NextID)
}

func TestSaveLoadRoundTrip(t *testing.T) {
	db := testDB(t)

	rm := &roadmap.Roadmap{}
	id, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{
		Title:     "ship the widget",
		Body:      "wire up the new widget endpoint",
		Component: "widget",
		KB:        []int{1, 2},
		Ticket:    []string{"tk-test-000"},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, id)

	require.NoError(t, roadmap.Save(db, "acme-corp", rm))

	loaded, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, loaded.Sections.Now, 1)

	item := loaded.Sections.Now[0]
	assert.Equal(t, 1, item.ID)
	assert.Equal(t, "ship the widget", item.Title)
	assert.Equal(t, "wire up the new widget endpoint", item.Body)
	assert.Equal(t, "widget", item.Component)
	assert.Equal(t, []int{1, 2}, item.KB)
	assert.Equal(t, []string{"tk-test-000"}, item.Ticket)
	assert.NotEmpty(t, item.CreatedAt)
	assert.NotEmpty(t, item.UpdatedAt)
	assert.Equal(t, 1, loaded.NextID)
}

func TestLoadDocWithoutRoadmapMetadata(t *testing.T) {
	db := testDB(t)

	_, err := store.UpsertDocument(db, store.Document{
		Type:    "roadmap",
		Project: "acme-corp",
		Title:   "roadmap",
		Content: "# Roadmap",
	})
	require.NoError(t, err)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	assert.Empty(t, rm.Sections.Now)
	assert.Zero(t, rm.NextID)
}

func TestSingletonEnforced(t *testing.T) {
	db := testDB(t)

	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "first"})
	require.NoError(t, err)
	require.NoError(t, roadmap.Save(db, "acme-corp", rm))

	loaded, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	_, err = roadmap.AddItem(loaded, roadmap.SectionNext, roadmap.Item{Title: "second"})
	require.NoError(t, err)
	require.NoError(t, roadmap.Save(db, "acme-corp", loaded))

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM documents WHERE type='roadmap' AND project=?", "acme-corp").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestAddItemAssignsSequentialIDs(t *testing.T) {
	rm := &roadmap.Roadmap{}

	id1, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "one"})
	require.NoError(t, err)
	id2, err := roadmap.AddItem(rm, roadmap.SectionNext, roadmap.Item{Title: "two"})
	require.NoError(t, err)

	assert.Equal(t, 1, id1)
	assert.Equal(t, 2, id2)
	assert.Equal(t, 2, rm.NextID)
}

func TestAddItemInvalidSection(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.Section("bogus"), roadmap.Item{Title: "x"})
	assert.Error(t, err)
}

func TestUpdateItem(t *testing.T) {
	rm := &roadmap.Roadmap{}
	id, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "orig", Body: "orig body"})
	require.NoError(t, err)

	newTitle := "updated"
	newKB := []int{5, 6}
	err = roadmap.UpdateItem(rm, id, roadmap.Patch{Title: &newTitle, KB: &newKB})
	require.NoError(t, err)

	assert.Equal(t, "updated", rm.Sections.Now[0].Title)
	assert.Equal(t, "orig body", rm.Sections.Now[0].Body, "unpatched fields stay unchanged")
	assert.Equal(t, []int{5, 6}, rm.Sections.Now[0].KB)
}

func TestUpdateItemAllFields(t *testing.T) {
	rm := &roadmap.Roadmap{}
	id, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "orig"})
	require.NoError(t, err)

	title := "new title"
	body := "new body"
	component := "widget"
	why := "why it's parked"
	resume := "resume trigger"
	ticket := []string{"tk-test-001"}
	blockedBy := []roadmap.Blocker{{Project: "breadboard", Ref: "B1-1"}}

	err = roadmap.UpdateItem(rm, id, roadmap.Patch{
		Title:         &title,
		Body:          &body,
		Component:     &component,
		Why:           &why,
		ResumeTrigger: &resume,
		Ticket:        &ticket,
		BlockedBy:     &blockedBy,
	})
	require.NoError(t, err)

	item := rm.Sections.Now[0]
	assert.Equal(t, title, item.Title)
	assert.Equal(t, body, item.Body)
	assert.Equal(t, component, item.Component)
	assert.Equal(t, why, item.Why)
	assert.Equal(t, resume, item.ResumeTrigger)
	assert.Equal(t, ticket, item.Ticket)
	assert.Equal(t, blockedBy, item.BlockedBy)
}

func TestUpdateItemNotFound(t *testing.T) {
	rm := &roadmap.Roadmap{}
	err := roadmap.UpdateItem(rm, 999, roadmap.Patch{})
	assert.Error(t, err)
}

func TestMoveItemPreservesIDAcrossSections(t *testing.T) {
	rm := &roadmap.Roadmap{}
	id, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "movable"})
	require.NoError(t, err)

	require.NoError(t, roadmap.MoveItem(rm, id, roadmap.SectionParked))
	assert.Empty(t, rm.Sections.Now)
	require.Len(t, rm.Sections.Parked, 1)
	assert.Equal(t, id, rm.Sections.Parked[0].ID)

	require.NoError(t, roadmap.MoveItem(rm, id, roadmap.SectionNext))
	assert.Empty(t, rm.Sections.Parked)
	require.Len(t, rm.Sections.Next, 1)
	assert.Equal(t, id, rm.Sections.Next[0].ID)
}

func TestMoveItemSameSectionNoOp(t *testing.T) {
	rm := &roadmap.Roadmap{}
	id, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "stays put"})
	require.NoError(t, err)

	require.NoError(t, roadmap.MoveItem(rm, id, roadmap.SectionNow))
	require.Len(t, rm.Sections.Now, 1)
	assert.Equal(t, id, rm.Sections.Now[0].ID)
}

func TestMoveItemNotFound(t *testing.T) {
	rm := &roadmap.Roadmap{}
	err := roadmap.MoveItem(rm, 999, roadmap.SectionNow)
	assert.Error(t, err)
}

func TestMoveItemInvalidSection(t *testing.T) {
	rm := &roadmap.Roadmap{}
	id, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "x"})
	require.NoError(t, err)
	err = roadmap.MoveItem(rm, id, roadmap.Section("bogus"))
	assert.Error(t, err)
}

func TestMarkDone(t *testing.T) {
	rm := &roadmap.Roadmap{}
	id, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "finish me"})
	require.NoError(t, err)

	require.NoError(t, roadmap.MarkDone(rm, id))
	assert.Empty(t, rm.Sections.Now)
	require.Len(t, rm.Sections.Done, 1)
	assert.Equal(t, id, rm.Sections.Done[0].ID)
}

// TestMarkDoneWithStalePositionLandsLastInDenseTarget verifies the fix for
// the cross-section carry-over bug: an item's stale Position from its
// SOURCE section must not misorder it into the middle of an already-DENSE
// target section — it must land last, densely renumbered.
func TestMarkDoneWithStalePositionLandsLastInDenseTarget(t *testing.T) {
	rm := &roadmap.Roadmap{}

	// Done is already DENSE (1..2).
	doneID1, err := roadmap.AddItem(rm, roadmap.SectionDone, roadmap.Item{Title: "done one"})
	require.NoError(t, err)
	doneID2, err := roadmap.AddItem(rm, roadmap.SectionDone, roadmap.Item{Title: "done two"})
	require.NoError(t, err)
	require.NoError(t, roadmap.ReorderItem(rm, doneID1, 0)) // densifies Done: 1, 2

	// The item to finish carries a stale, LOW Position from its own
	// (also dense) section -- simulating "was at the top of Now, then
	// marked done".
	id, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "finish me"})
	require.NoError(t, err)
	require.NoError(t, roadmap.ReorderItem(rm, id, 0)) // Now dense: Position 1

	require.NoError(t, roadmap.MarkDone(rm, id))

	require.Empty(t, rm.Sections.Now)
	require.Len(t, rm.Sections.Done, 3)
	assert.Equal(t, doneID1, rm.Sections.Done[0].ID)
	assert.Equal(t, doneID2, rm.Sections.Done[1].ID)
	assert.Equal(t, id, rm.Sections.Done[2].ID, "lands LAST despite a low stale Position, not misordered into the middle")
	assert.Equal(t, 1, rm.Sections.Done[0].Position)
	assert.Equal(t, 2, rm.Sections.Done[1].Position)
	assert.Equal(t, 3, rm.Sections.Done[2].Position)

	md := roadmap.RenderMarkdown(rm, "component", nil, nil)
	idxOne := strings.Index(md, "### done one")
	idxTwo := strings.Index(md, "### done two")
	idxFinish := strings.Index(md, "### finish me")
	require.NotEqual(t, -1, idxOne)
	require.NotEqual(t, -1, idxTwo)
	require.NotEqual(t, -1, idxFinish)
	assert.Less(t, idxTwo, idxFinish, "finish me renders after both existing Done items")
}

func TestRemoveItem(t *testing.T) {
	rm := &roadmap.Roadmap{}
	id, err := roadmap.AddItem(rm, roadmap.SectionNext, roadmap.Item{Title: "gone"})
	require.NoError(t, err)

	require.NoError(t, roadmap.RemoveItem(rm, id))
	assert.Empty(t, rm.Sections.Next)

	err = roadmap.RemoveItem(rm, id)
	assert.Error(t, err, "removing an already-removed item errors")
}

// TestRemoveItemClosesGapInDenseSection verifies removing an item from a
// DENSE section closes the resulting gap in its 1..N numbering.
func TestRemoveItemClosesGapInDenseSection(t *testing.T) {
	rm := &roadmap.Roadmap{}
	idA, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "a"})
	require.NoError(t, err)
	idB, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "b"})
	require.NoError(t, err)
	idC, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "c"})
	require.NoError(t, err)
	require.NoError(t, roadmap.ReorderItem(rm, idA, 0)) // densifies: a=1, b=2, c=3

	require.NoError(t, roadmap.RemoveItem(rm, idB))

	require.Len(t, rm.Sections.Now, 2)
	assert.Equal(t, idA, rm.Sections.Now[0].ID)
	assert.Equal(t, 1, rm.Sections.Now[0].Position)
	assert.Equal(t, idC, rm.Sections.Now[1].ID)
	assert.Equal(t, 2, rm.Sections.Now[1].Position, "no gap left at the old position 3")
}

// TestRemoveItemLeavesLegacySectionUntouched verifies removing an item from
// a LEGACY (all-0) section doesn't spuriously promote it to DENSE.
func TestRemoveItemLeavesLegacySectionUntouched(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "a"})
	require.NoError(t, err)
	idB, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "b"})
	require.NoError(t, err)

	require.NoError(t, roadmap.RemoveItem(rm, idB))

	require.Len(t, rm.Sections.Now, 1)
	assert.Zero(t, rm.Sections.Now[0].Position)
}

func TestBlockedByRoundTrip(t *testing.T) {
	db := testDB(t)

	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{
		Title: "waiting on upstream",
		BlockedBy: []roadmap.Blocker{
			{Project: "breadboard", Ref: "B1-716", Note: "needs the sensor API"},
		},
	})
	require.NoError(t, err)
	require.NoError(t, roadmap.Save(db, "acme-corp", rm))

	loaded, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, loaded.Sections.Now, 1)
	require.Len(t, loaded.Sections.Now[0].BlockedBy, 1)
	assert.Equal(t, "breadboard", loaded.Sections.Now[0].BlockedBy[0].Project)
	assert.Equal(t, "B1-716", loaded.Sections.Now[0].BlockedBy[0].Ref)
	assert.Equal(t, "needs the sensor API", loaded.Sections.Now[0].BlockedBy[0].Note)
}

func TestRenderMarkdown(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{
		Title:     "active work",
		Body:      "doing the thing",
		Component: "widget",
		KB:        []int{1},
		Ticket:    []string{"tk-test-000"},
		BlockedBy: []roadmap.Blocker{{Project: "breadboard", Ref: "B1-716"}},
	})
	require.NoError(t, err)

	_, err = roadmap.AddItem(rm, roadmap.SectionParked, roadmap.Item{
		Title:         "deferred work",
		Why:           "waiting on decision",
		ResumeTrigger: "when the ticket resolves",
	})
	require.NoError(t, err)

	md := roadmap.RenderMarkdown(rm, "component", nil, nil)

	assert.Contains(t, md, "## In flight")
	assert.Contains(t, md, "## Next")
	assert.Contains(t, md, "## Deferred")
	assert.Contains(t, md, "## Parked")
	assert.Contains(t, md, "## Dropped")
	assert.Contains(t, md, "## Shipped")
	assert.Contains(t, md, "### active work")
	assert.Contains(t, md, "#### component: widget")
	assert.Contains(t, md, "kb: 1")
	assert.Contains(t, md, "ticket: tk-test-000")
	assert.Contains(t, md, "⛔ blocked by breadboard:B1-716")
	assert.Contains(t, md, "### deferred work")
	assert.Contains(t, md, "- Why: waiting on decision")
	assert.Contains(t, md, "- Resume trigger: when the ticket resolves")
}

func TestRenderMarkdownEmptySections(t *testing.T) {
	md := roadmap.RenderMarkdown(&roadmap.Roadmap{}, "component", nil, nil)
	assert.Contains(t, md, "_none_")
}

func TestRenderMarkdownDefaultAxisIsComponent(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "x", Component: "widget", Epic: "BB-1"})
	require.NoError(t, err)

	md := roadmap.RenderMarkdown(rm, "", nil, nil)
	assert.Contains(t, md, "#### component: widget")
	assert.Contains(t, md, "`epic: BB-1`")
}

func TestRenderMarkdownByEpicShowsComponentChipAndLabel(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "x", Component: "widget", Epic: "BB-1"})
	require.NoError(t, err)

	md := roadmap.RenderMarkdown(rm, "epic", map[string]string{"BB-1": "WiFi map"}, nil)
	assert.Contains(t, md, "#### epic: WiFi map")
	assert.Contains(t, md, "`component: widget`")
	assert.NotContains(t, md, "#### component: widget")
}

func TestRenderMarkdownByEpicFallsBackToRawIDWhenUnresolved(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "x", Epic: "BB-9"})
	require.NoError(t, err)

	md := roadmap.RenderMarkdown(rm, "epic", nil, nil)
	assert.Contains(t, md, "#### epic: BB-9")
}

// TestRenderMarkdownByComponentEpicChipShowsResolvedLabel verifies the
// inline epic chip (rendered when grouping by component) shows the SAME
// resolved epic label the epic-grouped header would show — not the raw
// epic id — when epicLabels resolves it.
func TestRenderMarkdownByComponentEpicChipShowsResolvedLabel(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "x", Component: "widget", Epic: "BB-1"})
	require.NoError(t, err)

	md := roadmap.RenderMarkdown(rm, "component", map[string]string{"BB-1": "WiFi map"}, nil)
	assert.Contains(t, md, "`epic: WiFi map`")
	assert.NotContains(t, md, "`epic: BB-1`")
}

// TestRenderMarkdownByComponentEpicChipFallsBackToRawIDWhenUnresolved
// verifies the inline epic chip falls back to the raw id, matching the
// epic-grouped header's fallback, when epicLabels doesn't resolve it.
func TestRenderMarkdownByComponentEpicChipFallsBackToRawIDWhenUnresolved(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "x", Component: "widget", Epic: "BB-9"})
	require.NoError(t, err)

	md := roadmap.RenderMarkdown(rm, "component", nil, nil)
	assert.Contains(t, md, "`epic: BB-9`")
}

func TestRenderMarkdownByEpicMarksBlockedEpic(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "x", Epic: "BB-1"})
	require.NoError(t, err)

	md := roadmap.RenderMarkdown(rm, "epic", nil, map[string]bool{"BB-1": true})
	assert.Contains(t, md, "#### epic: BB-1 ⛔")
}

// ── finding 5: empty sections marshal as [] not null ────────────────────────

func TestFreshRoadmapSectionsMarshalEmptyNotNull(t *testing.T) {
	db := testDB(t)

	rm, err := roadmap.Load(db, "brand-new-project")
	require.NoError(t, err)

	data, err := json.Marshal(rm)
	require.NoError(t, err)

	for _, field := range []string{`"now":[]`, `"next":[]`, `"parked":[]`, `"done":[]`} {
		assert.Contains(t, string(data), field)
	}
}

func TestNewRoadmapSectionsMarshalEmptyNotNull(t *testing.T) {
	data, err := json.Marshal(roadmap.New())
	require.NoError(t, err)

	for _, field := range []string{`"now":[]`, `"next":[]`, `"parked":[]`, `"done":[]`} {
		assert.Contains(t, string(data), field)
	}
}

// ── finding 3: 32KB content cap never wedges a save ─────────────────────────

func TestSaveTruncatesOversizedContent(t *testing.T) {
	db := testDB(t)

	rm := &roadmap.Roadmap{}
	const itemCount = 800
	// Built directly (bypassing AddItem) since itemCount exceeds OU-221's
	// maxRoadmapItems cap (500) -- this test exercises the separate,
	// unrelated content-column truncation path (renderContent/
	// store.MaxDocContentBytes), which the metadata column (and hence the
	// item-count cap) never enforces against.
	for i := 0; i < itemCount; i++ {
		rm.NextID++
		rm.Sections.Now = append(rm.Sections.Now, roadmap.Item{
			ID:    rm.NextID,
			Title: fmt.Sprintf("item %d", i),
			Body:  "a reasonably long body to help blow past the 32KB content cap for this test",
		})
	}

	require.NoError(t, roadmap.Save(db, "big-project", rm))

	doc, err := store.GetDocumentByKey(db, "roadmap", "big-project", "", "roadmap")
	require.NoError(t, err)
	require.NotNil(t, doc)

	assert.LessOrEqual(t, len(doc.Content), store.MaxDocContentBytes)
	assert.Contains(t, doc.Content, fmt.Sprintf("truncated; %d items", itemCount))

	raw, ok := doc.Metadata["data"]
	require.True(t, ok)
	var loaded roadmap.Roadmap
	require.NoError(t, json.Unmarshal([]byte(raw), &loaded))
	assert.Len(t, loaded.Sections.Now, itemCount, "metadata retains every item despite content truncation")
}

// ── finding 2: Mutate serializes the load-modify-save cycle ─────────────────

func TestMutateSequentialAddsBothSurvive(t *testing.T) {
	db := testDB(t)

	err := roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "first"})
		return err
	})
	require.NoError(t, err)

	err = roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "second"})
		return err
	})
	require.NoError(t, err)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 2)

	ids := map[int]bool{}
	for _, it := range rm.Sections.Now {
		ids[it.ID] = true
	}
	assert.Len(t, ids, 2, "distinct ids")
}

func TestMutateFnErrorRollsBack(t *testing.T) {
	db := testDB(t)

	err := roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, _ = roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "should not persist"})
		return fmt.Errorf("boom")
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	assert.Empty(t, rm.Sections.Now)
}

// ── error paths: Load/Save/Mutate propagate underlying store failures ──────

func TestLoadErrorPropagates(t *testing.T) {
	db := testDB(t)
	_, err := db.Exec("DROP TABLE documents")
	require.NoError(t, err)

	_, err = roadmap.Load(db, "acme-corp")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load roadmap")
}

func TestLoadDocWithCorruptMetadataErrors(t *testing.T) {
	db := testDB(t)

	_, err := store.UpsertDocument(db, store.Document{
		Type:     "roadmap",
		Project:  "acme-corp",
		Title:    "roadmap",
		Content:  "# Roadmap",
		Metadata: map[string]string{"data": "not json"},
	})
	require.NoError(t, err)

	_, err = roadmap.Load(db, "acme-corp")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse roadmap metadata")
}

func TestSaveErrorPropagates(t *testing.T) {
	db := testDB(t)
	require.NoError(t, db.Close())

	err := roadmap.Save(db, "acme-corp", roadmap.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "save roadmap")
}

func TestMutateBeginErrorPropagates(t *testing.T) {
	db := testDB(t)
	require.NoError(t, db.Close())

	err := roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error { return nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutate roadmap: begin")
}

func TestMutateLoadErrorPropagates(t *testing.T) {
	db := testDB(t)
	_, err := db.Exec("DROP TABLE documents")
	require.NoError(t, err)

	err = roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error { return nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutate roadmap:")
}

func TestMutateSaveErrorPropagates(t *testing.T) {
	db := testDB(t)

	_, err := db.Exec(`CREATE TRIGGER block_roadmap_insert BEFORE INSERT ON documents
		WHEN NEW.type = 'roadmap' BEGIN SELECT RAISE(ABORT, 'blocked for test'); END;`)
	require.NoError(t, err)

	err = roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, addErr := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "x"})
		return addErr
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutate roadmap:")
}

func TestMutateRebuildFTSErrorPropagates(t *testing.T) {
	db := testDB(t)
	_, err := db.Exec("DROP TABLE documents_fts")
	require.NoError(t, err)

	err = roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, addErr := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "x"})
		return addErr
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rebuild fts")
}

// ── v2: deferred/dropped states, lane, position, reorder ────────────────────

func TestValidSectionIncludesDeferredAndDropped(t *testing.T) {
	assert.True(t, roadmap.ValidSection(roadmap.SectionDeferred))
	assert.True(t, roadmap.ValidSection(roadmap.SectionDropped))
}

func TestAddItemToDeferredAndDropped(t *testing.T) {
	rm := &roadmap.Roadmap{}
	id1, err := roadmap.AddItem(rm, roadmap.SectionDeferred, roadmap.Item{Title: "deferred item"})
	require.NoError(t, err)
	id2, err := roadmap.AddItem(rm, roadmap.SectionDropped, roadmap.Item{Title: "dropped item"})
	require.NoError(t, err)

	require.Len(t, rm.Sections.Deferred, 1)
	assert.Equal(t, id1, rm.Sections.Deferred[0].ID)
	require.Len(t, rm.Sections.Dropped, 1)
	assert.Equal(t, id2, rm.Sections.Dropped[0].ID)
}

func TestMoveItemToDeferredAndDropped(t *testing.T) {
	rm := &roadmap.Roadmap{}
	id, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "movable"})
	require.NoError(t, err)

	require.NoError(t, roadmap.MoveItem(rm, id, roadmap.SectionDeferred))
	require.Len(t, rm.Sections.Deferred, 1)

	require.NoError(t, roadmap.MoveItem(rm, id, roadmap.SectionDropped))
	assert.Empty(t, rm.Sections.Deferred)
	require.Len(t, rm.Sections.Dropped, 1)
}

func TestReorderItem(t *testing.T) {
	rm := &roadmap.Roadmap{}
	id, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "x"})
	require.NoError(t, err)

	// A single item in the section: any target index clamps to 0, so its
	// densely-renumbered Position is always 1.
	require.NoError(t, roadmap.ReorderItem(rm, id, 5))
	assert.Equal(t, 1, rm.Sections.Now[0].Position)
	assert.NotEmpty(t, rm.Sections.Now[0].UpdatedAt)
}

// TestReorderItemMovesToTopAmongDefaultPositionSiblings is the HIGH-severity
// regression case: with ≥2 siblings still at the legacy/unset Position 0,
// reorder(id, 0) must physically move the item to the top, not no-op
// because "0 == 0" looked like a tie under the old raw-value semantics.
func TestReorderItemMovesToTopAmongDefaultPositionSiblings(t *testing.T) {
	rm := &roadmap.Roadmap{}
	idA, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "a"})
	require.NoError(t, err)
	idB, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "b"})
	require.NoError(t, err)
	idC, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "c"})
	require.NoError(t, err)
	for _, it := range rm.Sections.Now {
		require.Zero(t, it.Position, "siblings start at the legacy default")
	}

	// Move "c" (currently last) to the top.
	require.NoError(t, roadmap.ReorderItem(rm, idC, 0))

	require.Len(t, rm.Sections.Now, 3)
	assert.Equal(t, idC, rm.Sections.Now[0].ID, "c physically moved to the top of the slice")
	assert.Equal(t, idA, rm.Sections.Now[1].ID)
	assert.Equal(t, idB, rm.Sections.Now[2].ID)
	// Densely renumbered 1..N — no ties, no ambiguity for the next sort.
	assert.Equal(t, 1, rm.Sections.Now[0].Position)
	assert.Equal(t, 2, rm.Sections.Now[1].Position)
	assert.Equal(t, 3, rm.Sections.Now[2].Position)

	md := roadmap.RenderMarkdown(rm, "component", nil, nil)
	idxC := strings.Index(md, "### c")
	idxA := strings.Index(md, "### a")
	require.NotEqual(t, -1, idxC)
	require.NotEqual(t, -1, idxA)
	assert.Less(t, idxC, idxA, "c renders before a after reordering to the top")
}

func TestReorderItemNotFound(t *testing.T) {
	rm := &roadmap.Roadmap{}
	err := roadmap.ReorderItem(rm, 999, 1)
	assert.Error(t, err)
}

// TestReorderItemNegativePositionClampsToTop verifies a negative target
// index clamps to 0 (top) rather than panicking or being rejected.
func TestReorderItemNegativePositionClampsToTop(t *testing.T) {
	rm := &roadmap.Roadmap{}
	idA, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "a"})
	require.NoError(t, err)
	idB, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "b"})
	require.NoError(t, err)

	require.NoError(t, roadmap.ReorderItem(rm, idB, -5))

	require.Len(t, rm.Sections.Now, 2)
	assert.Equal(t, idB, rm.Sections.Now[0].ID)
	assert.Equal(t, idA, rm.Sections.Now[1].ID)
}

func TestMoveItemWithPosition(t *testing.T) {
	rm := &roadmap.Roadmap{}
	id, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "x"})
	require.NoError(t, err)

	// A single item lands in an empty target section: any target index
	// clamps to 0, so its densely-renumbered Position is always 1.
	require.NoError(t, roadmap.MoveItem(rm, id, roadmap.SectionParked, 3))
	require.Len(t, rm.Sections.Parked, 1)
	assert.Equal(t, 1, rm.Sections.Parked[0].Position)
}

func TestMoveItemSameSectionWithPositionSplicesAndRenumbers(t *testing.T) {
	rm := &roadmap.Roadmap{}
	id1, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "first"})
	require.NoError(t, err)
	_, err = roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "second"})
	require.NoError(t, err)

	// Same section, but an explicit position is supplied: unlike the no-op
	// same-section MoveItem call, this must still apply — position 7
	// clamps to the end (index 1, after "second" is removed-and-reinserted
	// ahead of it), so "first" ends up last, densely renumbered to 2.
	require.NoError(t, roadmap.MoveItem(rm, id1, roadmap.SectionNow, 7))
	require.Len(t, rm.Sections.Now, 2)
	assert.Equal(t, "second", rm.Sections.Now[0].Title)
	assert.Equal(t, "first", rm.Sections.Now[1].Title)
	assert.Equal(t, 1, rm.Sections.Now[0].Position)
	assert.Equal(t, 2, rm.Sections.Now[1].Position)
}

func TestUpdateItemEpic(t *testing.T) {
	rm := &roadmap.Roadmap{}
	id, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "x"})
	require.NoError(t, err)

	epic := "BB-707"
	require.NoError(t, roadmap.UpdateItem(rm, id, roadmap.Patch{Epic: &epic}))
	assert.Equal(t, "BB-707", rm.Sections.Now[0].Epic)
}

// TestAddItemWithEpicAndPosition verifies AddItem's optional position is a
// 0-based target index (mirroring MoveItem/ReorderItem — presence, not
// item.Position, signals intent): on a NON-empty section, index 0 splices
// the new item to the front and densely renumbers the whole section.
func TestAddItemWithEpicAndPosition(t *testing.T) {
	rm := &roadmap.Roadmap{}
	firstID, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "existing"})
	require.NoError(t, err)

	id, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "x", Epic: "BB-707"}, 0)
	require.NoError(t, err)

	require.Len(t, rm.Sections.Now, 2)
	assert.Equal(t, id, rm.Sections.Now[0].ID, "position 0 splices the new item to the front")
	assert.Equal(t, "BB-707", rm.Sections.Now[0].Epic)
	assert.Equal(t, 1, rm.Sections.Now[0].Position)
	assert.Equal(t, firstID, rm.Sections.Now[1].ID)
	assert.Equal(t, 2, rm.Sections.Now[1].Position)
}

// TestAddItemNoPositionAppendsLast verifies AddItem with no position given
// plain-appends: last in a legacy (all-0) section, and last (densely
// renumbered) in an already-dense section.
func TestAddItemNoPositionAppendsLast(t *testing.T) {
	rm := &roadmap.Roadmap{}
	idA, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "a"})
	require.NoError(t, err)
	idB, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "b"}, 0) // makes the section dense
	require.NoError(t, err)
	idC, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "c"})
	require.NoError(t, err)

	require.Len(t, rm.Sections.Now, 3)
	assert.Equal(t, idB, rm.Sections.Now[0].ID)
	assert.Equal(t, idA, rm.Sections.Now[1].ID)
	assert.Equal(t, idC, rm.Sections.Now[2].ID, "no-position add lands last in a dense section")
	assert.Equal(t, 1, rm.Sections.Now[0].Position)
	assert.Equal(t, 2, rm.Sections.Now[1].Position)
	assert.Equal(t, 3, rm.Sections.Now[2].Position)
}

// TestAddItemIgnoresStaleItemPositionField verifies AddItem no longer
// treats item.Position as an ordering input — only the variadic position
// controls placement; a caller-set item.Position is reset.
func TestAddItemIgnoresStaleItemPositionField(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "a"})
	require.NoError(t, err)

	id, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "b", Position: 99})
	require.NoError(t, err)

	require.Len(t, rm.Sections.Now, 2)
	assert.Equal(t, id, rm.Sections.Now[1].ID, "stale item.Position isn't an ordering input -- still appends last")
	assert.Zero(t, rm.Sections.Now[1].Position, "the section stays legacy/all-0; item.Position is reset, not carried through")
}

// ── render: axis grouping + position ordering ────────────────────────────────

func TestRenderMarkdownGroupsByComponentAndOrdersByPosition(t *testing.T) {
	db := testDB(t)

	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "ungrouped"})
	require.NoError(t, err)
	_, err = roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "backend second", Component: "backend"})
	require.NoError(t, err)
	_, err = roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "backend first", Component: "backend"})
	require.NoError(t, err)
	_, err = roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "frontend only", Component: "frontend"})
	require.NoError(t, err)
	// Set the explicit sort order directly on the section slice -- AddItem
	// no longer takes a raw Position value as ordering input (see
	// TestAddItemIgnoresStaleItemPositionField); this test is exercising
	// canonicalizeSections'/RenderMarkdown's Position-sort, not AddItem.
	rm.Sections.Now[1].Position = 2 // backend second
	rm.Sections.Now[2].Position = 1 // backend first

	// Storage order is purely Position-sorted (see canonicalizeSections);
	// RenderMarkdown does the axis grouping at render time (groupByAxis).
	require.NoError(t, roadmap.Save(db, "acme-corp", rm))
	loaded, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)

	md := roadmap.RenderMarkdown(loaded, "component", nil, nil)

	assert.Contains(t, md, "#### component: backend")
	assert.Contains(t, md, "#### component: frontend")
	ungroupedIdx := strings.Index(md, "### ungrouped")
	backendHeadingIdx := strings.Index(md, "#### component: backend")
	require.NotEqual(t, -1, ungroupedIdx)
	require.NotEqual(t, -1, backendHeadingIdx)
	assert.Less(t, ungroupedIdx, backendHeadingIdx, "ungrouped items render before named groups")

	firstIdx := strings.Index(md, "### backend first")
	secondIdx := strings.Index(md, "### backend second")
	require.NotEqual(t, -1, firstIdx)
	require.NotEqual(t, -1, secondIdx)
	assert.Less(t, firstIdx, secondIdx, "position 1 renders before position 2 within the same group")
}

func TestRenderMarkdownDeferredAndDroppedSections(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionDeferred, roadmap.Item{Title: "deferred item", Why: "later"})
	require.NoError(t, err)
	_, err = roadmap.AddItem(rm, roadmap.SectionDropped, roadmap.Item{Title: "dropped item"})
	require.NoError(t, err)

	md := roadmap.RenderMarkdown(rm, "component", nil, nil)
	assert.Contains(t, md, "### deferred item")
	assert.Contains(t, md, "- Why: later")
	assert.Contains(t, md, "### dropped item")
}

// ── back-compat: a v1 doc (no epic/position/schema_version/deferred/dropped)
// still loads, and re-saves upgraded without losing data ───────────────────

func TestLoadV1DocRoundTrips(t *testing.T) {
	db := testDB(t)

	v1JSON := `{"sections":{"now":[{"id":1,"title":"legacy item","body":"legacy body","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"}],"next":[],"parked":[],"done":[]},"next_id":1}`
	_, err := store.UpsertDocument(db, store.Document{
		Type:     "roadmap",
		Project:  "acme-corp",
		Title:    "roadmap",
		Content:  "# Roadmap",
		Metadata: map[string]string{"data": v1JSON},
	})
	require.NoError(t, err)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	assert.Equal(t, 0, rm.SchemaVersion, "v1 doc loads with schema_version unset")
	require.Len(t, rm.Sections.Now, 1)
	assert.Equal(t, "legacy item", rm.Sections.Now[0].Title)
	assert.Equal(t, "legacy body", rm.Sections.Now[0].Body)
	assert.Empty(t, rm.Sections.Deferred)
	assert.Empty(t, rm.Sections.Dropped)

	require.NoError(t, roadmap.Save(db, "acme-corp", rm))

	reloaded, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	assert.Equal(t, 2, reloaded.SchemaVersion, "re-save upgrades to the current schema version")
	require.Len(t, reloaded.Sections.Now, 1)
	assert.Equal(t, "legacy item", reloaded.Sections.Now[0].Title, "v1 data survives the upgrade")
	assert.Equal(t, "legacy body", reloaded.Sections.Now[0].Body)
	assert.Empty(t, reloaded.Sections.Now[0].Epic, "no epic on an upgraded v1 item")
	assert.Zero(t, reloaded.Sections.Now[0].Position, "no position on an upgraded v1 item")
}

// ── canonical order on Save: storage carries pure Position order; render
// groups on top of it (component vs epic can each regroup the same items) ──

func TestSaveCanonicalizesToPositionOrderOnly(t *testing.T) {
	db := testDB(t)

	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "third"})
	require.NoError(t, err)
	_, err = roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "first", Component: "backend"})
	require.NoError(t, err)
	_, err = roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "second", Component: "frontend"})
	require.NoError(t, err)
	// Set the explicit sort order directly on the section slice -- AddItem
	// no longer takes a raw Position value as ordering input.
	rm.Sections.Now[0].Position = 3 // third
	rm.Sections.Now[1].Position = 1 // first
	rm.Sections.Now[2].Position = 2 // second

	require.NoError(t, roadmap.Save(db, "acme-corp", rm))

	loaded, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)

	// Storage order is pure Position order — no axis grouping baked in, so
	// the same stored order can be regrouped either way at render time.
	wantTitles := make([]string, len(loaded.Sections.Now))
	for i, it := range loaded.Sections.Now {
		wantTitles[i] = it.Title
	}
	assert.Equal(t, []string{"first", "second", "third"}, wantTitles)

	mdByComponent := roadmap.RenderMarkdown(loaded, "component", nil, nil)
	assert.Contains(t, mdByComponent, "#### component: backend")
	assert.Contains(t, mdByComponent, "#### component: frontend")
}

// ── finding 2: same-section MoveItem with a position doesn't re-append,
// so it doesn't disturb same-Position tie-break order among siblings ───────

// TestMoveItemSameSectionWithPositionTargetsIndexAmongSiblings verifies
// that position is a literal 0-based target index within the OTHER
// siblings (the item being moved is removed first, then reinserted) — not
// a raw sort-key value compared against the siblings' existing Position.
func TestMoveItemSameSectionWithPositionTargetsIndexAmongSiblings(t *testing.T) {
	rm := &roadmap.Roadmap{}
	idA, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "a"})
	require.NoError(t, err)
	idB, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "b"})
	require.NoError(t, err)
	idC, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "c"})
	require.NoError(t, err)

	// Move "a" to index 1 among the remaining siblings [b, c] -> b, a, c.
	require.NoError(t, roadmap.MoveItem(rm, idA, roadmap.SectionNow, 1))

	require.Len(t, rm.Sections.Now, 3)
	assert.Equal(t, idB, rm.Sections.Now[0].ID)
	assert.Equal(t, idA, rm.Sections.Now[1].ID)
	assert.Equal(t, idC, rm.Sections.Now[2].ID)
	assert.Equal(t, 1, rm.Sections.Now[0].Position)
	assert.Equal(t, 2, rm.Sections.Now[1].Position)
	assert.Equal(t, 3, rm.Sections.Now[2].Position)

	md := roadmap.RenderMarkdown(rm, "component", nil, nil)
	idxB := strings.Index(md, "### b")
	idxA := strings.Index(md, "### a")
	idxC := strings.Index(md, "### c")
	require.NotEqual(t, -1, idxA)
	require.NotEqual(t, -1, idxB)
	require.NotEqual(t, -1, idxC)
	assert.Less(t, idxB, idxA, "b renders before a")
	assert.Less(t, idxA, idxC, "a renders before c")
}

// ── canonicalization is deterministic and idempotent ────────────────────────

func TestSaveCanonicalizationIsIdempotent(t *testing.T) {
	db := testDB(t)

	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "ungrouped"})
	require.NoError(t, err)
	_, err = roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "backend second", Component: "backend"})
	require.NoError(t, err)
	_, err = roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "backend first", Component: "backend"})
	require.NoError(t, err)
	rm.Sections.Now[1].Position = 2 // backend second
	rm.Sections.Now[2].Position = 1 // backend first

	require.NoError(t, roadmap.Save(db, "acme-corp", rm))
	doc1, err := store.GetDocumentByKey(db, "roadmap", "acme-corp", "", "roadmap")
	require.NoError(t, err)
	data1 := doc1.Metadata["data"]

	loaded, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.NoError(t, roadmap.Save(db, "acme-corp", loaded))

	doc2, err := store.GetDocumentByKey(db, "roadmap", "acme-corp", "", "roadmap")
	require.NoError(t, err)
	data2 := doc2.Metadata["data"]

	assert.JSONEq(t, data1, data2, "re-saving an already-canonical roadmap doesn't churn stored order")
	assert.Equal(t, doc1.Content, doc2.Content, "markdown mirror is stable across a re-save")
}

func TestMutateConcurrentAddsAllSurvive(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "roadmap.db")

	db, err := store.InitDB(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	const n = 20
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = roadmap.Mutate(db, "concurrent-project", func(rm *roadmap.Roadmap) error {
				_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: fmt.Sprintf("item %d", i)})
				return err
			})
		}(i)
	}
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}

	rm, err := roadmap.Load(db, "concurrent-project")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, n, "no lost updates")

	ids := map[int]bool{}
	for _, it := range rm.Sections.Now {
		ids[it.ID] = true
	}
	assert.Len(t, ids, n, "distinct ids, no duplicates")
}

// ── Filter: component/epic axis filtering ───────────────────────────────────

func TestFilterNoFiltersReturnsSameRoadmap(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "x"})
	require.NoError(t, err)

	filtered := roadmap.Filter(rm, "", "")
	assert.Same(t, rm, filtered, "no filters is a no-op returning the same pointer")
}

func TestFilterByComponent(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "widget item", Component: "widget"})
	require.NoError(t, err)
	_, err = roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "gadget item", Component: "gadget"})
	require.NoError(t, err)

	filtered := roadmap.Filter(rm, "widget", "")
	require.Len(t, filtered.Sections.Now, 1)
	assert.Equal(t, "widget item", filtered.Sections.Now[0].Title)
}

func TestFilterByEpic(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "epic item", Epic: "BB-707"})
	require.NoError(t, err)
	_, err = roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "other item", Epic: "BB-1"})
	require.NoError(t, err)

	filtered := roadmap.Filter(rm, "", "BB-707")
	require.Len(t, filtered.Sections.Now, 1)
	assert.Equal(t, "epic item", filtered.Sections.Now[0].Title)
}

func TestFilterByComponentAndEpicBothMustMatch(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "matches both", Component: "widget", Epic: "BB-707"})
	require.NoError(t, err)
	_, err = roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "component only", Component: "widget"})
	require.NoError(t, err)

	filtered := roadmap.Filter(rm, "widget", "BB-707")
	require.Len(t, filtered.Sections.Now, 1)
	assert.Equal(t, "matches both", filtered.Sections.Now[0].Title)
}

func TestFilterAcrossAllSections(t *testing.T) {
	rm := &roadmap.Roadmap{}
	for _, s := range []roadmap.Section{roadmap.SectionNow, roadmap.SectionNext, roadmap.SectionDeferred, roadmap.SectionParked, roadmap.SectionDropped, roadmap.SectionDone} {
		_, err := roadmap.AddItem(rm, s, roadmap.Item{Title: string(s) + " item", Component: "widget"})
		require.NoError(t, err)
	}

	filtered := roadmap.Filter(rm, "widget", "")
	assert.Len(t, filtered.Sections.Now, 1)
	assert.Len(t, filtered.Sections.Next, 1)
	assert.Len(t, filtered.Sections.Deferred, 1)
	assert.Len(t, filtered.Sections.Parked, 1)
	assert.Len(t, filtered.Sections.Dropped, 1)
	assert.Len(t, filtered.Sections.Done, 1)
}

// ── EpicIDs ──────────────────────────────────────────────────────────────────

func TestEpicIDsDistinctFirstAppearanceOrder(t *testing.T) {
	rm := &roadmap.Roadmap{}
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "a", Epic: "BB-2"})
	require.NoError(t, err)
	_, err = roadmap.AddItem(rm, roadmap.SectionNext, roadmap.Item{Title: "b", Epic: "BB-1"})
	require.NoError(t, err)
	_, err = roadmap.AddItem(rm, roadmap.SectionDone, roadmap.Item{Title: "c", Epic: "BB-2"})
	require.NoError(t, err)
	_, err = roadmap.AddItem(rm, roadmap.SectionDone, roadmap.Item{Title: "d"})
	require.NoError(t, err)

	assert.Equal(t, []string{"BB-2", "BB-1"}, roadmap.EpicIDs(rm))
}

func TestEpicIDsEmptyRoadmap(t *testing.T) {
	assert.Empty(t, roadmap.EpicIDs(&roadmap.Roadmap{}))
}

// ── OU-221: item-count cap with archive-done guidance ───────────────────────

// roadmapItemCap mirrors the unexported maxRoadmapItems (500) for
// black-box tests -- kept as a local literal rather than reaching into the
// package so this file stays in package roadmap_test.
const roadmapItemCap = 500

// fillDoneItems appends roadmapItemCap bare items directly to rm's Done
// section (bypassing AddItem, which enforces the cap these tests exercise)
// so tests can cheaply put a roadmap AT the cap without paying for hundreds
// of real AddItem calls.
func fillDoneItems(rm *roadmap.Roadmap) {
	for i := 0; i < roadmapItemCap; i++ {
		rm.NextID++
		rm.Sections.Done = append(rm.Sections.Done, roadmap.Item{ID: rm.NextID, Title: fmt.Sprintf("done %d", rm.NextID)})
	}
}

func TestAddItemRejectsAtCap(t *testing.T) {
	rm := &roadmap.Roadmap{}
	fillDoneItems(rm)

	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "one too many"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("%d-item cap", roadmapItemCap))
	assert.Contains(t, err.Error(), "archive or remove done items")
	assert.Contains(t, err.Error(), "op=remove")
	assert.Len(t, rm.Sections.Now, 0, "the rejected item is never added")
}

func TestAddItemAfterArchivingDoneItemSucceeds(t *testing.T) {
	rm := &roadmap.Roadmap{}
	fillDoneItems(rm)

	// Archive (remove) one done item to free a slot -- the remediation the
	// cap error instructs.
	freedID := rm.Sections.Done[0].ID
	require.NoError(t, roadmap.RemoveItem(rm, freedID))

	id, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "fits now"})
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 1)
	assert.Equal(t, id, rm.Sections.Now[0].ID)
}

func TestSeedRejectsAtCap(t *testing.T) {
	rm := &roadmap.Roadmap{}
	fillDoneItems(rm)

	_, err := roadmap.Seed(rm, []backlog.Item{{ID: "tk-test-001", Title: "new", Priority: "P0"}}, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("%d-item cap", roadmapItemCap))
}

// TestNonGrowingOpsUnaffectedAtCap verifies update/reorder/remove/done-move
// all still work at (or over) the cap -- only item-count-GROWING operations
// (AddItem, and transitively Seed) are gated.
func TestNonGrowingOpsUnaffectedAtCap(t *testing.T) {
	rm := &roadmap.Roadmap{}
	fillDoneItems(rm)
	id := rm.Sections.Done[0].ID

	// update
	newTitle := "renamed while at cap"
	require.NoError(t, roadmap.UpdateItem(rm, id, roadmap.Patch{Title: &newTitle}))
	assert.Equal(t, newTitle, rm.Sections.Done[0].Title)

	// reorder (within the same section, no count change)
	require.NoError(t, roadmap.ReorderItem(rm, id, 1))

	// move (done -> parked -> done; total count never changes)
	require.NoError(t, roadmap.MoveItem(rm, id, roadmap.SectionParked))
	require.NoError(t, roadmap.MarkDone(rm, id))
	require.Len(t, rm.Sections.Done, roadmapItemCap)

	// remove (shrinks, always allowed)
	require.NoError(t, roadmap.RemoveItem(rm, id))
	require.Len(t, rm.Sections.Done, roadmapItemCap-1)
}

// TestAddItemNormalSizeRoadmapUnaffected verifies a handful of items per
// section -- ordinary usage -- is nowhere near the cap and every op
// (add/move/reorder/update) behaves exactly as before this change.
func TestAddItemNormalSizeRoadmapUnaffected(t *testing.T) {
	rm := &roadmap.Roadmap{}
	for _, s := range []roadmap.Section{roadmap.SectionNow, roadmap.SectionNext, roadmap.SectionDeferred, roadmap.SectionParked, roadmap.SectionDropped, roadmap.SectionDone} {
		for i := 0; i < 5; i++ {
			_, err := roadmap.AddItem(rm, s, roadmap.Item{Title: fmt.Sprintf("%s item %d", s, i)})
			require.NoError(t, err)
		}
	}
	require.Len(t, rm.Sections.Now, 5)
	require.Len(t, rm.Sections.Done, 5)

	id := rm.Sections.Now[0].ID
	require.NoError(t, roadmap.MoveItem(rm, id, roadmap.SectionParked))
	require.NoError(t, roadmap.ReorderItem(rm, rm.Sections.Parked[0].ID, 0))
	title := "still fine"
	require.NoError(t, roadmap.UpdateItem(rm, rm.Sections.Parked[0].ID, roadmap.Patch{Title: &title}))
}
