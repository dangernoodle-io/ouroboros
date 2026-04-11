package main

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
)

func resetAllDB(t *testing.T) {
	t.Helper()
	_, err := db.Exec("DELETE FROM items")
	require.NoError(t, err)
	_, err = db.Exec("DELETE FROM plans")
	require.NoError(t, err)
	_, err = db.Exec("DELETE FROM projects")
	require.NoError(t, err)
	_, err = db.Exec("DELETE FROM config")
	require.NoError(t, err)
	_, err = db.Exec("DELETE FROM documents")
	require.NoError(t, err)
}

func TestHandleProject(t *testing.T) {
	resetAllDB(t)

	// Create a project
	req := makeRequest(map[string]interface{}{
		"name": "acme-corp",
	})

	result, err := handleProject(db, bk)(context.TODO(), req)
	require.NoError(t, err)

	var proj backlog.Project
	err = unmarshalResult(result, &proj)
	require.NoError(t, err)
	assert.Equal(t, "acme-corp", proj.Name)
	assert.Equal(t, "AC", proj.Prefix)
	assert.NotZero(t, proj.ID)

	// List projects
	req = makeRequest(map[string]interface{}{})
	result, err = handleProject(db, bk)(context.TODO(), req)
	require.NoError(t, err)

	var projects []backlog.Project
	err = unmarshalResult(result, &projects)
	require.NoError(t, err)
	assert.Len(t, projects, 1)
	assert.Equal(t, "acme-corp", projects[0].Name)
}

func TestHandleProjectPrefixCollision(t *testing.T) {
	resetAllDB(t)

	// Create first project with AC prefix
	req := makeRequest(map[string]interface{}{
		"name": "acme-corp",
	})
	result, err := handleProject(db, bk)(context.TODO(), req)
	require.NoError(t, err)

	var proj backlog.Project
	err = unmarshalResult(result, &proj)
	require.NoError(t, err)
	assert.Equal(t, "AC", proj.Prefix)

	// Create second project starting with A — should get derived prefix
	req = makeRequest(map[string]interface{}{
		"name": "alpha-team",
	})
	result, err = handleProject(db, bk)(context.TODO(), req)
	require.NoError(t, err)

	var proj2 backlog.Project
	err = unmarshalResult(result, &proj2)
	require.NoError(t, err)
	assert.NotEqual(t, "AC", proj2.Prefix)
}

func TestHandleItemCreate(t *testing.T) {
	resetAllDB(t)

	// Create project first
	projReq := makeRequest(map[string]interface{}{
		"name": "acme-corp",
	})
	projResult, err := handleProject(db, bk)(context.TODO(), projReq)
	require.NoError(t, err)

	var proj backlog.Project
	err = unmarshalResult(projResult, &proj)
	require.NoError(t, err)

	// Create item
	itemReq := makeRequest(map[string]interface{}{
		"project":     "acme-corp",
		"priority":    "P1",
		"title":       "Fix login bug",
		"description": "Users cannot log in",
	})
	itemResult, err := handleItem(db, bk)(context.TODO(), itemReq)
	require.NoError(t, err)

	var item backlog.Item
	err = unmarshalResult(itemResult, &item)
	require.NoError(t, err)
	assert.Equal(t, "AC-1", item.ID)
	assert.Equal(t, "P1", item.Priority)
	assert.Equal(t, "Fix login bug", item.Title)
	assert.Equal(t, "Users cannot log in", item.Description)
	assert.Equal(t, "open", item.Status)
}

func TestHandleItemGet(t *testing.T) {
	resetAllDB(t)

	// Create project and item
	projReq := makeRequest(map[string]interface{}{
		"name": "acme-corp",
	})
	_, err := handleProject(db, bk)(context.TODO(), projReq)
	require.NoError(t, err)

	itemReq := makeRequest(map[string]interface{}{
		"project":  "acme-corp",
		"priority": "P2",
		"title":    "Update docs",
	})
	createResult, err := handleItem(db, bk)(context.TODO(), itemReq)
	require.NoError(t, err)

	var createdItem backlog.Item
	err = unmarshalResult(createResult, &createdItem)
	require.NoError(t, err)

	// Get item by ID
	getReq := makeRequest(map[string]interface{}{
		"id": "AC-1",
	})
	getResult, err := handleItem(db, bk)(context.TODO(), getReq)
	require.NoError(t, err)

	var retrievedItem backlog.Item
	err = unmarshalResult(getResult, &retrievedItem)
	require.NoError(t, err)
	assert.Equal(t, "AC-1", retrievedItem.ID)
	assert.Equal(t, "Update docs", retrievedItem.Title)
	assert.Equal(t, "P2", retrievedItem.Priority)
}

