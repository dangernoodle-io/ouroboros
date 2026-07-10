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
	_, err = db.Exec("DELETE FROM edges")
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

// ============ Batch write tests ============

func resetBacklogDBBatch(t *testing.T) {
	t.Helper()
	_, err := db.Exec("DELETE FROM items")
	require.NoError(t, err)
	_, err = db.Exec("DELETE FROM plans")
	require.NoError(t, err)
	_, err = db.Exec("DELETE FROM projects")
	require.NoError(t, err)
	_, err = db.Exec("DELETE FROM edges")
	require.NoError(t, err)
}

// TestHandleBacklogBatchCreateAndUpdate tests batch with mixed creates and updates.
func TestHandleBacklogBatchCreateAndUpdate(t *testing.T) {
	resetBacklogDBBatch(t)

	proj, err := backlog.CreateProject(db, "test-project", "TP")
	require.NoError(t, err)

	item1, err := backlog.AddItem(db, proj.ID, "TP", "P0", "Task 1", "Initial", "", "", "")
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

	result, err := handleBacklog(db, nil)(context.TODO(), req)
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

func TestHandleBacklogDeleteNonexistent(t *testing.T) {
	resetAllDB(t)

	// Try to delete non-existent item
	deleteReq := makeRequest(map[string]interface{}{
		"delete_ids": []interface{}{"NONEXISTENT"},
	})
	deleteResult, err := handleBacklog(db, bk)(context.TODO(), deleteReq)
	require.NoError(t, err)

	var deleteResp map[string]interface{}
	err = unmarshalResult(deleteResult, &deleteResp)
	require.NoError(t, err)
	assert.Equal(t, float64(0), deleteResp["deleted"])
}

// TestHandleBacklogCreate creates an item via entries=[{...}] and verifies response.
func TestHandleBacklogCreate(t *testing.T) {
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
	result, err := handleBacklog(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var resp []map[string]interface{}
	err = unmarshalResult(result, &resp)
	require.NoError(t, err)
	require.Len(t, resp, 1)
	assert.Equal(t, "create", resp[0]["action"])
	assert.Equal(t, "AC-1", resp[0]["id"])
}

// TestHandleBacklogCreateWithNotesAndComponent creates an item with notes and component.
func TestHandleBacklogCreateWithNotesAndComponent(t *testing.T) {
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
	result, err := handleBacklog(db, nil)(context.TODO(), req)
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

// TestHandleBacklogCreateWithEpic verifies entries[].epic is persisted on
// create when it resolves to a pre-existing item.
func TestHandleBacklogCreateWithEpic(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	epic, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "EPIC: demo", "", "", "", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "P1",
				"title":    "epic child",
				"epic":     epic.ID,
			},
		},
	})
	result, err := handleBacklog(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	item, err := backlog.GetItem(db, "AC-2")
	require.NoError(t, err)
	assert.Equal(t, epic.ID, item.Epic)
}

// TestHandleBacklogCreateEpicNotFound_Errors verifies create rejects a
// dangling/typo'd epic id — mirrors the edge target-exists check.
func TestHandleBacklogCreateEpicNotFound_Errors(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "P1",
				"title":    "orphan child",
				"epic":     "AC-999",
			},
		},
	})
	result, err := handleBacklog(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.True(t, result.IsError)
	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, `epic item "AC-999" not found`)

	// Nothing persisted: the bad epic rolls back the whole atomic create.
	_, err = backlog.GetItem(db, "AC-1")
	assert.Error(t, err)
}

// TestHandleBacklogCreateWithAliasResolvedEpic verifies an epic id referring
// to a renamed item (via item_id_aliases) still validates — mirrors the
// EpicLabels alias-fallback path.
func TestHandleBacklogCreateWithAliasResolvedEpic(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	epic, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "EPIC: renamed", "", "", "", "")
	require.NoError(t, err)

	_, err = db.Exec("INSERT INTO item_id_aliases (old_id, new_id, renamed_at) VALUES (?, ?, ?)",
		"OLD-1", epic.ID, "2024-01-01T00:00:00Z")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "P1",
				"title":    "epic child via old id",
				"epic":     "OLD-1",
			},
		},
	})
	result, err := handleBacklog(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)
}

// TestHandleBacklogCreateEpicNotScalar_Errors verifies create rejects a
// non-string epic (single-valued enforcement).
func TestHandleBacklogCreateEpicNotScalar_Errors(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "P1",
				"title":    "x",
				"epic":     []interface{}{"AC-1", "AC-2"},
			},
		},
	})
	result, err := handleBacklog(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.True(t, result.IsError)
	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "single-valued")
}

// TestHandleBacklogUpdateComponent verifies entries[].component patches an
// existing item (the success path — see
// TestHandleBacklogUpdateComponentNotScalar_Errors for the rejection path).
func TestHandleBacklogUpdateComponent(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "task", "", "", "", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{"id": item.ID, "component": "widget"},
		},
	})
	result, err := handleBacklog(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	updated, err := backlog.GetItem(db, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "widget", updated.Component)
}

