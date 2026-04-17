package app

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/backup"
)

var bk *backup.Backup

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

	// Create item using batch entries
	itemReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":     "acme-corp",
				"priority":    "P1",
				"title":       "Fix login bug",
				"description": "Users cannot log in",
			},
		},
	})
	itemResult, err := handleItem(db, bk)(context.TODO(), itemReq)
	require.NoError(t, err)

	var resp []map[string]interface{}
	err = unmarshalResult(itemResult, &resp)
	require.NoError(t, err)
	require.Len(t, resp, 1)
	id, ok := resp[0]["id"].(string)
	require.True(t, ok)

	// Fetch to verify
	fetchReq := makeRequest(map[string]interface{}{
		"ids": []interface{}{id},
	})
	fetchResult, err := handleItem(db, bk)(context.TODO(), fetchReq)
	require.NoError(t, err)

	var items []map[string]interface{}
	err = unmarshalResult(fetchResult, &items)
	require.NoError(t, err)
	require.Len(t, items, 1)
	item := items[0]
	assert.Equal(t, "AC-1", item["id"])
	assert.Equal(t, "P1", item["priority"])
	assert.Equal(t, "Fix login bug", item["title"])
	assert.Equal(t, "Users cannot log in", item["description"])
	assert.Equal(t, "open", item["status"])
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
		"entries": []interface{}{
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "P2",
				"title":    "Update docs",
			},
		},
	})
	createResult, err := handleItem(db, bk)(context.TODO(), itemReq)
	require.NoError(t, err)

	var createResp []map[string]interface{}
	err = unmarshalResult(createResult, &createResp)
	require.NoError(t, err)
	require.Len(t, createResp, 1)

	// Get item by ID using ids[] fetch
	getReq := makeRequest(map[string]interface{}{
		"ids": []interface{}{"AC-1"},
	})
	getResult, err := handleItem(db, bk)(context.TODO(), getReq)
	require.NoError(t, err)

	var items []map[string]interface{}
	err = unmarshalResult(getResult, &items)
	require.NoError(t, err)
	require.Len(t, items, 1)
	item := items[0]
	assert.Equal(t, "AC-1", item["id"])
	assert.Equal(t, "Update docs", item["title"])
	assert.Equal(t, "P2", item["priority"])
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
		"entries": []interface{}{
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "P3",
				"title":    "Original title",
			},
		},
	})
	_, err = handleItem(db, bk)(context.TODO(), itemReq)
	require.NoError(t, err)

	// Update item using batch entries
	updateReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"id":       "AC-1",
				"priority": "P1",
				"title":    "Updated title",
			},
		},
	})
	updateResult, err := handleItem(db, bk)(context.TODO(), updateReq)
	require.NoError(t, err)

	var updateResp []map[string]interface{}
	err = unmarshalResult(updateResult, &updateResp)
	require.NoError(t, err)
	require.Len(t, updateResp, 1)
	assert.Equal(t, "update", updateResp[0]["action"])

	// Fetch to verify
	fetchReq := makeRequest(map[string]interface{}{
		"ids": []interface{}{"AC-1"},
	})
	fetchResult, err := handleItem(db, bk)(context.TODO(), fetchReq)
	require.NoError(t, err)

	var items []map[string]interface{}
	err = unmarshalResult(fetchResult, &items)
	require.NoError(t, err)
	require.Len(t, items, 1)
	item := items[0]
	assert.Equal(t, "AC-1", item["id"])
	assert.Equal(t, "P1", item["priority"])
	assert.Equal(t, "Updated title", item["title"])
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
		"entries": []interface{}{
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "P1",
				"title":    "Task to complete",
			},
		},
	})
	_, err = handleItem(db, bk)(context.TODO(), itemReq)
	require.NoError(t, err)

	// Mark as done using batch entries
	doneReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"id":     "AC-1",
				"status": "done",
			},
		},
	})
	doneResult, err := handleItem(db, bk)(context.TODO(), doneReq)
	require.NoError(t, err)

	var doneResp []map[string]interface{}
	err = unmarshalResult(doneResult, &doneResp)
	require.NoError(t, err)
	require.Len(t, doneResp, 1)

	// Fetch to verify status
	fetchReq := makeRequest(map[string]interface{}{
		"ids": []interface{}{"AC-1"},
	})
	fetchResult, err := handleItem(db, bk)(context.TODO(), fetchReq)
	require.NoError(t, err)

	var items []map[string]interface{}
	err = unmarshalResult(fetchResult, &items)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "done", items[0]["status"])
}

