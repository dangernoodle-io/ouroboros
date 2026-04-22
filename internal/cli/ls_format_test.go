package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/store"
)

func TestPrintTableBasic(t *testing.T) {
	var buf bytes.Buffer
	headers := []string{"Name", "Age"}
	rows := [][]string{
		{"Alice", "30"},
		{"Bob", "25"},
	}
	err := printTable(&buf, headers, rows)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Name")
	assert.Contains(t, output, "Age")
	assert.Contains(t, output, "Alice")
	assert.Contains(t, output, "30")
}

func TestPrintTableEmpty(t *testing.T) {
	var buf bytes.Buffer
	headers := []string{"ID", "Title"}
	err := printTable(&buf, headers, [][]string{})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "ID")
	assert.Contains(t, output, "Title")
}

func TestPrintJSON(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]string{"key": "value"}
	err := printJSON(&buf, data)
	require.NoError(t, err)

	var unmarshalled map[string]string
	err = json.Unmarshal(buf.Bytes(), &unmarshalled)
	require.NoError(t, err)
	assert.Equal(t, "value", unmarshalled["key"])
}

func TestFormatItemDetail(t *testing.T) {
	var buf bytes.Buffer
	item := &backlog.Item{
		ID:          "AC-1",
		Priority:    "P0",
		Status:      "open",
		Title:       "Fix critical bug",
		Component:   "auth",
		Description: "Investigate and fix the login issue",
		Notes:       "Blocker for release",
	}
	formatItemDetail(&buf, item, "acme-corp")

	output := buf.String()
	assert.Contains(t, output, "AC-1")
	assert.Contains(t, output, "[P0]")
	assert.Contains(t, output, "[open]")
	assert.Contains(t, output, "acme-corp")
	assert.Contains(t, output, "Component: auth")
	assert.Contains(t, output, "Fix critical bug")
	assert.Contains(t, output, "Description:")
	assert.Contains(t, output, "Notes:")
}

func TestFormatItemDetailNoComponent(t *testing.T) {
	var buf bytes.Buffer
	item := &backlog.Item{
		ID:          "OP-5",
		Priority:    "P3",
		Status:      "done",
		Title:       "Task",
		Component:   "",
		Description: "Desc",
	}
	formatItemDetail(&buf, item, "other-project")

	output := buf.String()
	assert.NotContains(t, output, "Component:")
	assert.Contains(t, output, "OP-5")
}

func TestFormatKBDetail(t *testing.T) {
	var buf bytes.Buffer
	doc := &store.Document{
		ID:       42,
		Type:     "decision",
		Project:  "acme-corp",
		Category: "architecture",
		Title:    "Use PostgreSQL",
		Content:  "PostgreSQL provides better performance",
		Tags:     []string{"database", "backend"},
		Notes:    "Approved by team",
	}
	formatKBDetail(&buf, doc)

	output := buf.String()
	assert.Contains(t, output, "42")
	assert.Contains(t, output, "[decision]")
	assert.Contains(t, output, "acme-corp")
	assert.Contains(t, output, "Category: architecture")
	assert.Contains(t, output, "Use PostgreSQL")
	assert.Contains(t, output, "database, backend")
	assert.Contains(t, output, "Content:")
	assert.Contains(t, output, "Notes:")
}

func TestFormatPlanDetail(t *testing.T) {
	var buf bytes.Buffer
	plan := &backlog.Plan{
		ID:      17,
		Status:  "active",
		Title:   "Q1 roadmap",
		Content: "Plan the quarterly work",
	}
	formatPlanDetail(&buf, plan, "acme-corp")

	output := buf.String()
	assert.Contains(t, output, "17")
	assert.Contains(t, output, "[active]")
	assert.Contains(t, output, "acme-corp")
	assert.Contains(t, output, "Q1 roadmap")
	assert.Contains(t, output, "Content:")
}
