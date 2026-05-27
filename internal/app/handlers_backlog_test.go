package app

import (
	"context"
	"strings"
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

func TestHandleItemListReturnsNoItems(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	handler := handleItem(db, bk)

	// Call with list mode — should succeed and return "no items"
	req := makeRequest(map[string]interface{}{
		"projects": []interface{}{"acme-corp"},
	})
	result, err := handler(context.TODO(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
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

func TestHandleItemListOutput(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Task one", "", "", "")
	require.NoError(t, err)

	listReq := makeRequest(map[string]interface{}{
		"projects": []interface{}{"acme-corp"},
	})
	listResult, err := handleItem(db, bk)(context.TODO(), listReq)
	require.NoError(t, err)

	textContent, ok := mcp.AsTextContent(listResult.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "AC-1")
	assert.Contains(t, textContent.Text, "Task one")
}

// TestHandleItemCreate creates an item via entries=[{...}] and verifies response.
func TestHandleItemCreate(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "P2",
				"title":    "New task",
			},
		},
	})
	result, err := handleItem(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var resp []map[string]interface{}
	err = unmarshalResult(result, &resp)
	require.NoError(t, err)
	require.Len(t, resp, 1)
	assert.Equal(t, "create", resp[0]["action"])
	assert.Equal(t, "AC-1", resp[0]["id"])
}

// TestHandleItemCreateWithNotesAndComponent creates an item with notes and component.
func TestHandleItemCreateWithNotesAndComponent(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":     "acme-corp",
				"priority":    "P1",
				"title":       "Task with extras",
				"description": "A short description",
				"notes":       "Detailed notes here",
				"component":   "api",
			},
		},
	})
	result, err := handleItem(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var resp []map[string]interface{}
	require.NoError(t, unmarshalResult(result, &resp))
	require.Len(t, resp, 1)
	assert.Equal(t, "create", resp[0]["action"])

	// Verify stored data
	item, err := backlog.GetItem(db, "AC-1")
	require.NoError(t, err)
	assert.Equal(t, "Detailed notes here", item.Notes)
	assert.Equal(t, "api", item.Component)
}

// TestHandleItemInvalidPriority verifies error on bad priority in create mode.
func TestHandleItemInvalidPriority(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "P9",
				"title":    "Bad priority task",
			},
		},
	})
	result, err := handleItem(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleItemNonexistentProject verifies error when project doesn't exist.
func TestHandleItemNonexistentProject(t *testing.T) {
	resetAllDB(t)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":  "nonexistent-project",
				"priority": "P1",
				"title":    "Orphan task",
			},
		},
	})
	result, err := handleItem(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleItemDescriptionTooLong verifies error when description exceeds 500 chars.
func TestHandleItemDescriptionTooLong(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	longDesc := strings.Repeat("a", 502)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":     "acme-corp",
				"priority":    "P1",
				"title":       "Long desc task",
				"description": longDesc,
			},
		},
	})
	result, err := handleItem(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "description exceeds 500 char hard cap")
}

// TestHandleItemUpdateInvalidPriority verifies error on bad priority in update mode.
func TestHandleItemUpdateInvalidPriority(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Original task", "", "", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"id":       item.ID,
				"priority": "INVALID",
			},
		},
	})
	result, err := handleItem(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleItemListPriorityFilter tests listing with priority_min and priority_max.
func TestHandleItemListPriorityFilter(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Critical", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P2", "Normal", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P5", "Low", "", "", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"projects":     []interface{}{"acme-corp"},
		"priority_min": "P1",
		"priority_max": "P3",
	})
	result, err := handleItem(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "Normal")
	assert.NotContains(t, textContent.Text, "Critical")
	assert.NotContains(t, textContent.Text, "Low")
}

