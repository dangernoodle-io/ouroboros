package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
)

func TestLSPlansList(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.CreatePlan(db, "Q1 roadmap", "Plan for Q1", &proj.ID, nil)
	require.NoError(t, err)
	_, err = backlog.CreatePlan(db, "Q2 roadmap", "Plan for Q2", &proj.ID, nil)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSPlans(&buf, db, "", "", false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "ID")
	assert.Contains(t, output, "STATUS")
	assert.Contains(t, output, "Q1 roadmap")
	assert.Contains(t, output, "Q2 roadmap")
	assert.Contains(t, output, "draft")
}

func TestLSPlansListJSON(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.CreatePlan(db, "Q1 planning", "Plan content", &proj.ID, nil)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSPlans(&buf, db, "", "", true)
	require.NoError(t, err)

	var plans []backlog.Plan
	err = json.Unmarshal(buf.Bytes(), &plans)
	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.Equal(t, "Q1 planning", plans[0].Title)
	assert.Equal(t, "draft", plans[0].Status)
}

func TestLSPlansProjectFilter(t *testing.T) {
	db := newTestDB(t)
	proj1, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	proj2, err := backlog.CreateProject(db, "other-project", "OP")
	require.NoError(t, err)

	_, err = backlog.CreatePlan(db, "AC plan", "Plan", &proj1.ID, nil)
	require.NoError(t, err)
	_, err = backlog.CreatePlan(db, "OP plan", "Plan", &proj2.ID, nil)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSPlans(&buf, db, "acme-corp", "", false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "AC plan")
	assert.NotContains(t, output, "OP plan")
}

func TestLSPlansStatusFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.CreatePlan(db, "Plan 1", "Content", &proj.ID, nil)
	require.NoError(t, err)
	plan2, err := backlog.CreatePlan(db, "Plan 2", "Content", &proj.ID, nil)
	require.NoError(t, err)

	_, err = backlog.UpdatePlan(db, plan2.ID, map[string]string{"status": "complete"})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSPlans(&buf, db, "", "complete", false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Plan 2")
	assert.NotContains(t, output, "Plan 1")
}

func TestLSPlansDetailJSON(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	result, err := backlog.CreatePlan(db, "Detailed plan", "This is the content", &proj.ID, nil)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSPlanDetail(&buf, db, "1", true)
	require.NoError(t, err)

	var plan backlog.Plan
	err = json.Unmarshal(buf.Bytes(), &plan)
	require.NoError(t, err)
	assert.Equal(t, result.ID, plan.ID)
	assert.Equal(t, "Detailed plan", plan.Title)
	assert.Equal(t, "This is the content", plan.Content)
}

func TestLSPlansDetailPlain(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.CreatePlan(db, "Q1 strategy", "Quarterly planning document", &proj.ID, nil)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSPlanDetail(&buf, db, "1", false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "[draft]")
	assert.Contains(t, output, "acme-corp")
	assert.Contains(t, output, "Q1 strategy")
	assert.Contains(t, output, "Content:")
}

func TestLSPlansProjectNotFound(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runLSPlans(&buf, db, "nonexistent", "", false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "ID")
	assert.Contains(t, output, "STATUS")
}