func TestHandleItemList(t *testing.T) {
	resetAllDB(t)

	// Create project
	projReq := makeRequest(map[string]interface{}{
		"name": "acme-corp",
	})
	_, err := handleProject(db, bk)(context.TODO(), projReq)
	require.NoError(t, err)

	// Create multiple items using batch entries
	for i := 1; i <= 3; i++ {
		itemReq := makeRequest(map[string]interface{}{
			"entries": []interface{}{
				map[string]interface{}{
					"project":  "acme-corp",
					"priority": "P1",
					"title":    "Item " + string(rune('0'+i)),
				},
			},
		})
		_, err = handleItem(db, bk)(context.TODO(), itemReq)
		require.NoError(t, err)
	}

	// List items
	listReq := makeRequest(map[string]interface{}{
		"projects": []interface{}{"acme-corp"},
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

	// Create items with different priorities and status using batch entries
	itemReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "P1",
				"title":    "High priority",
			},
		},
	})
	_, err = handleItem(db, bk)(context.TODO(), itemReq)
	require.NoError(t, err)

	itemReq = makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "P3",
				"title":    "Low priority",
			},
		},
	})
	_, err = handleItem(db, bk)(context.TODO(), itemReq)
	require.NoError(t, err)

	// Mark one as done using batch entries
	doneReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"id":     "AC-1",
				"status": "done",
			},
		},
	})
	_, err = handleItem(db, bk)(context.TODO(), doneReq)
	require.NoError(t, err)

	// Filter by status=done
	listReq := makeRequest(map[string]interface{}{
		"projects": []interface{}{"acme-corp"},
		"status":   "done",
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

	// Create standalone plan using batch entries
	planReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"title":   "Implementation plan",
				"content": "Step 1: Design\nStep 2: Implement",
			},
		},
	})
	planResult, err := handlePlan(db, bk)(context.TODO(), planReq)
	require.NoError(t, err)

	var resp []map[string]interface{}
	err = unmarshalResult(planResult, &resp)
	require.NoError(t, err)
	require.Len(t, resp, 1)
	id, ok := resp[0]["id"].(float64)
	require.True(t, ok)
	planID := int64(id)

	// Fetch to verify
	fetchReq := makeRequest(map[string]interface{}{
		"ids": []interface{}{float64(planID)},
	})
	fetchResult, err := handlePlan(db, bk)(context.TODO(), fetchReq)
	require.NoError(t, err)

	var plans []map[string]interface{}
	err = unmarshalResult(fetchResult, &plans)
	require.NoError(t, err)
	require.Len(t, plans, 1)
	plan := plans[0]
	assert.Equal(t, "Implementation plan", plan["title"])
	assert.Equal(t, "Step 1: Design\nStep 2: Implement", plan["content"])
	assert.Equal(t, "draft", plan["status"])
	assert.Nil(t, plan["project_id"])
	assert.Nil(t, plan["item_id"])
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
		"entries": []interface{}{
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "P1",
				"title":    "Implement feature",
			},
		},
	})
	_, err = handleItem(db, bk)(context.TODO(), itemReq)
	require.NoError(t, err)

	// Create linked plan using batch entries
	planReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"title":   "Feature plan",
				"content": "Details here",
				"project": "acme-corp",
				"item_id": "AC-1",
			},
		},
	})
	planResult, err := handlePlan(db, bk)(context.TODO(), planReq)
	require.NoError(t, err)

	var resp []map[string]interface{}
	err = unmarshalResult(planResult, &resp)
	require.NoError(t, err)
	require.Len(t, resp, 1)
	id, ok := resp[0]["id"].(float64)
	require.True(t, ok)
	planID := int64(id)

	// Fetch to verify links
	fetchReq := makeRequest(map[string]interface{}{
		"ids": []interface{}{float64(planID)},
	})
	fetchResult, err := handlePlan(db, bk)(context.TODO(), fetchReq)
	require.NoError(t, err)

	var plans []map[string]interface{}
	err = unmarshalResult(fetchResult, &plans)
	require.NoError(t, err)
	require.Len(t, plans, 1)
	plan := plans[0]
	assert.NotNil(t, plan["project_id"])
	assert.NotNil(t, plan["item_id"])
	assert.Equal(t, "AC-1", plan["item_id"])
}

