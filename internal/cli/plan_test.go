package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
)

// ── create ────────────────────────────────────────────────────────────────────

func TestRunPlanCreate(t *testing.T) {
	db := newTestDB(t)
	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runPlanCreate(&buf, db, "acme-corp", "Q1 Roadmap", "Plan content here", "", "")
	require.NoError(t, err)

	var plan backlog.Plan
	require.NoError(t, json.Unmarshal(buf.Bytes(), &plan))
	assert.Equal(t, "Q1 Roadmap", plan.Title)
	assert.Equal(t, "Plan content here", plan.Content)
	assert.Equal(t, "draft", plan.Status)
	assert.NotZero(t, plan.ID)
}

func TestRunPlanCreateWithStatus(t *testing.T) {
	db := newTestDB(t)
	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runPlanCreate(&buf, db, "acme-corp", "Active Plan", "Content", "active", "")
	require.NoError(t, err)

	var plan backlog.Plan
	require.NoError(t, json.Unmarshal(buf.Bytes(), &plan))
	assert.Equal(t, "active", plan.Status)
}

func TestRunPlanCreateWithItemID(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Task", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runPlanCreate(&buf, db, "acme-corp", "Item Plan", "Content", "", item.ID)
	require.NoError(t, err)

	var plan backlog.Plan
	require.NoError(t, json.Unmarshal(buf.Bytes(), &plan))
	assert.Equal(t, "Item Plan", plan.Title)
	require.NotNil(t, plan.ItemID)
	assert.Equal(t, item.ID, *plan.ItemID)
}

func TestRunPlanCreateProjectNotFound(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runPlanCreate(&buf, db, "nonexistent", "Title", "Content", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plan create")
}

func TestRunPlanCreateNoProject(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	// empty project still creates a plan with no project linkage
	err := runPlanCreate(&buf, db, "", "Title", "Content", "", "")
	require.NoError(t, err)

	var plan backlog.Plan
	require.NoError(t, json.Unmarshal(buf.Bytes(), &plan))
	assert.Nil(t, plan.ProjectID)
}

// ── get ───────────────────────────────────────────────────────────────────────

func TestRunPlanGet(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	created, err := backlog.CreatePlan(db, "Q1 Roadmap", "Content", &proj.ID, nil)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runPlanGet(&buf, db, "1")
	require.NoError(t, err)

	var plan backlog.Plan
	require.NoError(t, json.Unmarshal(buf.Bytes(), &plan))
	assert.Equal(t, created.ID, plan.ID)
	assert.Equal(t, "Q1 Roadmap", plan.Title)
}

func TestRunPlanGetNotFound(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runPlanGet(&buf, db, "9999")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plan get")
}

func TestRunPlanGetInvalidID(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runPlanGet(&buf, db, "not-a-number")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid id")
}

// ── update ────────────────────────────────────────────────────────────────────

func TestRunPlanUpdate(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.CreatePlan(db, "Old Title", "Old content", &proj.ID, nil)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runPlanUpdate(&buf, db, "1", map[string]string{"title": "New Title"})
	require.NoError(t, err)

	var plan backlog.Plan
	require.NoError(t, json.Unmarshal(buf.Bytes(), &plan))
	assert.Equal(t, "New Title", plan.Title)
}

func TestRunPlanUpdateContent(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.CreatePlan(db, "Title", "Old content", &proj.ID, nil)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runPlanUpdate(&buf, db, "1", map[string]string{"content": "New content"})
	require.NoError(t, err)

	var plan backlog.Plan
	require.NoError(t, json.Unmarshal(buf.Bytes(), &plan))
	assert.Equal(t, "New content", plan.Content)
}

func TestRunPlanUpdateStatus(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.CreatePlan(db, "Title", "Content", &proj.ID, nil)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runPlanUpdate(&buf, db, "1", map[string]string{"status": "done"})
	require.NoError(t, err)

	var plan backlog.Plan
	require.NoError(t, json.Unmarshal(buf.Bytes(), &plan))
	assert.Equal(t, "done", plan.Status)
}

func TestRunPlanUpdateNoFields(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runPlanUpdate(&buf, db, "1", map[string]string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one")
}

func TestRunPlanUpdateInvalidID(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runPlanUpdate(&buf, db, "bad-id", map[string]string{"title": "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid id")
}

func TestRunPlanUpdateNotFound(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runPlanUpdate(&buf, db, "9999", map[string]string{"title": "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plan update")
}

// ── list ─────────────────────────────────────────────────────────────────────

func TestRunPlanList(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.CreatePlan(db, "Plan A", "Content", &proj.ID, nil)
	require.NoError(t, err)
	_, err = backlog.CreatePlan(db, "Plan B", "Content", &proj.ID, nil)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runPlanList(&buf, db, "", "")
	require.NoError(t, err)

	var plans []backlog.Plan
	require.NoError(t, json.Unmarshal(buf.Bytes(), &plans))
	assert.Len(t, plans, 2)
}

func TestRunPlanListEmpty(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runPlanList(&buf, db, "", "")
	require.NoError(t, err)
	assert.Equal(t, "[]", strings.TrimSpace(buf.String()))
}

func TestRunPlanListProjectFilter(t *testing.T) {
	db := newTestDB(t)
	proj1, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	proj2, err := backlog.CreateProject(db, "other-proj", "OP")
	require.NoError(t, err)
	_, err = backlog.CreatePlan(db, "AC Plan", "Content", &proj1.ID, nil)
	require.NoError(t, err)
	_, err = backlog.CreatePlan(db, "OP Plan", "Content", &proj2.ID, nil)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runPlanList(&buf, db, "acme-corp", "")
	require.NoError(t, err)

	var plans []backlog.Plan
	require.NoError(t, json.Unmarshal(buf.Bytes(), &plans))
	require.Len(t, plans, 1)
	assert.Equal(t, "AC Plan", plans[0].Title)
}

func TestRunPlanListStatusFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.CreatePlan(db, "Draft Plan", "Content", &proj.ID, nil)
	require.NoError(t, err)
	p2, err := backlog.CreatePlan(db, "Done Plan", "Content", &proj.ID, nil)
	require.NoError(t, err)
	_, err = backlog.UpdatePlan(db, p2.ID, map[string]string{"status": "done"})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runPlanList(&buf, db, "", "done")
	require.NoError(t, err)

	var plans []backlog.Plan
	require.NoError(t, json.Unmarshal(buf.Bytes(), &plans))
	require.Len(t, plans, 1)
	assert.Equal(t, "Done Plan", plans[0].Title)
}

func TestRunPlanListProjectNotFound(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runPlanList(&buf, db, "nonexistent", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plan list")
}
