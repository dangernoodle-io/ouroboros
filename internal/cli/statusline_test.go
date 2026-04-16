package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/store"
)

func TestRunStatuslineEmpty(t *testing.T) {
	db := newTestDB(t)
	var buf bytes.Buffer
	err := runStatusline(&buf, db, "", false)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestRunStatuslineKBOnly(t *testing.T) {
	db := newTestDB(t)

	// Insert KB documents
	_, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "ouroboros",
		Title:   "Use PostgreSQL",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "ouroboros",
		Title:   "Use SQLite",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type:    "fact",
		Project: "ouroboros",
		Title:   "Database location",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runStatusline(&buf, db, "", false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "KB 3")
	assert.Contains(t, output, "2D") // 2 decisions
	assert.Contains(t, output, "1F") // 1 fact
}

func TestRunStatuslineBacklogOnly(t *testing.T) {
	db := newTestDB(t)

	// Create a project and add items
	p, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.AddItem(db, p.ID, "AC", "P0", "Critical task", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, p.ID, "AC", "P1", "High priority task", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, p.ID, "AC", "P1", "Another high priority", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runStatusline(&buf, db, "", false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "BL 3")
	assert.Contains(t, output, "open")
}

func TestRunStatuslineFull(t *testing.T) {
	db := newTestDB(t)

	// Add KB entries
	_, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "ouroboros",
		Title:   "Use SQLite",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type:    "fact",
		Project: "ouroboros",
		Title:   "Fact 1",
	})
	require.NoError(t, err)

	// Add backlog items
	p, err := backlog.CreateProject(db, "ouroboros", "OUR")
	require.NoError(t, err)

	_, err = backlog.AddItem(db, p.ID, "OUR", "P1", "Task 1", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, p.ID, "OUR", "P2", "Task 2", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runStatusline(&buf, db, "", false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "KB 2")
	assert.Contains(t, output, "BL 2")
	assert.Contains(t, output, "open")
}

func TestRunStatuslineJSON(t *testing.T) {
	db := newTestDB(t)

	// Add KB entries
	_, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "test-project",
		Title:   "Decision 1",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "test-project",
		Title:   "Decision 2",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type:    "fact",
		Project: "test-project",
		Title:   "Fact 1",
	})
	require.NoError(t, err)

	// Add backlog items
	p, err := backlog.CreateProject(db, "test-project", "TP")
	require.NoError(t, err)

	_, err = backlog.AddItem(db, p.ID, "TP", "P0", "Critical item", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, p.ID, "TP", "P1", "High priority item", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runStatusline(&buf, db, "", true)
	require.NoError(t, err)

	var data statuslineData
	require.NoError(t, json.Unmarshal(buf.Bytes(), &data))

	assert.Equal(t, 3, data.KB.Total)
	assert.Equal(t, 2, data.Backlog.Total)
	assert.Len(t, data.KB.Types, 2) // decision and fact

	// Verify priority counts
	assert.Len(t, data.Backlog.Items, 2) // P0 and P1
}

func TestRunStatuslineProjectFilter(t *testing.T) {
	db := newTestDB(t)

	// Add KB entries for different projects
	_, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "project-a",
		Title:   "Decision A1",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "project-a",
		Title:   "Decision A2",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "project-b",
		Title:   "Decision B1",
	})
	require.NoError(t, err)

	// Add backlog items
	pA, err := backlog.CreateProject(db, "project-a", "PA")
	require.NoError(t, err)
	pB, err := backlog.CreateProject(db, "project-b", "PB")
	require.NoError(t, err)

	_, err = backlog.AddItem(db, pA.ID, "PA", "P1", "Item A1", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, pB.ID, "PB", "P1", "Item B1", "", "", "")
	require.NoError(t, err)

	// Query with project filter
	var buf bytes.Buffer
	err = runStatusline(&buf, db, "project-a", true)
	require.NoError(t, err)

	var data statuslineData
	require.NoError(t, json.Unmarshal(buf.Bytes(), &data))

	assert.Equal(t, "project-a", data.Project)
	assert.Equal(t, 2, data.KB.Total) // only project-a
	assert.Equal(t, 1, data.Backlog.Total)
}

func TestTypeAbbrev(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"decision", "D"},
		{"fact", "F"},
		{"note", "N"},
		{"plan", "P"},
		{"relation", "R"},
		{"", "?"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, typeAbbrev(tt.input))
		})
	}
}

func TestPriorityColor(t *testing.T) {
	tests := []struct {
		priority string
		expected string
	}{
		{"P0", "\033[31m"}, // red
		{"P1", "\033[33m"}, // yellow
		{"P2", "\033[36m"}, // cyan
		{"P3", "\033[2m"},  // dim
		{"P6", "\033[2m"},  // dim
	}

	for _, tt := range tests {
		t.Run(tt.priority, func(t *testing.T) {
			assert.Equal(t, tt.expected, priorityColor(tt.priority))
		})
	}
}
