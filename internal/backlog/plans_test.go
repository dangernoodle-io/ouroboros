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

	item, err := backlog.AddItem(d, p.ID, "AC", "P1", "test-item", "", "", "", "")
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
		"status": "active",
	})
	require.NoError(t, err)

	assert.Equal(t, "new-title", updated.Title)
	assert.Equal(t, "old-content", updated.Content)
	assert.Equal(t, "active", updated.Status)
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

// TestListPlansOmitsContent asserts list mode returns compact PlanSummary
// rows (no content field to leak into agent context), while GetPlan by id
// still returns the full Plan including content.
func TestListPlansOmitsContent(t *testing.T) {
	d := testDB(t)

	created, err := backlog.CreatePlan(d, "summary-plan", "sensitive plan content", nil, nil)
	require.NoError(t, err)

	plans, err := backlog.ListPlans(d, backlog.PlanFilter{})
	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.Equal(t, created.ID, plans[0].ID)
	assert.Equal(t, "summary-plan", plans[0].Title)
	assert.Equal(t, "draft", plans[0].Status)

	// PlanSummary has no Content field at all — compile-time enforcement via
	// the struct shape itself; runtime check confirms the fetched-by-id path
	// still returns full content.
	full, err := backlog.GetPlan(d, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "sensitive plan content", full.Content)
}

func TestListPlansFilterStatus(t *testing.T) {
	d := testDB(t)

	_, err := backlog.CreatePlan(d, "draft-plan", "content", nil, nil)
	require.NoError(t, err)

	plan2, err := backlog.CreatePlan(d, "published-plan", "content", nil, nil)
	require.NoError(t, err)

	_, err = backlog.UpdatePlan(d, plan2.ID, map[string]string{"status": "active"})
	require.NoError(t, err)

	status := "active"
	plans, err := backlog.ListPlans(d, backlog.PlanFilter{Status: &status})
	require.NoError(t, err)

	assert.Len(t, plans, 1)
	assert.Equal(t, "published-plan", plans[0].Title)
}

func TestListPlansFilterProject(t *testing.T) {
	d := testDB(t)
	p1 := createTestProject(t, d)
	p2, err := backlog.CreateProject(d, "other-corp", "OC")
	require.NoError(t, err)

	_, err = backlog.CreatePlan(d, "plan1", "content1", &p1.ID, nil)
	require.NoError(t, err)
	_, err = backlog.CreatePlan(d, "plan2", "content2", &p2.ID, nil)
	require.NoError(t, err)
	_, err = backlog.CreatePlan(d, "plan3", "content3", nil, nil)
	require.NoError(t, err)

	plans, err := backlog.ListPlans(d, backlog.PlanFilter{ProjectIDs: []int64{p1.ID}})
	require.NoError(t, err)
	assert.Len(t, plans, 1)
	assert.Equal(t, "plan1", plans[0].Title)
}

func TestListPlansFilterMultiProject(t *testing.T) {
	d := testDB(t)
	p1 := createTestProject(t, d)
	p2, err := backlog.CreateProject(d, "other-corp", "OC")
	require.NoError(t, err)

	_, err = backlog.CreatePlan(d, "plan1", "content1", &p1.ID, nil)
	require.NoError(t, err)
	_, err = backlog.CreatePlan(d, "plan2", "content2", &p2.ID, nil)
	require.NoError(t, err)
	_, err = backlog.CreatePlan(d, "plan3", "content3", nil, nil)
	require.NoError(t, err)

	plans, err := backlog.ListPlans(d, backlog.PlanFilter{ProjectIDs: []int64{p1.ID, p2.ID}})
	require.NoError(t, err)
	assert.Len(t, plans, 2)
}

func TestUpdatePlanInvalidStatus(t *testing.T) {
	d := testDB(t)

	plan, err := backlog.CreatePlan(d, "test-plan", "content", nil, nil)
	require.NoError(t, err)

	_, err = backlog.UpdatePlan(d, plan.ID, map[string]string{"status": "published"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid status")
	assert.Contains(t, err.Error(), "draft, active, done")
}

func TestUpdatePlanValidStatuses(t *testing.T) {
	d := testDB(t)

	for _, status := range []string{"draft", "active", "done"} {
		plan, err := backlog.CreatePlan(d, "plan-"+status, "content", nil, nil)
		require.NoError(t, err)

		updated, err := backlog.UpdatePlan(d, plan.ID, map[string]string{"status": status})
		require.NoError(t, err)
		assert.Equal(t, status, updated.Status)
	}
}

func TestCreatePlanContentTooBig(t *testing.T) {
	d := testDB(t)

	bigContent := string(make([]byte, backlog.MaxPlanContentBytes+1))
	_, err := backlog.CreatePlan(d, "big-plan", bigContent, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "content exceeds")
}

func TestUpdatePlanContentTooBig(t *testing.T) {
	d := testDB(t)

	plan, err := backlog.CreatePlan(d, "test-plan", "content", nil, nil)
	require.NoError(t, err)

	bigContent := string(make([]byte, backlog.MaxPlanContentBytes+1))
	_, err = backlog.UpdatePlan(d, plan.ID, map[string]string{"content": bigContent})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "content exceeds")
}
