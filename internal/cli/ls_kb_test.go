package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/store"
)

func TestLSKBList(t *testing.T) {
	db := newTestDB(t)
	_, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Use PostgreSQL",
		Tags:    []string{"database", "backend"},
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type:    "fact",
		Project: "acme-corp",
		Title:   "Team size is 5",
		Tags:    []string{"org"},
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSKB(&buf, db, "", "", "", []string{}, "", 50, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "ID")
	assert.Contains(t, output, "TYPE")
	assert.Contains(t, output, "decision")
	assert.Contains(t, output, "fact")
	assert.Contains(t, output, "Use PostgreSQL")
	assert.Contains(t, output, "Team size is 5")
}

func TestLSKBListJSON(t *testing.T) {
	db := newTestDB(t)
	result, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Use PostgreSQL",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSKB(&buf, db, "", "", "", []string{}, "", 50, true)
	require.NoError(t, err)

	var summaries []store.DocumentSummary
	err = json.Unmarshal(buf.Bytes(), &summaries)
	require.NoError(t, err)
	require.Greater(t, len(summaries), 0)
	assert.Equal(t, result.ID, summaries[0].ID)
}

func TestLSKBTypeFilter(t *testing.T) {
	db := newTestDB(t)
	_, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Doc1",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type:    "fact",
		Project: "acme-corp",
		Title:   "Doc2",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSKB(&buf, db, "", "decision", "", []string{}, "", 50, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "decision")
	assert.Contains(t, output, "Doc1")
	// Fact should not be in the results
	assert.NotContains(t, output, "fact")
}

func TestLSKBSearch(t *testing.T) {
	db := newTestDB(t)
	_, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Database choice",
		Content: "We chose PostgreSQL for its performance",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type:    "fact",
		Project: "acme-corp",
		Title:   "Team size",
		Content: "5 engineers",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSKB(&buf, db, "", "", "", []string{}, "PostgreSQL", 50, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Database choice")
	// Team size fact should not match
	assert.NotContains(t, output, "Team size")
}

func TestLSKBCategoryFilter(t *testing.T) {
	db := newTestDB(t)
	_, err := store.UpsertDocument(db, store.Document{
		Type:     "decision",
		Project:  "acme-corp",
		Category: "architecture",
		Title:    "Architecture decision",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type:     "decision",
		Project:  "acme-corp",
		Category: "process",
		Title:    "Process decision",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSKB(&buf, db, "", "", "architecture", []string{}, "", 50, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Architecture decision")
	assert.NotContains(t, output, "Process decision")
}

func TestLSKBProjectFilter(t *testing.T) {
	db := newTestDB(t)
	_, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "ACME decision",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "other-project",
		Title:   "OTHER decision",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSKB(&buf, db, "acme-corp", "", "", []string{}, "", 50, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "ACME decision")
	assert.NotContains(t, output, "OTHER decision")
}

func TestLSKBDetailJSON(t *testing.T) {
	db := newTestDB(t)
	result, err := store.UpsertDocument(db, store.Document{
		Type:     "decision",
		Project:  "acme-corp",
		Category: "arch",
		Title:    "Use PostgreSQL",
		Content:  "For performance",
		Tags:     []string{"db", "backend"},
		Notes:    "Approved",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSKBDetail(&buf, db, "1", true)
	require.NoError(t, err)

	var doc store.Document
	err = json.Unmarshal(buf.Bytes(), &doc)
	require.NoError(t, err)
	assert.Equal(t, result.ID, doc.ID)
	assert.Equal(t, "decision", doc.Type)
	assert.Equal(t, "Use PostgreSQL", doc.Title)
}

func TestLSKBDetailPlain(t *testing.T) {
	db := newTestDB(t)
	_, err := store.UpsertDocument(db, store.Document{
		Type:     "decision",
		Project:  "acme-corp",
		Category: "architecture",
		Title:    "Use PostgreSQL",
		Content:  "Performance benefits",
		Tags:     []string{"database", "backend"},
		Notes:    "Team consensus",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSKBDetail(&buf, db, "1", false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "[decision]")
	assert.Contains(t, output, "acme-corp")
	assert.Contains(t, output, "Category: architecture")
	assert.Contains(t, output, "Use PostgreSQL")
	assert.Contains(t, output, "database, backend")
	assert.Contains(t, output, "Content:")
	assert.Contains(t, output, "Notes:")
}

func TestLSKBLimit(t *testing.T) {
	db := newTestDB(t)
	for i := 0; i < 5; i++ {
		_, err := store.UpsertDocument(db, store.Document{
			Type:    "fact",
			Project: "acme-corp",
			Title:   fmt.Sprintf("Fact %d", i),
		})
		require.NoError(t, err)
	}

	var buf bytes.Buffer
	err := runLSKB(&buf, db, "", "", "", []string{}, "", 2, true)
	require.NoError(t, err)

	var summaries []store.DocumentSummary
	err = json.Unmarshal(buf.Bytes(), &summaries)
	require.NoError(t, err)
	assert.Equal(t, 2, len(summaries))
}