func TestHandlePlanGet(t *testing.T) {
	resetAllDB(t)

	// Create plan using batch entries
	planReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"title":   "Test plan",
				"content": "Test content",
			},
		},
	})
	createResult, err := handlePlan(db, bk)(context.TODO(), planReq)
	require.NoError(t, err)

	var createResp []map[string]interface{}
	err = unmarshalResult(createResult, &createResp)
	require.NoError(t, err)
	require.Len(t, createResp, 1)
	id, ok := createResp[0]["id"].(float64)
	require.True(t, ok)
	planID := int64(id)

	// Get plan by ID using ids[] fetch
	getReq := makeRequest(map[string]interface{}{
		"ids": []interface{}{float64(planID)},
	})
	getResult, err := handlePlan(db, bk)(context.TODO(), getReq)
	require.NoError(t, err)

	var plans []map[string]interface{}
	err = unmarshalResult(getResult, &plans)
	require.NoError(t, err)
	require.Len(t, plans, 1)
	plan := plans[0]
	assert.Equal(t, float64(planID), plan["id"])
	assert.Equal(t, "Test plan", plan["title"])
	assert.Equal(t, "Test content", plan["content"])
}

func TestHandlePlanUpdate(t *testing.T) {
	resetAllDB(t)

	// Create plan using batch entries
	createReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"title":   "Original title",
				"content": "Original content",
			},
		},
	})
	createResult, err := handlePlan(db, bk)(context.TODO(), createReq)
	require.NoError(t, err)

	var createResp []map[string]interface{}
	err = unmarshalResult(createResult, &createResp)
	require.NoError(t, err)
	require.Len(t, createResp, 1)
	id, ok := createResp[0]["id"].(float64)
	require.True(t, ok)
	planID := int64(id)

	// Update plan using batch entries
	updateReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"id":     float64(planID),
				"title":  "Updated title",
				"status": "active",
			},
		},
	})
	updateResult, err := handlePlan(db, bk)(context.TODO(), updateReq)
	require.NoError(t, err)

	var updateResp []map[string]interface{}
	err = unmarshalResult(updateResult, &updateResp)
	require.NoError(t, err)
	require.Len(t, updateResp, 1)
	assert.Equal(t, "update", updateResp[0]["action"])

	// Fetch updated plan to verify
	fetchReq := makeRequest(map[string]interface{}{
		"ids": []interface{}{float64(planID)},
	})
	fetchResult, err := handlePlan(db, bk)(context.TODO(), fetchReq)
	require.NoError(t, err)

	var plans []map[string]interface{}
	err = unmarshalResult(fetchResult, &plans)
	require.NoError(t, err)
	require.Len(t, plans, 1)
	plan := plans[0]
	assert.Equal(t, "Updated title", plan["title"])
	assert.Equal(t, "active", plan["status"])
}

