package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/testutil"
)

func TestBuildItemFilter_AllFields(t *testing.T) {
	db := testutil.TestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	f, err := buildItemFilter(db, Request{
		Projects:    []string{"acme-corp"},
		PriorityMin: "P1",
		PriorityMax: "P3",
		Status:      "open",
		Component:   "widget",
		Epic:        "AC-1",
		Since:       "24h",
		Sort:        "created",
		Limit:       5,
	})
	require.NoError(t, err)
	require.Len(t, f.ProjectIDs, 1)
	assert.Equal(t, proj.ID, f.ProjectIDs[0])
	require.NotNil(t, f.PriorityMin)
	assert.Equal(t, 1, *f.PriorityMin)
	require.NotNil(t, f.PriorityMax)
	assert.Equal(t, 3, *f.PriorityMax)
	require.NotNil(t, f.Status)
	assert.Equal(t, "open", *f.Status)
	require.NotNil(t, f.Component)
	assert.Equal(t, "widget", *f.Component)
	require.NotNil(t, f.Epic)
	assert.Equal(t, "AC-1", *f.Epic)
	assert.False(t, f.EpicsOnly)
	require.NotNil(t, f.CreatedSince)
	assert.True(t, f.SortByCreated)
	assert.Equal(t, 5, f.Limit)
}

func TestBuildItemFilter_EpicsOnlyOverridesEpic(t *testing.T) {
	db := testutil.TestDB(t)

	f, err := buildItemFilter(db, Request{EpicsOnly: true, Epic: "AC-1"})
	require.NoError(t, err)
	assert.True(t, f.EpicsOnly)
	assert.Nil(t, f.Epic)
}

func TestBuildItemFilter_UnknownProject_Errors(t *testing.T) {
	db := testutil.TestDB(t)

	_, err := buildItemFilter(db, Request{Projects: []string{"nonexistent"}})
	require.Error(t, err)
}

func TestBuildItemFilter_BadPriorityMin_Errors(t *testing.T) {
	db := testutil.TestDB(t)

	_, err := buildItemFilter(db, Request{PriorityMin: "bogus"})
	require.Error(t, err)
}

func TestBuildItemFilter_BadPriorityMax_Errors(t *testing.T) {
	db := testutil.TestDB(t)

	_, err := buildItemFilter(db, Request{PriorityMax: "bogus"})
	require.Error(t, err)
}

// TestBuildItemFilter_InvertedPriorityRange_Errors is the OU-347 regression:
// priority_min lower severity (higher P#) than priority_max is unsatisfiable
// SQL (priority_int >= min AND priority_int <= max) and previously silently
// returned zero rows instead of erroring. This is the shared filter builder
// for both query.Get (list mode) and query.Search's backlog path, so one
// guard covers both.
func TestBuildItemFilter_InvertedPriorityRange_Errors(t *testing.T) {
	db := testutil.TestDB(t)

	_, err := buildItemFilter(db, Request{PriorityMin: "P2", PriorityMax: "P1"})
	require.EqualError(t, err, "invalid priority range: priority_min P2 is lower severity than priority_max P1 (P0 highest .. P6 lowest)")
}

// TestBuildItemFilter_PriorityRangeEqual_Valid confirms min == max (a
// single-priority filter) is not treated as inverted.
func TestBuildItemFilter_PriorityRangeEqual_Valid(t *testing.T) {
	db := testutil.TestDB(t)

	f, err := buildItemFilter(db, Request{PriorityMin: "P2", PriorityMax: "P2"})
	require.NoError(t, err)
	require.NotNil(t, f.PriorityMin)
	require.NotNil(t, f.PriorityMax)
	assert.Equal(t, 2, *f.PriorityMin)
	assert.Equal(t, 2, *f.PriorityMax)
}

// TestBuildItemFilter_PriorityRangeOnlyMin_Valid confirms the guard is
// skipped (no error) when only one bound is supplied.
func TestBuildItemFilter_PriorityRangeOnlyMin_Valid(t *testing.T) {
	db := testutil.TestDB(t)

	f, err := buildItemFilter(db, Request{PriorityMin: "P2"})
	require.NoError(t, err)
	require.NotNil(t, f.PriorityMin)
	assert.Nil(t, f.PriorityMax)
}

func TestBuildItemFilter_BadSince_Errors(t *testing.T) {
	db := testutil.TestDB(t)

	_, err := buildItemFilter(db, Request{Since: "not-a-date"})
	require.Error(t, err)
}

func TestBuildItemFilter_BadSort_Errors(t *testing.T) {
	db := testutil.TestDB(t)

	_, err := buildItemFilter(db, Request{Sort: "bogus"})
	require.EqualError(t, err, `invalid sort value "bogus": expected "created"`)
}
