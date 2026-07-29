package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/roadmap"
	"dangernoodle.io/ouroboros/internal/store"
	"dangernoodle.io/ouroboros/internal/testutil"
)

func TestGet_UnknownDomain_Errors(t *testing.T) {
	db := testutil.TestDB(t)

	_, err := Get(db, Request{})
	require.EqualError(t, err, ErrDomainRequired)

	_, err = Get(db, Request{Domain: "bogus"})
	require.EqualError(t, err, ErrDomainRequired)
}

func TestGet_DomainKB_IDsFetch(t *testing.T) {
	db := testutil.TestDB(t)

	doc, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "Use PostgreSQL", Content: "c1", Notes: "narrative"})
	require.NoError(t, err)

	res, err := Get(db, Request{Domain: "kb", IDs: []any{float64(doc.ID)}})
	require.NoError(t, err)
	require.Len(t, res.Docs, 1)
	got := res.Docs[0]
	assert.Equal(t, "Use PostgreSQL", got.Title)
	assert.Empty(t, got.Notes) // stripped, non-verbose
	assert.Empty(t, got.Edges)

	res, err = Get(db, Request{Domain: "kb", IDs: []any{float64(doc.ID)}, Verbose: true})
	require.NoError(t, err)
	require.Len(t, res.Docs, 1)
	withEdges := res.Docs[0]
	assert.Equal(t, "narrative", withEdges.Notes)
}

func TestGet_DomainKB_IDsWrongType_Errors(t *testing.T) {
	db := testutil.TestDB(t)

	_, err := Get(db, Request{Domain: "kb", IDs: []any{"not-a-number"}})
	require.EqualError(t, err, "ids for domain=kb must be integers")
}

func TestGet_DomainKB_FilterList(t *testing.T) {
	db := testutil.TestDB(t)
	_, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "d1", Content: "c1"})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{Type: "fact", Project: "acme-corp", Title: "f1", Content: "c2"})
	require.NoError(t, err)

	res, err := Get(db, Request{Domain: "kb", Types: []string{"decision"}})
	require.NoError(t, err)
	require.Len(t, res.DocSummaries, 1)
	assert.Equal(t, "d1", res.DocSummaries[0].Title)
}

func TestGet_DomainBacklog_IDsFetch(t *testing.T) {
	db := testutil.TestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "fix the bug", "desc", "narrative", "", "")
	require.NoError(t, err)

	res, err := Get(db, Request{Domain: "backlog", IDs: []any{item.ID}})
	require.NoError(t, err)
	require.Len(t, res.ItemsJSON, 1)
	got := res.ItemsJSON[0]
	assert.Equal(t, "fix the bug", got.Title)
	assert.Empty(t, got.Notes)
	assert.Empty(t, got.Edges)

	res, err = Get(db, Request{Domain: "backlog", IDs: []any{item.ID}, Verbose: true})
	require.NoError(t, err)
	require.Len(t, res.ItemsJSON, 1)
	withEdges := res.ItemsJSON[0]
	assert.Equal(t, "narrative", withEdges.Notes)
}

func TestGet_DomainBacklog_IDsWrongType_Errors(t *testing.T) {
	db := testutil.TestDB(t)

	_, err := Get(db, Request{Domain: "backlog", IDs: []any{float64(728)}})
	require.EqualError(t, err, `ids for domain=backlog must be prefixed strings like "B1-728"`)
}

func TestGet_DomainBacklog_FilterList(t *testing.T) {
	db := testutil.TestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item one", "desc", "", "", "")
	require.NoError(t, err)

	res, err := Get(db, Request{Domain: "backlog", Projects: []string{"acme-corp"}})
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	assert.Equal(t, "item one", res.Items[0].Title)
}

func TestGet_DomainBacklog_FilterList_BadPriority_Errors(t *testing.T) {
	db := testutil.TestDB(t)

	_, err := Get(db, Request{Domain: "backlog", PriorityMin: "bogus"})
	require.Error(t, err)
}

// TestGet_DomainBacklog_FilterList_InvertedPriorityRange_Errors is the
// OU-347 regression on Get's list-mode path (the ticket's reported case):
// min=P2/max=P1 is unsatisfiable and previously returned zero rows silently.
func TestGet_DomainBacklog_FilterList_InvertedPriorityRange_Errors(t *testing.T) {
	db := testutil.TestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item one", "desc", "", "", "")
	require.NoError(t, err)

	_, err = Get(db, Request{Domain: "backlog", PriorityMin: "P2", PriorityMax: "P1"})
	require.EqualError(t, err, "invalid priority range: priority_min P2 is lower severity than priority_max P1 (P0 highest .. P6 lowest)")
}

func TestGet_DomainBacklog_FilterList_UnknownProject_Errors(t *testing.T) {
	db := testutil.TestDB(t)

	_, err := Get(db, Request{Domain: "backlog", Projects: []string{"nonexistent"}})
	require.Error(t, err)
}

func TestGet_DomainRoadmap_Structured(t *testing.T) {
	db := testutil.TestDB(t)
	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "Structured item"})
		return err
	}))

	res, err := Get(db, Request{Domain: "roadmap", Projects: []string{"acme-corp"}})
	require.NoError(t, err)
	require.NotNil(t, res.Roadmap)
	require.Len(t, res.Roadmap.Sections.Now, 1)
	assert.Equal(t, "Structured item", res.Roadmap.Sections.Now[0].Title)
}

func TestGet_DomainRoadmap_Markdown(t *testing.T) {
	db := testutil.TestDB(t)
	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "MD item"})
		return err
	}))

	res, err := Get(db, Request{Domain: "roadmap", Projects: []string{"acme-corp"}, Format: "md"})
	require.NoError(t, err)
	assert.Contains(t, res.Text, "# Roadmap")
	assert.Contains(t, res.Text, "MD item")
}

func TestGet_DomainRoadmap_HTML(t *testing.T) {
	db := testutil.TestDB(t)
	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "HTML item", Component: "widget"})
		return err
	}))

	res, err := Get(db, Request{Domain: "roadmap", Projects: []string{"acme-corp"}, Format: "html"})
	require.NoError(t, err)
	assert.Contains(t, res.Text, "<style>")
	assert.Contains(t, res.Text, "HTML item")
	assert.Contains(t, res.Text, "component: widget")
}

func TestGet_DomainRoadmap_MissingProject_Errors(t *testing.T) {
	db := testutil.TestDB(t)

	_, err := Get(db, Request{Domain: "roadmap"})
	require.EqualError(t, err, "projects[0] (project name) is required for domain=roadmap")
}
