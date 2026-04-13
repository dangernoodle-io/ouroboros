package backlog_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
)

func TestCreatePlan(t *testing.T) {
	d := testDB(t)

	plan, err := backlog.CreatePlan(d, "standalone-plan", "plan content", nil, nil)
	require.NoError(t, err)

	assert.NotZero(t, plan.ID)
	assert.Equal(t, "standalone-plan", plan.Title)
	assert.Equal(t, "plan content", plan.Content)
	assert.Nil(t, plan.ProjectID)
	assert.Nil(t, plan.ItemID)
	assert.Equal(t, "draft", plan.Status)
	assert.NotEmpty(t, plan.Created)
	assert.NotEmpty(t, plan.Updated)
}

func TestCreatePlanLinked(t *testing.T) {
	d := testDB(t)
	p := createTestProject(t, d)

	item, err := backlog.AddItem(d, p.ID, "AC", "P1", "test-item", "", "")
	require.NoError(t, err)

	plan, err := backlog.CreatePlan(d, "linked-plan", "content", &p.ID, &item.ID)
	require.NoError(t, err)

	assert.NotZero(t, plan.ID)
	assert.Equal(t, p.ID, *plan.ProjectID)
	assert.Equal(t, item.ID, *plan.ItemID)
}

func TestGetPlan(t *testing.T) {
	d := testDB(t)

	created, err := backlog.CreatePlan(d, "test-plan", "content", nil, nil)
	require.NoError(t, err)

	plan, err := backlog.GetPlan(d, created.ID)
	require.NoError(t, err)

	assert.Equal(t, created.ID, plan.ID)
	assert.Equal(t, "test-plan", plan.Title)
	assert.Equal(t, "content", plan.Content)
}

func TestGetPlanNotFound(t *testing.T) {
	d := testDB(t)

	_, err := backlog.GetPlan(d, 9999)
	assert.Error(t, err)
}

func TestUpdatePlan(t *testing.T) {
	d := testDB(t)

	created, err := backlog.CreatePlan(d, "old-title", "old-content", nil, nil)
	require.NoError(t, err)

	updated, err := backlog.UpdatePlan(d, created.ID, map[string]string{
		"title":  "new-title",
		"status": "published",
	})
	require.NoError(t, err)

	assert.Equal(t, "new-title", updated.Title)
	assert.Equal(t, "old-content", updated.Content)
	assert.Equal(t, "published", updated.Status)
}

func TestListPlans(t *testing.T) {
	d := testDB(t)

	_, err := backlog.CreatePlan(d, "plan1", "content1", nil, nil)
	require.NoError(t, err)

	_, err = backlog.CreatePlan(d, "plan2", "content2", nil, nil)
	require.NoError(t, err)

	plans, err := backlog.ListPlans(d, backlog.PlanFilter{})
	require.NoError(t, err)

	assert.Len(t, plans, 2)
}

func TestListPlansFilterStatus(t *testing.T) {
	d := testDB(t)

	_, err := backlog.CreatePlan(d, "draft-plan", "content", nil, nil)
	require.NoError(t, err)

	plan2, err := backlog.CreatePlan(d, "published-plan", "content", nil, nil)
	require.NoError(t, err)

	_, err = backlog.UpdatePlan(d, plan2.ID, map[string]string{"status": "published"})
	require.NoError(t, err)

	status := "published"
	plans, err := backlog.ListPlans(d, backlog.PlanFilter{Status: &status})
	require.NoError(t, err)

	assert.Len(t, plans, 1)
	assert.Equal(t, "published-plan", plans[0].Title)
}