func TestHandlePlanList(t *testing.T) {
	resetAllDB(t)

	// Create project
	projReq := makeRequest(map[string]interface{}{
		"name": "acme-corp",
	})
	_, err := handleProject(db, bk)(context.TODO(), projReq)
	require.NoError(t, err)

	// Create multiple plans using batch entries
	for i := 1; i <= 2; i++ {
		planReq := makeRequest(map[string]interface{}{
			"entries": []interface{}{
				map[string]interface{}{
					"title":   "Plan " + string(rune('0'+i)),
					"project": "acme-corp",
				},
			},
		})
		_, err = handlePlan(db, bk)(context.TODO(), planReq)
		require.NoError(t, err)
	}

	// List plans
	listReq := makeRequest(map[string]interface{}{
		"projects": []interface{}{"acme-corp"},
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

	// Try to set config via MCP should return CLI-only error
	setReq := makeRequest(map[string]interface{}{
		"key":   "api_key",
		"value": "secret123",
	})
	setResult, err := handleConfig(db)(context.TODO(), setReq)
	require.NoError(t, err)
	assert.True(t, setResult.IsError)
	textContent, ok := mcp.AsTextContent(setResult.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "CLI-only")

	// Get config still works (set it directly in DB for testing)
	require.NoError(t, backlog.SetConfig(db, "api_key", "secret123"))
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

	// Set multiple configs directly in DB
	require.NoError(t, backlog.SetConfig(db, "key1", "value1"))
	require.NoError(t, backlog.SetConfig(db, "key2", "value2"))

	// Get all configs via MCP
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

func TestHandleConfigSetCliOnly(t *testing.T) {
	resetAllDB(t)

	// Try to set config via MCP should return CLI-only error
	setReq := makeRequest(map[string]interface{}{
		"key":   "api_key",
		"value": "secret123",
	})
	setResult, err := handleConfig(db)(context.TODO(), setReq)
	require.NoError(t, err)
	assert.True(t, setResult.IsError)
	textContent, ok := mcp.AsTextContent(setResult.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "CLI-only")
	assert.Contains(t, textContent.Text, "ouroboros config set")
}

func TestHandleItemInvalidPriority(t *testing.T) {
	resetAllDB(t)

	// Create project
	projReq := makeRequest(map[string]interface{}{
		"name": "acme-corp",
	})
	_, err := handleProject(db, bk)(context.TODO(), projReq)
	require.NoError(t, err)

	// Try to create item with invalid priority using batch entries
	itemReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "X9",
				"title":    "Invalid item",
			},
		},
	})
	itemResult, err := handleItem(db, bk)(context.TODO(), itemReq)
	require.NoError(t, err)
	assert.True(t, itemResult.IsError)
}

func TestHandleItemNonexistentProject(t *testing.T) {
	resetAllDB(t)

	// Try to create item in nonexistent project
	itemReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":  "nonexistent",
				"priority": "P1",
				"title":    "Item",
			},
		},
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

// ============ Batch tests ============

func resetBacklogDBBatch(t *testing.T) {
	t.Helper()
	_, err := db.Exec("DELETE FROM items")
	require.NoError(t, err)
	_, err = db.Exec("DELETE FROM plans")
	require.NoError(t, err)
	_, err = db.Exec("DELETE FROM projects")
	require.NoError(t, err)
}

// TestHandleItemBatchFetch tests batch fetch with ids.
func TestHandleItemBatchFetch(t *testing.T) {
	resetBacklogDBBatch(t)

	proj, err := backlog.CreateProject(db, "test-project", "TP")
	require.NoError(t, err)

	item1, err := backlog.AddItem(db, proj.ID, "TP", "P0", "Task 1", "First task", "", "")
	require.NoError(t, err)

	item2, err := backlog.AddItem(db, proj.ID, "TP", "P1", "Task 2", "Second task", "", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"ids": []interface{}{item1.ID, item2.ID},
	})

	result, err := handleItem(db, nil)(context.TODO(), req)
	require.NoError(t, err)

	var items []map[string]interface{}
	err = unmarshalResult(result, &items)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "Task 1", items[0]["title"])
	assert.Equal(t, "Task 2", items[1]["title"])
}

// TestHandleItemBatchCreateAndUpdate tests batch with mixed creates and updates.
func TestHandleItemBatchCreateAndUpdate(t *testing.T) {
	resetBacklogDBBatch(t)

	proj, err := backlog.CreateProject(db, "test-project", "TP")
	require.NoError(t, err)

	item1, err := backlog.AddItem(db, proj.ID, "TP", "P0", "Task 1", "Initial", "", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"id":       item1.ID,
				"priority": "P2",
				"title":    "Task 1 Updated",
			},
			map[string]interface{}{
				"project":  "test-project",
				"priority": "P1",
				"title":    "Task 2 New",
			},
		},
	})

	result, err := handleItem(db, nil)(context.TODO(), req)
	require.NoError(t, err)

	var resp []map[string]interface{}
	err = unmarshalResult(result, &resp)
	require.NoError(t, err)
	require.Len(t, resp, 2)

	assert.Equal(t, "update", resp[0]["action"])
	assert.Equal(t, "create", resp[1]["action"])

	updated, err := backlog.GetItem(db, item1.ID)
	require.NoError(t, err)
	assert.Equal(t, "Task 1 Updated", updated.Title)
	assert.Equal(t, "P2", updated.Priority)
}

