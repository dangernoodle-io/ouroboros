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

func TestSearch_UnknownDomain_Errors(t *testing.T) {
	db := testutil.TestDB(t)

	_, err := Search(db, Request{})
	require.EqualError(t, err, ErrDomainRequired)
}

func TestSearch_DomainKB_SingleQuery(t *testing.T) {
	db := testutil.TestDB(t)
	_, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "ouroboros", Title: "tiktoken", Content: "Use tiktoken for token counting"})
	require.NoError(t, err)

	res, err := Search(db, Request{Domain: "kb", Query: "tiktoken"})
	require.NoError(t, err)
	require.Len(t, res.DocSummaries, 1)
	assert.Equal(t, "tiktoken", res.DocSummaries[0].Title)
}

func TestSearch_DomainKB_QueriesBatch(t *testing.T) {
	db := testutil.TestDB(t)
	_, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "alpha widget", Content: "c"})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "beta gadget", Content: "c"})
	require.NoError(t, err)

	res, err := Search(db, Request{Domain: "kb", Queries: []string{"widget", "gadget", "nonexistentterm"}})
	require.NoError(t, err)
	require.Len(t, res.DocSummarySets, 3)
	require.Len(t, res.DocSummarySets[0], 1)
	assert.Equal(t, "alpha widget", res.DocSummarySets[0][0].Title)
	require.Len(t, res.DocSummarySets[1], 1)
	assert.Equal(t, "beta gadget", res.DocSummarySets[1][0].Title)
	assert.NotNil(t, res.DocSummarySets[2]) // empty-not-nil invariant
	assert.Len(t, res.DocSummarySets[2], 0)
}

func TestSearch_DomainKB_NeitherQueryNorQueries_Errors(t *testing.T) {
	db := testutil.TestDB(t)

	_, err := Search(db, Request{Domain: "kb"})
	require.EqualError(t, err, "query or queries is required")
}

// TestSearch_DomainKB_QueryWithTags is the OU-330 regression: a non-empty
// full-text query combined with a tags filter must only return docs
// matching BOTH, and Score must be populated.
func TestSearch_DomainKB_QueryWithTags(t *testing.T) {
	db := testutil.TestDB(t)
	_, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "widget one", Content: "c", Tags: []string{"release"}})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "widget two", Content: "c", Tags: []string{"ops"}})
	require.NoError(t, err)

	res, err := Search(db, Request{Domain: "kb", Query: "widget", Tags: []string{"release"}})
	require.NoError(t, err)
	require.Len(t, res.DocSummaries, 1)
	assert.Equal(t, "widget one", res.DocSummaries[0].Title)
	assert.NotZero(t, res.DocSummaries[0].Score)
}

// TestSearch_DomainKB_QueryNoTags_RowsPreserved_ScorePopulated pins the
// no-filter shape: rows and count must be identical to a query with no tags
// set (empty Tags is a no-op, matching pre-OU-330 behavior), and additionally
// confirms bm25 Score is now populated on this path (previously always 0).
func TestSearch_DomainKB_QueryNoTags_RowsPreserved_ScorePopulated(t *testing.T) {
	db := testutil.TestDB(t)
	_, err := store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "widget one", Content: "c", Tags: []string{"release"}})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "widget two", Content: "c", Tags: []string{"ops"}})
	require.NoError(t, err)

	res, err := Search(db, Request{Domain: "kb", Query: "widget"})
	require.NoError(t, err)
	require.Len(t, res.DocSummaries, 2)
	titles := []string{res.DocSummaries[0].Title, res.DocSummaries[1].Title}
	assert.Contains(t, titles, "widget one")
	assert.Contains(t, titles, "widget two")
	assert.NotZero(t, res.DocSummaries[0].Score, "FTS no-tags path should populate BM25 score")
	assert.NotZero(t, res.DocSummaries[1].Score, "FTS no-tags path should populate BM25 score")
}

func TestSearch_DomainBacklog_ReturnsMatches(t *testing.T) {
	db := testutil.TestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "fix the flux capacitor", "desc", "", "", "")
	require.NoError(t, err)

	res, err := Search(db, Request{Domain: "backlog", Query: "flux"})
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	assert.Equal(t, "fix the flux capacitor", res.Items[0].Title)
}

func TestSearch_DomainBacklog_MissingQuery_Errors(t *testing.T) {
	db := testutil.TestDB(t)

	_, err := Search(db, Request{Domain: "backlog"})
	require.EqualError(t, err, "query is required")
}

func TestSearch_DomainBacklog_FilterErrors(t *testing.T) {
	db := testutil.TestDB(t)

	_, err := Search(db, Request{Domain: "backlog", Query: "flux", Sort: "bogus"})
	require.Error(t, err)
}

func TestSearch_DomainRoadmap(t *testing.T) {
	db := testutil.TestDB(t)
	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "Unique searchable widget"})
		return err
	}))

	res, err := Search(db, Request{Domain: "roadmap", Query: "widget"})
	require.NoError(t, err)
	require.Len(t, res.DocSummaries, 1)
	assert.Equal(t, "roadmap", res.DocSummaries[0].Type)
}

func TestSearch_DomainRoadmap_MissingQuery_Errors(t *testing.T) {
	db := testutil.TestDB(t)

	_, err := Search(db, Request{Domain: "roadmap"})
	require.EqualError(t, err, "query is required")
}

// TestSearch_DomainRoadmap_QueryWithComponent is the OU-330 regression: a
// full-text query combined with component must only surface projects whose
// (component-filtered) roadmap still has matching items.
func TestSearch_DomainRoadmap_QueryWithComponent(t *testing.T) {
	db := testutil.TestDB(t)
	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "Unique searchable widget", Component: "core"})
		return err
	}))
	require.NoError(t, roadmap.Mutate(db, "other-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "Unique searchable widget", Component: "plugin"})
		return err
	}))

	res, err := Search(db, Request{Domain: "roadmap", Query: "widget", Component: "core"})
	require.NoError(t, err)
	require.Len(t, res.DocSummaries, 1)
	assert.Equal(t, "acme-corp", res.DocSummaries[0].Project)
}

// TestSearch_DomainRoadmap_QueryWithEpic mirrors the component case for the
// epic grouping axis.
func TestSearch_DomainRoadmap_QueryWithEpic(t *testing.T) {
	db := testutil.TestDB(t)
	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "Unique searchable widget", Epic: "B1-1"})
		return err
	}))

	res, err := Search(db, Request{Domain: "roadmap", Query: "widget", Epic: "B1-1"})
	require.NoError(t, err)
	require.Len(t, res.DocSummaries, 1)

	res, err = Search(db, Request{Domain: "roadmap", Query: "widget", Epic: "B1-999"})
	require.NoError(t, err)
	require.Len(t, res.DocSummaries, 0)
}

// TestSearch_DomainRoadmap_NoFilter_Unchanged pins the no-filter shape: no
// component/epic means the original doc-summary hit list, unmodified.
func TestSearch_DomainRoadmap_NoFilter_Unchanged(t *testing.T) {
	db := testutil.TestDB(t)
	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "Unique searchable widget", Component: "core"})
		return err
	}))

	res, err := Search(db, Request{Domain: "roadmap", Query: "widget"})
	require.NoError(t, err)
	require.Len(t, res.DocSummaries, 1)
	assert.Equal(t, "roadmap", res.DocSummaries[0].Type)
	assert.Equal(t, "acme-corp", res.DocSummaries[0].Project)
}
