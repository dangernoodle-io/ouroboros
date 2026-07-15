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
