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

// TestSearch_DomainKB_ORFallback_PartialMatch is the OU-346 regression: a
// multi-term query where AND matches nothing but OR matches something
// surfaces the partial match, and the wire-visible DocumentSummary.Relaxed
// flag distinguishes it from an exact match.
func TestSearch_DomainKB_ORFallback_PartialMatch(t *testing.T) {
	db := testutil.TestDB(t)
	_, err := store.UpsertDocument(db, store.Document{Type: "note", Project: "acme-corp", Title: "bb_data egress design", Content: "notes about egress, no bb_event or bb_sink here"})
	require.NoError(t, err)

	res, err := Search(db, Request{Domain: "kb", Query: "bb_data bb_event bb_sink egress transport"})
	require.NoError(t, err)
	require.Len(t, res.DocSummaries, 1)
	assert.Equal(t, "bb_data egress design", res.DocSummaries[0].Title)
	assert.True(t, res.DocSummaries[0].Relaxed)
}

// TestSearch_DomainKB_QueriesBatch_EachQueryRelaxedIndependently confirms
// the queries[] batch applies the AND->OR fallback per-query: one query
// matches AND exactly, one only via the OR fallback, one matches nothing
// even relaxed.
func TestSearch_DomainKB_QueriesBatch_EachQueryRelaxedIndependently(t *testing.T) {
	db := testutil.TestDB(t)
	_, err := store.UpsertDocument(db, store.Document{Type: "note", Project: "acme-corp", Title: "alpha and beta doc", Content: "c"})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{Type: "note", Project: "acme-corp", Title: "partial widget doc", Content: "widget term present here"})
	require.NoError(t, err)

	res, err := Search(db, Request{Domain: "kb", Queries: []string{"alpha beta", "widget gadget", "zzznothere zzzalsonothere"}})
	require.NoError(t, err)
	require.Len(t, res.DocSummarySets, 3)

	require.Len(t, res.DocSummarySets[0], 1)
	assert.False(t, res.DocSummarySets[0][0].Relaxed, "exact AND match must not be flagged relaxed")

	require.Len(t, res.DocSummarySets[1], 1)
	assert.True(t, res.DocSummarySets[1][0].Relaxed, "OR-fallback match must be flagged relaxed")

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

// TestSearch_DomainBacklog_ORFallback_PartialMatch is the OU-346 regression
// for backlog: a multi-term query where AND matches nothing but OR matches
// something surfaces the partial match, with Result.Relaxed set.
func TestSearch_DomainBacklog_ORFallback_PartialMatch(t *testing.T) {
	db := testutil.TestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "bb_data egress design", "notes about egress, no bb_event or bb_sink here", "", "", "")
	require.NoError(t, err)

	res, err := Search(db, Request{Domain: "backlog", Query: "bb_data bb_event bb_sink egress transport"})
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	assert.Equal(t, "bb_data egress design", res.Items[0].Title)
	assert.True(t, res.Relaxed)
}

// TestSearch_DomainBacklog_ANDMatches_NotRelaxed confirms an exact AND match
// leaves Result.Relaxed false.
func TestSearch_DomainBacklog_ANDMatches_NotRelaxed(t *testing.T) {
	db := testutil.TestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "fix the flux capacitor", "desc", "", "", "")
	require.NoError(t, err)

	res, err := Search(db, Request{Domain: "backlog", Query: "flux"})
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	assert.False(t, res.Relaxed)
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