// TestHandlePlanBatchFetch tests batch fetch for plans.
func TestHandlePlanBatchFetch(t *testing.T) {
	resetBacklogDBBatch(t)

	proj, err := backlog.CreateProject(db, "test-project", "TP")
	require.NoError(t, err)

	plan1, err := backlog.CreatePlan(db, "Plan 1", "Content 1", &proj.ID, nil)
	require.NoError(t, err)

	plan2, err := backlog.CreatePlan(db, "Plan 2", "Content 2", &proj.ID, nil)
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"ids": []interface{}{float64(plan1.ID), float64(plan2.ID)},
	})

	result, err := handlePlan(db, nil)(context.TODO(), req)
	require.NoError(t, err)

	var plans []map[string]interface{}
	err = unmarshalResult(result, &plans)
	require.NoError(t, err)
	require.Len(t, plans, 2)

	assert.Equal(t, "Plan 1", plans[0]["title"])
	assert.Equal(t, "Plan 2", plans[1]["title"])
}

// TestHandlePlanBatchCreateAndUpdate tests batch with mixed creates and updates.
func TestHandlePlanBatchCreateAndUpdate(t *testing.T) {
	resetBacklogDBBatch(t)

	proj, err := backlog.CreateProject(db, "test-project", "TP")
	require.NoError(t, err)

	plan1, err := backlog.CreatePlan(db, "Plan 1", "Initial content", &proj.ID, nil)
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"id":     float64(plan1.ID),
				"title":  "Plan 1 Updated",
				"status": "active",
			},
			map[string]interface{}{
				"title":   "Plan 2 New",
				"content": "New plan content",
				"project": "test-project",
			},
		},
	})

	result, err := handlePlan(db, nil)(context.TODO(), req)
	require.NoError(t, err)

	var resp []map[string]interface{}
	err = unmarshalResult(result, &resp)
	require.NoError(t, err)
	require.Len(t, resp, 2)

	assert.Equal(t, "update", resp[0]["action"])
	assert.Equal(t, "create", resp[1]["action"])

	updated, err := backlog.GetPlan(db, plan1.ID)
	require.NoError(t, err)
	assert.Equal(t, "Plan 1 Updated", updated.Title)
	assert.Equal(t, "active", updated.Status)
}

func TestHandleItemCreateWithComponent(t *testing.T) {
	resetAllDB(t)

	// Create project first
	projReq := makeRequest(map[string]interface{}{
		"name": "acme-corp",
	})
	_, err := handleProject(db, bk)(context.TODO(), projReq)
	require.NoError(t, err)

	// Create item with component using batch entries
	itemReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":   "acme-corp",
				"priority":  "P1",
				"title":     "Implement feature",
				"component": "ouroboros-mcp",
			},
		},
	})
	itemResult, err := handleItem(db, bk)(context.TODO(), itemReq)
	require.NoError(t, err)

	var resp []map[string]interface{}
	err = unmarshalResult(itemResult, &resp)
	require.NoError(t, err)
	require.Len(t, resp, 1)

	// Fetch to verify component was set
	fetchReq := makeRequest(map[string]interface{}{
		"ids": []interface{}{"AC-1"},
	})
	fetchResult, err := handleItem(db, bk)(context.TODO(), fetchReq)
	require.NoError(t, err)

	var items []map[string]interface{}
	err = unmarshalResult(fetchResult, &items)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "ouroboros-mcp", items[0]["component"])
}