func TestHandleItemUpdate(t *testing.T) {
	resetAllDB(t)

	// Create project and item
	projReq := makeRequest(map[string]interface{}{
		"name": "acme-corp",
	})
	_, err := handleProject(db, bk)(context.TODO(), projReq)
	require.NoError(t, err)

	itemReq := makeRequest(map[string]interface{}{
		"project":  "acme-corp",
		"priority": "P3",
		"title":    "Original title",
	})
	_, err = handleItem(db, bk)(context.TODO(), itemReq)
	require.NoError(t, err)

	// Update item
	updateReq := makeRequest(map[string]interface{}{
		"id":       "AC-1",
		"priority": "P1",
		"title":    "Updated title",
	})
	updateResult, err := handleItem(db, bk)(context.TODO(), updateReq)
	require.NoError(t, err)

	var updatedItem backlog.Item
	err = unmarshalResult(updateResult, &updatedItem)
	require.NoError(t, err)
	assert.Equal(t, "AC-1", updatedItem.ID)
	assert.Equal(t, "P1", updatedItem.Priority)
	assert.Equal(t, "Updated title", updatedItem.Title)
}

func TestHandleItemDone(t *testing.T) {
	resetAllDB(t)

	// Create project and item
	projReq := makeRequest(map[string]interface{}{
		"name": "acme-corp",
	})
	_, err := handleProject(db, bk)(context.TODO(), projReq)
	require.NoError(t, err)

	itemReq := makeRequest(map[string]interface{}{
		"project":  "acme-corp",
		"priority": "P1",
		"title":    "Task to complete",
	})
	_, err = handleItem(db, bk)(context.TODO(), itemReq)
	require.NoError(t, err)

	// Mark as done
	doneReq := makeRequest(map[string]interface{}{
		"id":     "AC-1",
		"status": "done",
	})
	doneResult, err := handleItem(db, bk)(context.TODO(), doneReq)
	require.NoError(t, err)

	var doneItem backlog.Item
	err = unmarshalResult(doneResult, &doneItem)
	require.NoError(t, err)
	assert.Equal(t, "done", doneItem.Status)
}

func TestHandleItemList(t *testing.T) {
	resetAllDB(t)

	// Create project
	projReq := makeRequest(map[string]interface{}{
		"name": "acme-corp",
	})
	_, err := handleProject(db, bk)(context.TODO(), projReq)
	require.NoError(t, err)

	// Create multiple items
	for i := 1; i <= 3; i++ {
		itemReq := makeRequest(map[string]interface{}{
			"project":  "acme-corp",
			"priority": "P1",
			"title":    "Item " + string(rune('0'+i)),
		})
		_, err = handleItem(db, bk)(context.TODO(), itemReq)
		require.NoError(t, err)
	}

	// List items
	listReq := makeRequest(map[string]interface{}{
		"project": "acme-corp",
	})
	listResult, err := handleItem(db, bk)(context.TODO(), listReq)
	require.NoError(t, err)

	// Result is text in list mode
	require.Len(t, listResult.Content, 1)
	textContent, ok := mcp.AsTextContent(listResult.Content[0])
	require.True(t, ok)
	text := textContent.Text

	assert.Contains(t, text, "AC-1")
	assert.Contains(t, text, "AC-2")
	assert.Contains(t, text, "AC-3")
	assert.Contains(t, text, "Item 1")
	assert.Contains(t, text, "Item 2")
	assert.Contains(t, text, "Item 3")
}

func TestHandleItemListFilter(t *testing.T) {
	resetAllDB(t)

	// Create project
	projReq := makeRequest(map[string]interface{}{
		"name": "acme-corp",
	})
	_, err := handleProject(db, bk)(context.TODO(), projReq)
	require.NoError(t, err)

	// Create items with different priorities and status
	itemReq := makeRequest(map[string]interface{}{
		"project":  "acme-corp",
		"priority": "P1",
		"title":    "High priority",
	})
	_, err = handleItem(db, bk)(context.TODO(), itemReq)
	require.NoError(t, err)

	itemReq = makeRequest(map[string]interface{}{
		"project":  "acme-corp",
		"priority": "P3",
		"title":    "Low priority",
	})
	_, err = handleItem(db, bk)(context.TODO(), itemReq)
	require.NoError(t, err)

	// Mark one as done
	doneReq := makeRequest(map[string]interface{}{
		"id":     "AC-1",
		"status": "done",
	})
	_, err = handleItem(db, bk)(context.TODO(), doneReq)
	require.NoError(t, err)

	// Filter by status=done
	listReq := makeRequest(map[string]interface{}{
		"project": "acme-corp",
		"status":  "done",
	})
	listResult, err := handleItem(db, bk)(context.TODO(), listReq)
	require.NoError(t, err)

	textContent, ok := mcp.AsTextContent(listResult.Content[0])
	require.True(t, ok)
	text := textContent.Text

	assert.Contains(t, text, "AC-1")
	assert.NotContains(t, text, "AC-2")
}

