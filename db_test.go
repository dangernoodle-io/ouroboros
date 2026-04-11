package main

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testDB creates an in-memory database for testing.
func testDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	require.NoError(t, applySchema(db))
	t.Cleanup(func() { db.Close() })
	return db
}

func TestUpsertAndGetDocument(t *testing.T) {
	db := testDB(t)

	doc := Document{
		Type:     "note",
		Project:  "acme-corp",
		Category: "procedure",
		Title:    "release-process",
		Content:  "1. Tag\n2. Push\n3. Monitor",
		Metadata: map[string]string{"version": "1.0"},
		Tags:     []string{"release", "ci"},
	}

	id, err := upsertDocument(db, doc)
	require.NoError(t, err)
	require.Greater(t, id, int64(0))

	// Verify full document includes content and metadata
	retrieved, err := getDocument(db, id)
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, "note", retrieved.Type)
	assert.Equal(t, "acme-corp", retrieved.Project)
	assert.Equal(t, "procedure", retrieved.Category)
	assert.Equal(t, "release-process", retrieved.Title)
	assert.Equal(t, "1. Tag\n2. Push\n3. Monitor", retrieved.Content)
	assert.Equal(t, map[string]string{"version": "1.0"}, retrieved.Metadata)
	assert.ElementsMatch(t, []string{"release", "ci"}, retrieved.Tags)
	assert.NotEmpty(t, retrieved.CreatedAt)
	assert.NotEmpty(t, retrieved.UpdatedAt)
}

func TestUpsertUpdatesExisting(t *testing.T) {
	db := testDB(t)

	doc1 := Document{
		Type:     "note",
		Project:  "acme-corp",
		Category: "guide",
		Title:    "onboarding",
		Content:  "Welcome to acme-corp",
		Tags:     []string{"team", "new-hire"},
	}

	id1, err := upsertDocument(db, doc1)
	require.NoError(t, err)

	retrieved1, err := getDocument(db, id1)
	require.NoError(t, err)
	firstUpdatedAt := retrieved1.UpdatedAt

	// Upsert same document with different content
	doc2 := Document{
		Type:     "note",
		Project:  "acme-corp",
		Category: "guide",
		Title:    "onboarding",
		Content:  "Welcome! Updated guide.",
		Tags:     []string{"team"},
	}

	id2, err := upsertDocument(db, doc2)
	require.NoError(t, err)

	// Should be same ID
	assert.Equal(t, id1, id2)

	retrieved2, err := getDocument(db, id1)
	require.NoError(t, err)

	assert.Equal(t, "Welcome! Updated guide.", retrieved2.Content)
	assert.ElementsMatch(t, []string{"team"}, retrieved2.Tags)
	// CreatedAt should not change
	assert.Equal(t, retrieved1.CreatedAt, retrieved2.CreatedAt)
	// UpdatedAt should be updated (or at least not before the original)
	assert.GreaterOrEqual(t, retrieved2.UpdatedAt, firstUpdatedAt)
}

func TestQueryDocumentsByType(t *testing.T) {
	db := testDB(t)

	// Insert documents of different types
	doc1 := Document{Type: "decision", Project: "acme-corp", Title: "Use PostgreSQL"}
	doc2 := Document{Type: "fact", Project: "acme-corp", Title: "DB Host"}
	doc3 := Document{Type: "note", Project: "acme-corp", Title: "Release Notes"}

	_, err := upsertDocument(db, doc1)
	require.NoError(t, err)
	_, err = upsertDocument(db, doc2)
	require.NoError(t, err)
	_, err = upsertDocument(db, doc3)
	require.NoError(t, err)

	// Query by type
	summaries, err := queryDocuments(db, "note", "", "", "", nil, 50)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "note", summaries[0].Type)
	assert.Equal(t, "Release Notes", summaries[0].Title)
}

func TestQueryDocumentsByProject(t *testing.T) {
	db := testDB(t)

	doc1 := Document{Type: "note", Project: "acme-corp", Title: "Notes 1"}
	doc2 := Document{Type: "note", Project: "example-org", Title: "Notes 2"}

	_, err := upsertDocument(db, doc1)
	require.NoError(t, err)
	_, err = upsertDocument(db, doc2)
	require.NoError(t, err)

	summaries, err := queryDocuments(db, "", "acme-corp", "", "", nil, 50)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "acme-corp", summaries[0].Project)
}

func TestQueryDocumentsByCategory(t *testing.T) {
	db := testDB(t)

	doc1 := Document{Type: "fact", Project: "acme-corp", Category: "config", Title: "App Name"}
	doc2 := Document{Type: "fact", Project: "acme-corp", Category: "deployment", Title: "Region"}

	_, err := upsertDocument(db, doc1)
	require.NoError(t, err)
	_, err = upsertDocument(db, doc2)
	require.NoError(t, err)

	summaries, err := queryDocuments(db, "", "", "config", "", nil, 50)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "config", summaries[0].Category)
}

func TestQueryDocumentsFTS(t *testing.T) {
	db := testDB(t)

	doc1 := Document{
		Type:    "note",
		Project: "acme-corp",
		Title:   "release-process",
		Content: "Tag and push to trigger goreleaser",
	}
	doc2 := Document{
		Type:    "note",
		Project: "acme-corp",
		Title:   "deployment",
		Content: "Deploy to production",
	}

	_, err := upsertDocument(db, doc1)
	require.NoError(t, err)
	_, err = upsertDocument(db, doc2)
	require.NoError(t, err)

	summaries, err := queryDocuments(db, "", "", "", "goreleaser", nil, 50)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "release-process", summaries[0].Title)
}

