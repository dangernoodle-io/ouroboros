package roadmap_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/roadmap"
)

func TestSeedBucketsByStatusAndPriority(t *testing.T) {
	items := []backlog.Item{
		{ID: "BB-1", Priority: "P0", Title: "now item", Status: "open"},
		{ID: "BB-2", Priority: "P1", Title: "next item p1", Status: "open"},
		{ID: "BB-3", Priority: "P2", Title: "next item p2", Status: "open"},
		{ID: "BB-4", Priority: "P3", Title: "parked item", Status: "open"},
		{ID: "BB-5", Priority: "", Title: "no priority", Status: "open"},
		{ID: "BB-6", Priority: "P0", Title: "shipped item", Status: "done"},
	}

	rm := roadmap.New()
	res, err := roadmap.Seed(rm, items, false)

	require.NoError(t, err)
	assert.Equal(t, roadmap.SeedResult{Added: 6}, res)
	require.Len(t, rm.Sections.Now, 1)
	assert.Equal(t, "now item", rm.Sections.Now[0].Title)
	require.Len(t, rm.Sections.Next, 2)
	require.Len(t, rm.Sections.Parked, 2)
	require.Len(t, rm.Sections.Done, 1)
	assert.Equal(t, "shipped item", rm.Sections.Done[0].Title)
}

func TestSeedCopiesComponentEpicAndTicket(t *testing.T) {
	items := []backlog.Item{
		{ID: "BB-9", Priority: "P0", Title: "wifi fix", Status: "open", Component: "wifi", Epic: "BB-1"},
	}

	rm := roadmap.New()
	_, err := roadmap.Seed(rm, items, false)
	require.NoError(t, err)

	require.Len(t, rm.Sections.Now, 1)
	item := rm.Sections.Now[0]
	assert.Equal(t, "wifi", item.Component)
	assert.Equal(t, "BB-1", item.Epic)
	assert.Equal(t, []string{"BB-9"}, item.Ticket)
}

func TestSeedTruncatesLongDescriptionIntoBody(t *testing.T) {
	long := make([]byte, 500)
	for i := range long {
		long[i] = 'x'
	}
	items := []backlog.Item{
		{ID: "BB-1", Priority: "P0", Title: "long desc", Status: "open", Description: string(long)},
	}

	rm := roadmap.New()
	_, err := roadmap.Seed(rm, items, false)
	require.NoError(t, err)

	require.Len(t, rm.Sections.Now, 1)
	assert.LessOrEqual(t, len(rm.Sections.Now[0].Body), 280)
	assert.NotEmpty(t, rm.Sections.Now[0].Body)
}

func TestSeedAdditiveDedupSkipsAlreadySeeded(t *testing.T) {
	items := []backlog.Item{
		{ID: "BB-1", Priority: "P0", Title: "now item", Status: "open"},
	}

	rm := roadmap.New()
	first, err := roadmap.Seed(rm, items, false)
	require.NoError(t, err)
	require.Equal(t, roadmap.SeedResult{Added: 1}, first)

	second, err := roadmap.Seed(rm, items, false)
	require.NoError(t, err)
	assert.Equal(t, roadmap.SeedResult{Skipped: 1}, second)
	assert.Len(t, rm.Sections.Now, 1, "re-running additive seed must not duplicate")
}

func TestSeedReplaceResyncsChangedItem(t *testing.T) {
	items := []backlog.Item{
		{ID: "BB-1", Priority: "P0", Title: "was now", Status: "open"},
	}

	rm := roadmap.New()
	_, err := roadmap.Seed(rm, items, false)
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 1)

	// Priority changed to P1: a --replace re-run should re-bucket it into Next.
	items[0].Priority = "P1"
	items[0].Title = "now next"
	res, err := roadmap.Seed(rm, items, true)

	require.NoError(t, err)
	assert.Equal(t, roadmap.SeedResult{Replaced: 1}, res)
	assert.Empty(t, rm.Sections.Now)
	require.Len(t, rm.Sections.Next, 1)
	assert.Equal(t, "now next", rm.Sections.Next[0].Title)
	assert.Equal(t, []string{"BB-1"}, rm.Sections.Next[0].Ticket)
}

func TestSeedReplaceDoesNotTouchManualItems(t *testing.T) {
	rm := roadmap.New()
	_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "manual item"})
	require.NoError(t, err)

	items := []backlog.Item{
		{ID: "BB-1", Priority: "P0", Title: "seeded item", Status: "open"},
	}
	res, err := roadmap.Seed(rm, items, true)

	require.NoError(t, err)
	assert.Equal(t, roadmap.SeedResult{Added: 1}, res)
	require.Len(t, rm.Sections.Now, 2)
	titles := []string{rm.Sections.Now[0].Title, rm.Sections.Now[1].Title}
	assert.Contains(t, titles, "manual item")
	assert.Contains(t, titles, "seeded item")
}

func TestSeedEmptyBacklogIsNoOp(t *testing.T) {
	rm := roadmap.New()
	res, err := roadmap.Seed(rm, nil, false)

	require.NoError(t, err)
	assert.Equal(t, roadmap.SeedResult{}, res)
	assert.Empty(t, rm.Sections.Now)
	assert.Empty(t, rm.Sections.Next)
	assert.Empty(t, rm.Sections.Parked)
	assert.Empty(t, rm.Sections.Done)
}