// TestSearch_DomainBacklog_InvertedPriorityRange_Errors is the OU-347
// regression on Search's backlog path: buildItemFilter is the shared filter
// builder for both Get and Search, so the same inverted-range guard applies
// here too.
func TestSearch_DomainBacklog_InvertedPriorityRange_Errors(t *testing.T) {
	db := testutil.TestDB(t)

	_, err := Search(db, Request{Domain: "backlog", Query: "flux", PriorityMin: "P2", PriorityMax: "P1"})
	require.EqualError(t, err, "invalid priority range: priority_min P2 is lower severity than priority_max P1 (P0 highest .. P6 lowest)")
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

// TestSearch_DomainRoadmap_ORFallback_PartialMatch is the OU-346 regression
// for roadmap: a multi-term query where AND matches nothing but OR matches
// something surfaces the partial match, with the wire-visible
// DocumentSummary.Relaxed flag set (roadmap search shares kb's per-row
// plumbing with no new code — see searchRoadmap in search.go).
func TestSearch_DomainRoadmap_ORFallback_PartialMatch(t *testing.T) {
	db := testutil.TestDB(t)
	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "bb_data egress design"})
		return err
	}))

	andOnly, err := store.SearchDocuments(db, "bb_data bb_event bb_sink egress transport", []string{"roadmap"}, nil, nil, 10)
	require.NoError(t, err)
	require.Len(t, andOnly, 0)

	res, err := Search(db, Request{Domain: "roadmap", Query: "bb_data bb_event bb_sink egress transport"})
	require.NoError(t, err)
	require.Len(t, res.DocSummaries, 1)
	assert.Equal(t, "acme-corp", res.DocSummaries[0].Project)
	assert.True(t, res.DocSummaries[0].Relaxed)
}

// TestSearch_DomainRoadmap_ANDMatches_NotRelaxed confirms AND stays primary
// for roadmap: an exact match is never flagged relaxed.
func TestSearch_DomainRoadmap_ANDMatches_NotRelaxed(t *testing.T) {
	db := testutil.TestDB(t)
	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "Unique searchable widget"})
		return err
	}))

	res, err := Search(db, Request{Domain: "roadmap", Query: "widget"})
	require.NoError(t, err)
	require.Len(t, res.DocSummaries, 1)
	assert.False(t, res.DocSummaries[0].Relaxed)
}

// TestSearch_DomainRoadmap_SingleToken_NoFallback confirms a single-token
// query with no match behaves exactly as before OU-346 — no OR retry is
// possible, so the result stays empty rather than erroring or panicking.
func TestSearch_DomainRoadmap_SingleToken_NoFallback(t *testing.T) {
	db := testutil.TestDB(t)
	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "Unique searchable widget"})
		return err
	}))

	res, err := Search(db, Request{Domain: "roadmap", Query: "zzznothingmatchesthis"})
	require.NoError(t, err)
	assert.Len(t, res.DocSummaries, 0)
}

// TestSearch_DomainRoadmap_ORFallback_SurvivesComponentFilter confirms
// DocumentSummary.Relaxed (a plain value-copy through the component/epic
// post-filter loop, search.go:109-118) both survives an OR-relaxed match
// AND that the filter still actually excludes a non-matching-component doc
// that was only surfaced by the OR retry — the branch combination
// (Component/Epic set + AND-zero + OR-relaxed) that the unfiltered-path
// tests above don't exercise.
func TestSearch_DomainRoadmap_ORFallback_SurvivesComponentFilter(t *testing.T) {
	db := testutil.TestDB(t)
	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "bb_data egress design", Component: "core"})
		return err
	}))
	require.NoError(t, roadmap.Mutate(db, "other-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "bb_data egress design", Component: "plugin"})
		return err
	}))

	andOnly, err := store.SearchDocuments(db, "bb_data bb_event bb_sink egress transport", []string{"roadmap"}, nil, nil, 10)
	require.NoError(t, err)
	require.Len(t, andOnly, 0)

	res, err := Search(db, Request{Domain: "roadmap", Query: "bb_data bb_event bb_sink egress transport", Component: "core"})
	require.NoError(t, err)
	require.Len(t, res.DocSummaries, 1, "other-corp's component=plugin doc must be filtered out despite matching the OR retry")
	assert.Equal(t, "acme-corp", res.DocSummaries[0].Project)
	assert.True(t, res.DocSummaries[0].Relaxed, "Relaxed must survive the component post-filter loop")
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