// TestHandleBacklogUpdateEpic verifies entries[].epic patches an existing
// item when it resolves to a pre-existing epic item.
func TestHandleBacklogUpdateEpic(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	epic, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "EPIC: demo", "", "", "", "")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "task", "", "", "", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{"id": item.ID, "epic": epic.ID},
		},
	})
	result, err := handleBacklog(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	updated, err := backlog.GetItem(db, item.ID)
	require.NoError(t, err)
	assert.Equal(t, epic.ID, updated.Epic)
}

// TestHandleBacklogUpdateEpicNotFound_Errors verifies update rejects a
// dangling/typo'd epic id and rolls back the whole write.
func TestHandleBacklogUpdateEpicNotFound_Errors(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "task", "", "", "", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{"id": item.ID, "priority": "P3", "epic": "AC-999"},
		},
	})
	result, err := handleBacklog(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.True(t, result.IsError)
	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, `epic item "AC-999" not found`)

	// Nothing changed: the bad epic rolls back the priority change too.
	unchanged, err := backlog.GetItem(db, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "P1", unchanged.Priority)
	assert.Empty(t, unchanged.Epic)
}

// TestHandleBacklogUpdateEpicClear verifies passing an empty epic on update
// is allowed (no validation on empty — clearing stays a no-op given epic is
// only set when non-empty, matching the component field's convention).
func TestHandleBacklogUpdateEpicClear(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "task", "", "", "", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{"id": item.ID, "priority": "P2", "epic": ""},
		},
	})
	result, err := handleBacklog(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	updated, err := backlog.GetItem(db, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "P2", updated.Priority)
	assert.Empty(t, updated.Epic)
}

// TestHandleBacklogUpdateEpicNotScalar_Errors verifies update rejects a
// non-string epic (single-valued enforcement).
func TestHandleBacklogUpdateEpicNotScalar_Errors(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "task", "", "", "", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{"id": item.ID, "epic": []interface{}{"AC-1", "AC-2"}},
		},
	})
	result, err := handleBacklog(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.True(t, result.IsError)
	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "single-valued")
}

// TestHandleBacklogCreateComponentNotScalar_Errors verifies create rejects a
// non-string component (single-valued enforcement).
func TestHandleBacklogCreateComponentNotScalar_Errors(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":   "acme-corp",
				"priority":  "P1",
				"title":     "x",
				"component": []interface{}{"a", "b"},
			},
		},
	})
	result, err := handleBacklog(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.True(t, result.IsError)
	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "single-valued")
}

// TestHandleBacklogUpdateComponentNotScalar_Errors verifies update rejects a
// non-string component (single-valued enforcement).
func TestHandleBacklogUpdateComponentNotScalar_Errors(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "task", "", "", "", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{"id": item.ID, "component": []interface{}{"a", "b"}},
		},
	})
	result, err := handleBacklog(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.True(t, result.IsError)
	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "single-valued")
}

// TestHandleBacklogInvalidPriority verifies error on bad priority in create mode.
func TestHandleBacklogInvalidPriority(t *testing.T) {
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
	result, err := handleBacklog(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleBacklogNonexistentProject verifies error when project doesn't exist.
func TestHandleBacklogNonexistentProject(t *testing.T) {
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
	result, err := handleBacklog(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleBacklogDescriptionTooLong verifies error when description exceeds 500 chars.
func TestHandleBacklogDescriptionTooLong(t *testing.T) {
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
	result, err := handleBacklog(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "description exceeds 500 char hard cap")
}

// TestHandleBacklogUpdateInvalidPriority verifies error on bad priority in update mode.
func TestHandleBacklogUpdateInvalidPriority(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Original task", "", "", "", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"id":       item.ID,
				"priority": "INVALID",
			},
		},
	})
	result, err := handleBacklog(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleBacklogDeleteMultiple creates two items then deletes both via delete_ids.
func TestHandleBacklogDeleteMultiple(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item1, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Delete me 1", "", "", "", "")
	require.NoError(t, err)
	item2, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P2", "Delete me 2", "", "", "", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"delete_ids": []interface{}{item1.ID, item2.ID},
	})
	result, err := handleBacklog(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var resp map[string]interface{}
	require.NoError(t, unmarshalResult(result, &resp))
	assert.Equal(t, float64(2), resp["deleted"])
}

// TestHandleBacklogBatchCreate creates multiple items in a single entries call.
func TestHandleBacklogBatchCreate(t *testing.T) {
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
	result, err := handleBacklog(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var resp []map[string]interface{}
	require.NoError(t, unmarshalResult(result, &resp))
	require.Len(t, resp, 3)
	for _, r := range resp {
		assert.Equal(t, "create", r["action"])
	}
}

// TestHandleBacklogNoEntriesOrDeleteIDs_Errors verifies backlog rejects a call with neither
// delete_ids nor entries — this is a write-only tool now, reads live under get/search.
func TestHandleBacklogNoEntriesOrDeleteIDs_Errors(t *testing.T) {
	resetAllDB(t)

	req := makeRequest(map[string]interface{}{
		"projects": []interface{}{"acme-corp"},
	})
	result, err := handleBacklog(db, bk)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "delete_ids or entries is required")
}