func TestHandlePlanCreate(t *testing.T) {
	resetAllDB(t)

	// Create standalone plan
	planReq := makeRequest(map[string]interface{}{
		"title":   "Implementation plan",
		"content": "Step 1: Design\nStep 2: Implement",
	})
	planResult, err := handlePlan(db, bk)(context.TODO(), planReq)
	require.NoError(t, err)

	var plan backlog.Plan
	err = unmarshalResult(planResult, &plan)
	require.NoError(t, err)
	assert.Equal(t, "Implementation plan", plan.Title)
	assert.Equal(t, "Step 1: Design\nStep 2: Implement", plan.Content)
	assert.Equal(t, "draft", plan.Status)
	assert.Nil(t, plan.ProjectID)
	assert.Nil(t, plan.ItemID)
}

func TestHandlePlanCreateLinked(t *testing.T) {
	resetAllDB(t)

	// Create project and item
	projReq := makeRequest(map[string]interface{}{
		"name": "acme-corp",
	})
	_, err := handleProject(db, bk)(context.TODO(), projReq)
	require.NoError(t, err)

	itemReq := makeRequest(map[string]interface{}{
		"project":  "acme-corp",
		"priority": "P1",
		"title":    "Implement feature",
	})
	_, err = handleItem(db, bk)(context.TODO(), itemReq)
	require.NoError(t, err)

	// Create linked plan
	planReq := makeRequest(map[string]interface{}{
		"title":   "Feature plan",
		"content": "Details here",
		"project": "acme-corp",
		"item_id": "AC-1",
	})
	planResult, err := handlePlan(db, bk)(context.TODO(), planReq)
	require.NoError(t, err)

	var plan backlog.Plan
	err = unmarshalResult(planResult, &plan)
	require.NoError(t, err)
	assert.NotNil(t, plan.ProjectID)
	assert.NotNil(t, plan.ItemID)
	assert.Equal(t, "AC-1", *plan.ItemID)
}

func TestHandlePlanGet(t *testing.T) {
	resetAllDB(t)

	// Create plan
	planReq := makeRequest(map[string]interface{}{
		"title":   "Test plan",
		"content": "Test content",
	})
	createResult, err := handlePlan(db, bk)(context.TODO(), planReq)
	require.NoError(t, err)

	var createdPlan backlog.Plan
	err = unmarshalResult(createResult, &createdPlan)
	require.NoError(t, err)

	// Get plan by ID
	getReq := makeRequest(map[string]interface{}{
		"id": float64(createdPlan.ID),
	})
	getResult, err := handlePlan(db, bk)(context.TODO(), getReq)
	require.NoError(t, err)

	var retrievedPlan backlog.Plan
	err = unmarshalResult(getResult, &retrievedPlan)
	require.NoError(t, err)
	assert.Equal(t, createdPlan.ID, retrievedPlan.ID)
	assert.Equal(t, "Test plan", retrievedPlan.Title)
	assert.Equal(t, "Test content", retrievedPlan.Content)
}

func TestHandlePlanUpdate(t *testing.T) {
	resetAllDB(t)

	// Create plan
	planReq := makeRequest(map[string]interface{}{
		"title":   "Original title",
		"content": "Original content",
	})
	createResult, err := handlePlan(db, bk)(context.TODO(), planReq)
	require.NoError(t, err)

	var plan backlog.Plan
	err = unmarshalResult(createResult, &plan)
	require.NoError(t, err)

	// Update plan
	updateReq := makeRequest(map[string]interface{}{
		"id":     float64(plan.ID),
		"title":  "Updated title",
		"status": "active",
	})
	updateResult, err := handlePlan(db, bk)(context.TODO(), updateReq)
	require.NoError(t, err)

	var updatedPlan backlog.Plan
	err = unmarshalResult(updateResult, &updatedPlan)
	require.NoError(t, err)
	assert.Equal(t, plan.ID, updatedPlan.ID)
	assert.Equal(t, "Updated title", updatedPlan.Title)
	assert.Equal(t, "active", updatedPlan.Status)
}