// TestHandleItemListPriorityMinInvalid verifies error on bad priority_min.
func TestHandleItemListPriorityMinInvalid(t *testing.T) {
	resetAllDB(t)

	req := makeRequest(map[string]interface{}{
		"priority_min": "X9",
	})
	result, err := handleItem(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleItemListPriorityMaxInvalid verifies error on bad priority_max.
func TestHandleItemListPriorityMaxInvalid(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"priority_min": "P0",
		"priority_max": "P9",
	})
	result, err := handleItem(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleItemListStatusFilter tests filtering by status.
func TestHandleItemListStatusFilter(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Open task", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P2", "Another task", "", "", "")
	require.NoError(t, err)
	err = backlog.MarkDone(db, item.ID)
	require.NoError(t, err)

	// Filter for open only
	req := makeRequest(map[string]interface{}{
		"projects": []interface{}{"acme-corp"},
		"status":   "open",
	})
	result, err := handleItem(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "Another task")
	assert.NotContains(t, textContent.Text, "Open task")
}

// TestHandleItemListComponentFilter tests filtering by component.
func TestHandleItemListComponentFilter(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "API task", "", "", "api")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P2", "UI task", "", "", "ui")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"projects":  []interface{}{"acme-corp"},
		"component": "api",
	})
	result, err := handleItem(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	// Component appears in list output
	assert.Contains(t, textContent.Text, "api")
	assert.Contains(t, textContent.Text, "API task")
}

// TestHandleItemListNonexistentProject verifies error when filtering by bad project.
func TestHandleItemListNonexistentProject(t *testing.T) {
	resetAllDB(t)

	req := makeRequest(map[string]interface{}{
		"projects": []interface{}{"no-such-project"},
	})
	result, err := handleItem(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleItemListMultiProject tests listing across two projects.
func TestHandleItemListMultiProject(t *testing.T) {
	resetAllDB(t)

	proj1, err := backlog.CreateProject(db, "project-alpha", "PA")
	require.NoError(t, err)
	proj2, err := backlog.CreateProject(db, "project-beta", "PB")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj1.ID, proj1.Prefix, "P1", "Alpha task", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj2.ID, proj2.Prefix, "P2", "Beta task", "", "", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"projects": []interface{}{"project-alpha", "project-beta"},
	})
	result, err := handleItem(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "Alpha task")
	assert.Contains(t, textContent.Text, "Beta task")
}

// TestHandleItemDeleteMultiple creates two items then deletes both via delete_ids.
func TestHandleItemDeleteMultiple(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item1, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Delete me 1", "", "", "")
	require.NoError(t, err)
	item2, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P2", "Delete me 2", "", "", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"delete_ids": []interface{}{item1.ID, item2.ID},
	})
	result, err := handleItem(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var resp map[string]interface{}
	require.NoError(t, unmarshalResult(result, &resp))
	assert.Equal(t, float64(2), resp["deleted"])
}

// TestHandleItemBatchCreate creates multiple items in a single entries call.
func TestHandleItemBatchCreate(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "P0",
				"title":    "Batch task 1",
			},
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "P1",
				"title":    "Batch task 2",
			},
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "P2",
				"title":    "Batch task 3",
			},
		},
	})
	result, err := handleItem(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var resp []map[string]interface{}
	require.NoError(t, unmarshalResult(result, &resp))
	require.Len(t, resp, 3)
	for _, r := range resp {
		assert.Equal(t, "create", r["action"])
	}
}

// TestHandleItemGetVerbose tests ids fetch with verbose=true (notes are included).
func TestHandleItemGetVerbose(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Verbose task", "", "secret notes", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"ids":     []interface{}{item.ID},
		"verbose": true,
	})
	result, err := handleItem(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var items []map[string]interface{}
	require.NoError(t, unmarshalResult(result, &items))
	require.Len(t, items, 1)
	assert.Equal(t, "secret notes", items[0]["notes"])
}

// TestHandleItemGetMiss tests ids fetch where an ID doesn't exist returns an error.
func TestHandleItemGetMiss(t *testing.T) {
	resetAllDB(t)

	// Requesting a nonexistent item ID returns an error result
	req := makeRequest(map[string]interface{}{
		"ids": []interface{}{"AC-9999"},
	})
	result, err := handleItem(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError, "fetching nonexistent item should return error result")
}