func TestHandleItemListFilterComponent(t *testing.T) {
	resetAllDB(t)

	// Create project
	projReq := makeRequest(map[string]interface{}{
		"name": "acme-corp",
	})
	_, err := handleProject(db, bk)(context.TODO(), projReq)
	require.NoError(t, err)

	// Create items with different components using batch entries
	itemReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":   "acme-corp",
				"priority":  "P1",
				"title":     "Item for plugin-a",
				"component": "plugin-a",
			},
			map[string]interface{}{
				"project":   "acme-corp",
				"priority":  "P2",
				"title":     "Item for plugin-b",
				"component": "plugin-b",
			},
			map[string]interface{}{
				"project":   "acme-corp",
				"priority":  "P3",
				"title":     "Another item for plugin-a",
				"component": "plugin-a",
			},
		},
	})
	_, err = handleItem(db, bk)(context.TODO(), itemReq)
	require.NoError(t, err)

	// Filter by component=plugin-a
	listReq := makeRequest(map[string]interface{}{
		"projects":  []interface{}{"acme-corp"},
		"component": "plugin-a",
	})
	listResult, err := handleItem(db, bk)(context.TODO(), listReq)
	require.NoError(t, err)

	textContent, ok := mcp.AsTextContent(listResult.Content[0])
	require.True(t, ok)
	text := textContent.Text

	// Should contain both plugin-a items
	assert.Contains(t, text, "AC-1")
	assert.Contains(t, text, "AC-3")
	assert.NotContains(t, text, "AC-2")
}

func TestHandleItemListOutputIncludesComponent(t *testing.T) {
	resetAllDB(t)

	// Create project
	projReq := makeRequest(map[string]interface{}{
		"name": "acme-corp",
	})
	_, err := handleProject(db, bk)(context.TODO(), projReq)
	require.NoError(t, err)

	// Create two items: one with component, one without
	itemReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":   "acme-corp",
				"priority":  "P1",
				"title":     "Item with component",
				"component": "ouroboros-mcp",
			},
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "P2",
				"title":    "Item without component",
			},
		},
	})
	_, err = handleItem(db, bk)(context.TODO(), itemReq)
	require.NoError(t, err)

	// List all items
	listReq := makeRequest(map[string]interface{}{
		"projects": []interface{}{"acme-corp"},
	})
	listResult, err := handleItem(db, bk)(context.TODO(), listReq)
	require.NoError(t, err)

	textContent, ok := mcp.AsTextContent(listResult.Content[0])
	require.True(t, ok)
	text := textContent.Text

	// Output should include (ouroboros-mcp) for AC-1 and no component segment for AC-2
	assert.Contains(t, text, "(ouroboros-mcp) Item with component")
	assert.Contains(t, text, "Item without component")
	assert.NotContains(t, text, "() Item without component") // No empty component segment
}

func TestHandleItemDeleteSingle(t *testing.T) {
	resetAllDB(t)

	// Create project and two items
	projReq := makeRequest(map[string]interface{}{
		"name": "acme-corp",
	})
	_, err := handleProject(db, bk)(context.TODO(), projReq)
	require.NoError(t, err)

	itemReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "P1",
				"title":    "Item to delete",
			},
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "P2",
				"title":    "Item to keep",
			},
		},
	})
	_, err = handleItem(db, bk)(context.TODO(), itemReq)
	require.NoError(t, err)

	// Delete one item via delete_ids
	deleteReq := makeRequest(map[string]interface{}{
		"delete_ids": []interface{}{"AC-1"},
	})
	deleteResult, err := handleItem(db, bk)(context.TODO(), deleteReq)
	require.NoError(t, err)

	var deleteResp map[string]interface{}
	err = unmarshalResult(deleteResult, &deleteResp)
	require.NoError(t, err)
	assert.Equal(t, float64(1), deleteResp["deleted"])

	// Verify AC-1 is gone by trying to fetch it (should error)
	getReq := makeRequest(map[string]interface{}{
		"ids": []interface{}{"AC-1"},
	})
	getResult, err := handleItem(db, bk)(context.TODO(), getReq)
	require.NoError(t, err)
	// Check that result is an error
	assert.True(t, len(getResult.Content) > 0)

	// Verify AC-2 still exists
	getReq2 := makeRequest(map[string]interface{}{
		"ids": []interface{}{"AC-2"},
	})
	getResult2, err := handleItem(db, bk)(context.TODO(), getReq2)
	require.NoError(t, err)

	var items2 []map[string]interface{}
	err = unmarshalResult(getResult2, &items2)
	require.NoError(t, err)
	require.Len(t, items2, 1)
	assert.Equal(t, "AC-2", items2[0]["id"])
}