func TestHandlePlanList(t *testing.T) {
	resetAllDB(t)

	// Create project
	projReq := makeRequest(map[string]interface{}{
		"name": "acme-corp",
	})
	_, err := handleProject(db, bk)(context.TODO(), projReq)
	require.NoError(t, err)

	// Create multiple plans
	for i := 1; i <= 2; i++ {
		planReq := makeRequest(map[string]interface{}{
			"title":   "Plan " + string(rune('0'+i)),
			"project": "acme-corp",
		})
		_, err = handlePlan(db, bk)(context.TODO(), planReq)
		require.NoError(t, err)
	}

	// List plans
	listReq := makeRequest(map[string]interface{}{
		"project": "acme-corp",
	})
	listResult, err := handlePlan(db, bk)(context.TODO(), listReq)
	require.NoError(t, err)

	var plans []backlog.Plan
	err = unmarshalResult(listResult, &plans)
	require.NoError(t, err)
	assert.Len(t, plans, 2)
}

func TestHandleConfig(t *testing.T) {
	resetAllDB(t)

	// Set config
	setReq := makeRequest(map[string]interface{}{
		"key":   "api_key",
		"value": "secret123",
	})
	setResult, err := handleConfig(db)(context.TODO(), setReq)
	require.NoError(t, err)

	var setResp map[string]interface{}
	err = unmarshalResult(setResult, &setResp)
	require.NoError(t, err)
	updated, ok := setResp["updated"].(bool)
	require.True(t, ok)
	assert.True(t, updated)

	// Get config
	getReq := makeRequest(map[string]interface{}{
		"key": "api_key",
	})
	getResult, err := handleConfig(db)(context.TODO(), getReq)
	require.NoError(t, err)

	var getResp map[string]string
	err = unmarshalResult(getResult, &getResp)
	require.NoError(t, err)
	assert.Equal(t, "secret123", getResp["value"])
}

func TestHandleConfigGetAll(t *testing.T) {
	resetAllDB(t)

	// Set multiple configs
	for i, key := range []string{"key1", "key2"} {
		setReq := makeRequest(map[string]interface{}{
			"key":   key,
			"value": "value" + string(rune('0'+i+1)),
		})
		_, err := handleConfig(db)(context.TODO(), setReq)
		require.NoError(t, err)
	}

	// Get all configs
	getAllReq := makeRequest(map[string]interface{}{})
	getAllResult, err := handleConfig(db)(context.TODO(), getAllReq)
	require.NoError(t, err)

	var allConfigs map[string]string
	err = unmarshalResult(getAllResult, &allConfigs)
	require.NoError(t, err)
	assert.Len(t, allConfigs, 2)
	assert.Equal(t, "value1", allConfigs["key1"])
	assert.Equal(t, "value2", allConfigs["key2"])
}

func TestHandleConfigNotFound(t *testing.T) {
	resetAllDB(t)

	// Try to get nonexistent key
	getReq := makeRequest(map[string]interface{}{
		"key": "nonexistent",
	})
	getResult, err := handleConfig(db)(context.TODO(), getReq)
	require.NoError(t, err)
	assert.True(t, getResult.IsError)
}

func TestHandleItemInvalidPriority(t *testing.T) {
	resetAllDB(t)

	// Create project
	projReq := makeRequest(map[string]interface{}{
		"name": "acme-corp",
	})
	_, err := handleProject(db, bk)(context.TODO(), projReq)
	require.NoError(t, err)

	// Try to create item with invalid priority
	itemReq := makeRequest(map[string]interface{}{
		"project":  "acme-corp",
		"priority": "X9",
		"title":    "Invalid item",
	})
	itemResult, err := handleItem(db, bk)(context.TODO(), itemReq)
	require.NoError(t, err)
	assert.True(t, itemResult.IsError)
}

func TestHandleItemNonexistentProject(t *testing.T) {
	resetAllDB(t)

	// Try to create item in nonexistent project
	itemReq := makeRequest(map[string]interface{}{
		"project":  "nonexistent",
		"priority": "P1",
		"title":    "Item",
	})
	itemResult, err := handleItem(db, bk)(context.TODO(), itemReq)
	require.NoError(t, err)
	assert.True(t, itemResult.IsError)
}

func TestParsePriority(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{"P0", "P0", 0, false},
		{"P1", "P1", 1, false},
		{"P6", "P6", 6, false},
		{"invalid format", "P7", 0, true},
		{"invalid prefix", "X1", 0, true},
		{"invalid value", "P-1", 0, true},
		{"no prefix", "1", 0, true},
		{"empty", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePriority(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