func TestQueryDocumentsTagFilter(t *testing.T) {
	db := testDB(t)

	doc1 := Document{Type: "note", Project: "acme-corp", Title: "Release", Tags: []string{"ci", "release"}}
	doc2 := Document{Type: "note", Project: "acme-corp", Title: "Deploy", Tags: []string{"ci"}}
	doc3 := Document{Type: "note", Project: "acme-corp", Title: "Monitor", Tags: []string{"ops"}}

	_, err := upsertDocument(db, doc1)
	require.NoError(t, err)
	_, err = upsertDocument(db, doc2)
	require.NoError(t, err)
	_, err = upsertDocument(db, doc3)
	require.NoError(t, err)

	// Query for documents with both ci AND release tags
	summaries, err := queryDocuments(db, "", "", "", "", []string{"ci", "release"}, 50)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "Release", summaries[0].Title)
}

func TestQueryDocumentsReturnsNoContent(t *testing.T) {
	db := testDB(t)

	doc := Document{
		Type:     "note",
		Project:  "acme-corp",
		Title:    "test",
		Content:  "This is the content that should not be in summaries",
		Metadata: map[string]string{"key": "value"},
	}

	_, err := upsertDocument(db, doc)
	require.NoError(t, err)

	summaries, err := queryDocuments(db, "", "", "", "", nil, 50)
	require.NoError(t, err)
	require.Len(t, summaries, 1)

	// Verify summary does not include content or metadata
	assert.Equal(t, "test", summaries[0].Title)
	// DocumentSummary type does not have Content or Metadata fields, so just verify it's a summary
	assert.Equal(t, int64(1), summaries[0].ID)
}

func TestDeleteDocument(t *testing.T) {
	db := testDB(t)

	doc := Document{Type: "note", Project: "acme-corp", Title: "to-delete", Content: "content"}
	id, err := upsertDocument(db, doc)
	require.NoError(t, err)

	// Verify it exists
	retrieved, err := getDocument(db, id)
	require.NoError(t, err)
	assert.NotNil(t, retrieved)

	// Delete it
	err = deleteDocument(db, id)
	require.NoError(t, err)

	// Verify it's gone
	retrieved, err = getDocument(db, id)
	require.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestGetDocumentNotFound(t *testing.T) {
	db := testDB(t)

	doc, err := getDocument(db, 999)
	require.NoError(t, err)
	assert.Nil(t, doc)
}

func TestSearchDocuments(t *testing.T) {
	db := testDB(t)

	doc1 := Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Database Choice",
		Content: "We chose PostgreSQL for its ACID guarantees",
	}
	doc2 := Document{
		Type:    "fact",
		Project: "acme-corp",
		Title:   "DB Host",
		Content: "prod-db.example.com",
	}
	doc3 := Document{
		Type:    "note",
		Project: "example-org",
		Title:   "API Design",
		Content: "REST endpoints for service discovery",
	}

	_, err := upsertDocument(db, doc1)
	require.NoError(t, err)
	_, err = upsertDocument(db, doc2)
	require.NoError(t, err)
	_, err = upsertDocument(db, doc3)
	require.NoError(t, err)

	summaries, err := searchDocuments(db, "PostgreSQL", "", "", 50)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "Database Choice", summaries[0].Title)
}

func TestSearchDocumentsWithTypeFilter(t *testing.T) {
	db := testDB(t)

	doc1 := Document{Type: "decision", Project: "acme-corp", Title: "DB", Content: "PostgreSQL"}
	doc2 := Document{Type: "note", Project: "acme-corp", Title: "Note", Content: "PostgreSQL info"}

	_, err := upsertDocument(db, doc1)
	require.NoError(t, err)
	_, err = upsertDocument(db, doc2)
	require.NoError(t, err)

	summaries, err := searchDocuments(db, "PostgreSQL", "decision", "", 50)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "decision", summaries[0].Type)
}

func TestSearchDocumentsWithProjectFilter(t *testing.T) {
	db := testDB(t)

	doc1 := Document{Type: "note", Project: "acme-corp", Title: "Note 1", Content: "PostgreSQL"}
	doc2 := Document{Type: "note", Project: "other-proj", Title: "Note 2", Content: "PostgreSQL"}

	_, err := upsertDocument(db, doc1)
	require.NoError(t, err)
	_, err = upsertDocument(db, doc2)
	require.NoError(t, err)

	summaries, err := searchDocuments(db, "PostgreSQL", "", "acme-corp", 50)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "acme-corp", summaries[0].Project)
}

func TestClampLimit(t *testing.T) {
	tests := []struct {
		name       string
		limit      int
		defaultVal int
		maxVal     int
		expected   int
	}{
		{"zero returns default", 0, 50, 500, 50},
		{"negative returns default", -1, 50, 500, 50},
		{"within range", 25, 50, 500, 25},
		{"at max", 500, 50, 500, 500},
		{"exceeds max", 600, 50, 500, 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := clampLimit(tt.limit, tt.defaultVal, tt.maxVal)
			assert.Equal(t, tt.expected, result)
		})
	}
}