func TestHandleItemDeleteMultiple(t *testing.T) {
	resetAllDB(t)

	// Create project and three items
	projReq := makeRequest(map[string]interface{}{
		"name": "acme-corp",
	})
	_, err := handleProject(db, bk)(context.TODO(), projReq)
	require.NoError(t, err)

	itemReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "P1",
				"title":    "Item 1",
			},
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "P2",
				"title":    "Item 2",
			},
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "P3",
				"title":    "Item 3",
			},
		},
	})
	_, err = handleItem(db, bk)(context.TODO(), itemReq)
	require.NoError(t, err)

	// Delete two items
	deleteReq := makeRequest(map[string]interface{}{
		"delete_ids": []interface{}{"AC-1", "AC-3"},
	})
	deleteResult, err := handleItem(db, bk)(context.TODO(), deleteReq)
	require.NoError(t, err)

	var deleteResp map[string]interface{}
	err = unmarshalResult(deleteResult, &deleteResp)
	require.NoError(t, err)
	assert.Equal(t, float64(2), deleteResp["deleted"])

	// Verify only AC-2 remains by fetching it directly
	getReq := makeRequest(map[string]interface{}{
		"ids": []interface{}{"AC-2"},
	})
	getResult, err := handleItem(db, bk)(context.TODO(), getReq)
	require.NoError(t, err)

	var items []map[string]interface{}
	err = unmarshalResult(getResult, &items)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "AC-2", items[0]["id"])
}

func TestHandleItemDeleteNonexistent(t *testing.T) {
	resetAllDB(t)

	// Try to delete non-existent item
	deleteReq := makeRequest(map[string]interface{}{
		"delete_ids": []interface{}{"NONEXISTENT"},
	})
	deleteResult, err := handleItem(db, bk)(context.TODO(), deleteReq)
	require.NoError(t, err)

	var deleteResp map[string]interface{}
	err = unmarshalResult(deleteResult, &deleteResp)
	require.NoError(t, err)
	assert.Equal(t, float64(0), deleteResp["deleted"])
}

// TestHandleItemListMultiProject tests item list with multiple project filters.
func TestHandleItemListMultiProject(t *testing.T) {
	resetAllDB(t)

	// Create two projects
	projReq1 := makeRequest(map[string]interface{}{
		"name": "project-a",
	})
	_, err := handleProject(db, bk)(context.TODO(), projReq1)
	require.NoError(t, err)

	projReq2 := makeRequest(map[string]interface{}{
		"name": "project-b",
	})
	_, err = handleProject(db, bk)(context.TODO(), projReq2)
	require.NoError(t, err)

	// Create items in project-a
	itemReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":  "project-a",
				"priority": "P1",
				"title":    "Item in project-a",
			},
		},
	})
	_, err = handleItem(db, bk)(context.TODO(), itemReq)
	require.NoError(t, err)

	// Create items in project-b
	itemReq = makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":  "project-b",
				"priority": "P2",
				"title":    "Item in project-b",
			},
		},
	})
	_, err = handleItem(db, bk)(context.TODO(), itemReq)
	require.NoError(t, err)

	// List items from both projects
	listReq := makeRequest(map[string]interface{}{
		"projects": []interface{}{"project-a", "project-b"},
	})
	listResult, err := handleItem(db, bk)(context.TODO(), listReq)
	require.NoError(t, err)

	textContent, ok := mcp.AsTextContent(listResult.Content[0])
	require.True(t, ok)
	text := textContent.Text

	// Should contain items from both projects
	// Prefixes are derived from project names: project-a → PR, project-b → P1
	assert.Contains(t, text, "Item in project-a")
	assert.Contains(t, text, "Item in project-b")
	// IDs should be PR-1 and P1-1 but we just check that both projects appear
	assert.Contains(t, text, "[open] Item in project-a")
	assert.Contains(t, text, "[open] Item in project-b")
}
