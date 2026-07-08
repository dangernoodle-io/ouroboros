package roadmap_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func TestRemoveItem(t *testing.T) {
	rm := &roadmap.Roadmap{}
	id, err := roadmap.AddItem(rm, roadmap.SectionNext, roadmap.Item{Title: "gone"})
	require.NoError(t, err)

	require.NoError(t, roadmap.RemoveItem(rm, id))
	assert.Empty(t, rm.Sections.Next)

	err = roadmap.RemoveItem(rm, id)
	assert.Error(t, err, "removing an already-removed item errors")
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

	md := roadmap.RenderMarkdown(rm)

	assert.Contains(t, md, "## Now")
	assert.Contains(t, md, "## Next")
	assert.Contains(t, md, "## Parked")
	assert.Contains(t, md, "## Recently done")
	assert.Contains(t, md, "### active work")
	assert.Contains(t, md, "`component: widget`")
	assert.Contains(t, md, "kb: 1")
	assert.Contains(t, md, "ticket: tk-test-000")
	assert.Contains(t, md, "⛔ blocked by breadboard:B1-716")
	assert.Contains(t, md, "### deferred work")
	assert.Contains(t, md, "- Why: waiting on decision")
	assert.Contains(t, md, "- Resume trigger: when the ticket resolves")
}

func TestRenderMarkdownEmptySections(t *testing.T) {
	md := roadmap.RenderMarkdown(&roadmap.Roadmap{})
	assert.Contains(t, md, "_none_")
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
	for i := 0; i < itemCount; i++ {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{
			Title: fmt.Sprintf("item %d", i),
			Body:  "a reasonably long body to help blow past the 32KB content cap for this test",
		})
		require.NoError(t, err)
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
