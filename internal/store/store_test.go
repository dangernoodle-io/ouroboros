package store_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/store"
)

// testDB creates an in-memory database for testing.
func testDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	require.NoError(t, store.ApplySchema(db))
	t.Cleanup(func() { db.Close() })
	return db
}

func TestUpsertAndGetDocument(t *testing.T) {
	db := testDB(t)

	doc := store.Document{
		Type:     "note",
		Project:  "acme-corp",
		Category: "procedure",
		Title:    "release-process",
		Content:  "1. Tag\n2. Push\n3. Monitor",
		Metadata: map[string]string{"version": "1.0"},
		Tags:     []string{"release", "ci"},
	}

	result, err := store.UpsertDocument(db, doc)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Greater(t, result.ID, int64(0))
	assert.Equal(t, "created", result.Action)

	// Verify full document includes content and metadata
	retrieved, err := store.GetDocument(db, result.ID)
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

	doc1 := store.Document{
		Type:     "note",
		Project:  "acme-corp",
		Category: "guide",
		Title:    "onboarding",
		Content:  "Welcome to acme-corp",
		Tags:     []string{"team", "new-hire"},
	}

	result1, err := store.UpsertDocument(db, doc1)
	require.NoError(t, err)
	assert.Equal(t, "created", result1.Action)
	id1 := result1.ID

	retrieved1, err := store.GetDocument(db, id1)
	require.NoError(t, err)
	firstUpdatedAt := retrieved1.UpdatedAt

	// Upsert same document with different content
	doc2 := store.Document{
		Type:     "note",
		Project:  "acme-corp",
		Category: "guide",
		Title:    "onboarding",
		Content:  "Welcome! Updated guide.",
		Tags:     []string{"team"},
	}

	result2, err := store.UpsertDocument(db, doc2)
	require.NoError(t, err)
	assert.Equal(t, "updated", result2.Action)

	// Should be same ID
	assert.Equal(t, id1, result2.ID)

	retrieved2, err := store.GetDocument(db, id1)
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
	doc1 := store.Document{Type: "decision", Project: "acme-corp", Title: "Use PostgreSQL"}
	doc2 := store.Document{Type: "fact", Project: "acme-corp", Title: "DB Host"}
	doc3 := store.Document{Type: "note", Project: "acme-corp", Title: "Release Notes"}

	_, err := store.UpsertDocument(db, doc1)
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, doc2)
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, doc3)
	require.NoError(t, err)

	// Query by type
	summaries, err := store.QueryDocuments(db, "note", "", "", "", nil, 50)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "note", summaries[0].Type)
	assert.Equal(t, "Release Notes", summaries[0].Title)
}

func TestQueryDocumentsByProject(t *testing.T) {
	db := testDB(t)

	doc1 := store.Document{Type: "note", Project: "acme-corp", Title: "Notes 1"}
	doc2 := store.Document{Type: "note", Project: "example-org", Title: "Notes 2"}

	_, err := store.UpsertDocument(db, doc1)
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, doc2)
	require.NoError(t, err)

	summaries, err := store.QueryDocuments(db, "", "acme-corp", "", "", nil, 50)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "acme-corp", summaries[0].Project)
}

func TestQueryDocumentsByCategory(t *testing.T) {
	db := testDB(t)

	doc1 := store.Document{Type: "fact", Project: "acme-corp", Category: "config", Title: "App Name"}
	doc2 := store.Document{Type: "fact", Project: "acme-corp", Category: "deployment", Title: "Region"}

	_, err := store.UpsertDocument(db, doc1)
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, doc2)
	require.NoError(t, err)

	summaries, err := store.QueryDocuments(db, "", "", "config", "", nil, 50)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "config", summaries[0].Category)
}

func TestQueryDocumentsFTS(t *testing.T) {
	db := testDB(t)

	doc1 := store.Document{
		Type:    "note",
		Project: "acme-corp",
		Title:   "release-process",
		Content: "Tag and push to trigger goreleaser",
	}
	doc2 := store.Document{
		Type:    "note",
		Project: "acme-corp",
		Title:   "deployment",
		Content: "Deploy to production",
	}

	_, err := store.UpsertDocument(db, doc1)
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, doc2)
	require.NoError(t, err)

	summaries, err := store.QueryDocuments(db, "", "", "", "goreleaser", nil, 50)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "release-process", summaries[0].Title)
}

func TestQueryDocumentsTagFilter(t *testing.T) {
	db := testDB(t)

	doc1 := store.Document{Type: "note", Project: "acme-corp", Title: "Release", Tags: []string{"ci", "release"}}
	doc2 := store.Document{Type: "note", Project: "acme-corp", Title: "Deploy", Tags: []string{"ci"}}
	doc3 := store.Document{Type: "note", Project: "acme-corp", Title: "Monitor", Tags: []string{"ops"}}

	_, err := store.UpsertDocument(db, doc1)
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, doc2)
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, doc3)
	require.NoError(t, err)

	// Query for documents with both ci AND release tags
	summaries, err := store.QueryDocuments(db, "", "", "", "", []string{"ci", "release"}, 50)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "Release", summaries[0].Title)
}

func TestQueryDocumentsReturnsNoContent(t *testing.T) {
	db := testDB(t)

	doc := store.Document{
		Type:     "note",
		Project:  "acme-corp",
		Title:    "test",
		Content:  "This is the content that should not be in summaries",
		Metadata: map[string]string{"key": "value"},
	}

	_, err := store.UpsertDocument(db, doc)
	require.NoError(t, err)

	summaries, err := store.QueryDocuments(db, "", "", "", "", nil, 50)
	require.NoError(t, err)
	require.Len(t, summaries, 1)

	// Verify summary does not include content or metadata
	assert.Equal(t, "test", summaries[0].Title)
	// DocumentSummary type does not have Content or Metadata fields, so just verify it's a summary
	assert.Equal(t, int64(1), summaries[0].ID)
}

func TestDeleteDocument(t *testing.T) {
	db := testDB(t)

	doc := store.Document{Type: "note", Project: "acme-corp", Title: "to-delete", Content: "content"}
	result, err := store.UpsertDocument(db, doc)
	require.NoError(t, err)
	id := result.ID

	// Verify it exists
	retrieved, err := store.GetDocument(db, id)
	require.NoError(t, err)
	assert.NotNil(t, retrieved)

	// Delete it
	err = store.DeleteDocument(db, id)
	require.NoError(t, err)

	// Verify it's gone
	retrieved, err = store.GetDocument(db, id)
	require.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestGetDocumentNotFound(t *testing.T) {
	db := testDB(t)

	doc, err := store.GetDocument(db, 999)
	require.NoError(t, err)
	assert.Nil(t, doc)
}

func TestSearchDocuments(t *testing.T) {
	db := testDB(t)

	doc1 := store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Database Choice",
		Content: "We chose PostgreSQL for its ACID guarantees",
	}
	doc2 := store.Document{
		Type:    "fact",
		Project: "acme-corp",
		Title:   "DB Host",
		Content: "prod-db.example.com",
	}
	doc3 := store.Document{
		Type:    "note",
		Project: "example-org",
		Title:   "API Design",
		Content: "REST endpoints for service discovery",
	}

	_, err := store.UpsertDocument(db, doc1)
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, doc2)
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, doc3)
	require.NoError(t, err)

	summaries, err := store.SearchDocuments(db, "PostgreSQL", "", "", 50)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "Database Choice", summaries[0].Title)
}

func TestSearchDocumentsWithTypeFilter(t *testing.T) {
	db := testDB(t)

	doc1 := store.Document{Type: "decision", Project: "acme-corp", Title: "DB", Content: "PostgreSQL"}
	doc2 := store.Document{Type: "note", Project: "acme-corp", Title: "Note", Content: "PostgreSQL info"}

	_, err := store.UpsertDocument(db, doc1)
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, doc2)
	require.NoError(t, err)

	summaries, err := store.SearchDocuments(db, "PostgreSQL", "decision", "", 50)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "decision", summaries[0].Type)
}

func TestSearchDocumentsWithProjectFilter(t *testing.T) {
	db := testDB(t)

	doc1 := store.Document{Type: "note", Project: "acme-corp", Title: "Note 1", Content: "PostgreSQL"}
	doc2 := store.Document{Type: "note", Project: "other-proj", Title: "Note 2", Content: "PostgreSQL"}

	_, err := store.UpsertDocument(db, doc1)
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, doc2)
	require.NoError(t, err)

	summaries, err := store.SearchDocuments(db, "PostgreSQL", "", "acme-corp", 50)
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
			result := store.ClampLimit(tt.limit, tt.defaultVal, tt.maxVal)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTokenizeQuery(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected []string
	}{
		{
			name:     "normal words",
			query:    "postgresql database performance",
			expected: []string{"postgresql", "database", "performance"},
		},
		{
			name:     "mixed case normalized to lowercase",
			query:    "PostgreSQL Database PERFORMANCE",
			expected: []string{"postgresql", "database", "performance"},
		},
		{
			name:     "stop words filtered",
			query:    "what is the best database for performance",
			expected: []string{"best", "database", "performance"},
		},
		{
			name:     "punctuation stripped",
			query:    "postgresql, database! (performance)",
			expected: []string{"postgresql", "database", "performance"},
		},
		{
			name:     "all stop words returns empty",
			query:    "the is an a are you they we",
			expected: []string{},
		},
		{
			name:     "empty query",
			query:    "",
			expected: []string{},
		},
		{
			name:     "only whitespace",
			query:    "   ",
			expected: []string{},
		},
		{
			name:     "mixed punctuation and stop words",
			query:    "how do we deploy the service to production?",
			expected: []string{"deploy", "service", "production"},
		},
		{
			name:     "hyphenated words",
			query:    "release-process ci-cd",
			expected: []string{"release-process", "ci-cd"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := store.TokenizeQuery(tt.query)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestApplySchemaCreatesAllTables(t *testing.T) {
	db := testDB(t)

	// Check that documents table exists
	var result string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='documents'").Scan(&result)
	require.NoError(t, err)
	assert.Equal(t, "documents", result)

	// Check that documents_fts table exists
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='documents_fts'").Scan(&result)
	require.NoError(t, err)
	assert.Equal(t, "documents_fts", result)

	// Check that config table exists
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='config'").Scan(&result)
	require.NoError(t, err)
	assert.Equal(t, "config", result)

	// Check that projects table exists
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='projects'").Scan(&result)
	require.NoError(t, err)
	assert.Equal(t, "projects", result)

	// Check that items table exists
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='items'").Scan(&result)
	require.NoError(t, err)
	assert.Equal(t, "items", result)

	// Check that plans table exists
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='plans'").Scan(&result)
	require.NoError(t, err)
	assert.Equal(t, "plans", result)

	// Check that schema_migrations table exists
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='schema_migrations'").Scan(&result)
	require.NoError(t, err)
	assert.Equal(t, "schema_migrations", result)
}

func TestApplySchemaIdempotent(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Apply schema first time
	require.NoError(t, store.ApplySchema(db))

	// Apply schema second time - should not error
	require.NoError(t, store.ApplySchema(db))
}

func TestMigrationVersionTracking(t *testing.T) {
	db := testDB(t)

	// Query the schema_migrations table to verify versions were recorded
	rows, err := db.Query("SELECT version FROM schema_migrations ORDER BY version")
	require.NoError(t, err)
	defer rows.Close()

	var versions []int
	for rows.Next() {
		var v int
		require.NoError(t, rows.Scan(&v))
		versions = append(versions, v)
	}

	// Should have recorded migrations 1, 2, 3, and 4
	assert.Equal(t, []int{1, 2, 3, 4}, versions)

	// Verify applied_at is set (not NULL)
	var appliedAt string
	err = db.QueryRow("SELECT applied_at FROM schema_migrations WHERE version=1").Scan(&appliedAt)
	require.NoError(t, err)
	assert.NotEmpty(t, appliedAt)
}

func TestProjectIdColumnExists(t *testing.T) {
	db := testDB(t)

	// Insert a document
	doc := store.Document{
		Type:    "note",
		Project: "acme-corp",
		Title:   "test-doc",
		Content: "test content",
	}
	_, err := store.UpsertDocument(db, doc)
	require.NoError(t, err)

	// Verify project_id column exists and is NULL by default
	var projectID *int64
	err = db.QueryRow("SELECT project_id FROM documents WHERE title='test-doc'").Scan(&projectID)
	require.NoError(t, err)
	assert.Nil(t, projectID)

	// Verify schema for documents includes project_id column
	rows, err := db.Query("PRAGMA table_info(documents)")
	require.NoError(t, err)
	defer rows.Close()

	var columnNames []string
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notnull int
		var dfltValue *string
		var pk int
		require.NoError(t, rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk))
		columnNames = append(columnNames, name)
	}

	assert.Contains(t, columnNames, "project_id")
}

func TestNotesColumnExists(t *testing.T) {
	db := testDB(t)

	// Insert document with notes
	doc := store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Use PostgreSQL",
		Content: "Superior performance",
		Notes:   "Chosen for ACID guarantees and advanced features",
	}
	result, err := store.UpsertDocument(db, doc)
	require.NoError(t, err)

	// Retrieve and verify notes are persisted
	retrieved, err := store.GetDocument(db, result.ID)
	require.NoError(t, err)
	assert.Equal(t, "Chosen for ACID guarantees and advanced features", retrieved.Notes)

	// Verify notes column exists in schema
	rows, err := db.Query("PRAGMA table_info(documents)")
	require.NoError(t, err)
	defer rows.Close()

	var columnNames []string
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notnull int
		var dfltValue *string
		var pk int
		require.NoError(t, rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk))
		columnNames = append(columnNames, name)
	}

	assert.Contains(t, columnNames, "notes")
}

func TestKeywordSearch(t *testing.T) {
	db := testDB(t)

	// Insert test documents
	doc1 := store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Database Choice",
		Content: "We chose PostgreSQL for ACID guarantees",
		Tags:    []string{"database", "infrastructure"},
	}
	doc2 := store.Document{
		Type:    "fact",
		Project: "acme-corp",
		Title:   "DB Host",
		Content: "prod-db.example.com postgresql instance",
		Tags:    []string{"database", "production"},
	}
	doc3 := store.Document{
		Type:    "note",
		Project: "example-org",
		Title:   "API Design",
		Content: "REST endpoints for service discovery",
		Tags:    []string{"api"},
	}

	_, err := store.UpsertDocument(db, doc1)
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, doc2)
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, doc3)
	require.NoError(t, err)

	t.Run("basic keyword search", func(t *testing.T) {
		summaries, err := store.KeywordSearch(db, "postgresql", "", 50)
		require.NoError(t, err)
		require.Len(t, summaries, 2)
		// Should match both doc1 and doc2
		titles := []string{summaries[0].Title, summaries[1].Title}
		assert.Contains(t, titles, "Database Choice")
		assert.Contains(t, titles, "DB Host")
	})

	t.Run("keyword search with project filter", func(t *testing.T) {
		summaries, err := store.KeywordSearch(db, "postgresql", "acme-corp", 50)
		require.NoError(t, err)
		require.Len(t, summaries, 2)
		for _, s := range summaries {
			assert.Equal(t, "acme-corp", s.Project)
		}
	})

	t.Run("keyword search OR matching", func(t *testing.T) {
		summaries, err := store.KeywordSearch(db, "postgresql acid", "", 50)
		require.NoError(t, err)
		// Should match doc1 and doc2 (both have postgresql), and doc1 (has acid)
		require.Len(t, summaries, 2)
	})

	t.Run("keyword search with stop words filtered", func(t *testing.T) {
		// Query: "the best database" -> stops words removed -> "best database"
		// Only "database" remains as "best" is not in our docs
		summaries, err := store.KeywordSearch(db, "the best database", "", 50)
		require.NoError(t, err)
		// Should find doc1 and doc2 which contain "database"
		require.Greater(t, len(summaries), 0)
	})

	t.Run("keyword search no matches", func(t *testing.T) {
		summaries, err := store.KeywordSearch(db, "kubernetes", "", 50)
		require.NoError(t, err)
		require.Len(t, summaries, 0)
	})

	t.Run("keyword search all stop words", func(t *testing.T) {
		summaries, err := store.KeywordSearch(db, "the is a an are", "", 50)
		require.NoError(t, err)
		require.Nil(t, summaries)
	})

	t.Run("keyword search empty query", func(t *testing.T) {
		summaries, err := store.KeywordSearch(db, "", "", 50)
		require.NoError(t, err)
		require.Nil(t, summaries)
	})

	t.Run("keyword search respects limit", func(t *testing.T) {
		summaries, err := store.KeywordSearch(db, "example", "", 1)
		require.NoError(t, err)
		require.Len(t, summaries, 1)
	})
}

func TestSearchDocumentsWildcardFallback(t *testing.T) {
	db := testDB(t)

	// Insert a test document
	doc := store.Document{
		Type:    "note",
		Project: "acme-corp",
		Title:   "test-document",
		Content: "This is a test document with searchable content",
	}

	_, err := store.UpsertDocument(db, doc)
	require.NoError(t, err)

	// Test wildcard query falls back to list mode
	summaries, err := store.SearchDocuments(db, "*", "", "", 50)
	require.NoError(t, err)
	require.NotNil(t, summaries)
	require.Len(t, summaries, 1)
	assert.Equal(t, "test-document", summaries[0].Title)
}

func TestSearchDocumentsPunctuationOnlyFallback(t *testing.T) {
	db := testDB(t)

	// Insert a test document
	doc := store.Document{
		Type:    "note",
		Project: "example-org",
		Title:   "another-document",
		Content: "Content for fallback test",
	}

	_, err := store.UpsertDocument(db, doc)
	require.NoError(t, err)

	// Test punctuation-only query falls back to list mode
	summaries, err := store.SearchDocuments(db, "!!!", "", "", 50)
	require.NoError(t, err)
	require.NotNil(t, summaries)
	require.Len(t, summaries, 1)
	assert.Equal(t, "another-document", summaries[0].Title)
}

func TestSearchDocumentsEmptyStringReturnsEmpty(t *testing.T) {
	db := testDB(t)

	// Insert a document
	doc := store.Document{
		Type:    "note",
		Project: "acme-corp",
		Title:   "empty-query-test",
		Content: "test content",
	}

	_, err := store.UpsertDocument(db, doc)
	require.NoError(t, err)

	// Empty query should fall back and return results
	summaries, err := store.SearchDocuments(db, "", "", "", 50)
	require.NoError(t, err)
	require.NotNil(t, summaries)
	require.Len(t, summaries, 1)
}

func TestSearchDocumentsValidQueryStillWorks(t *testing.T) {
	db := testDB(t)

	// Insert documents
	doc1 := store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Language Choice",
		Content: "We decided to use Golang for backend services",
	}
	doc2 := store.Document{
		Type:    "note",
		Project: "acme-corp",
		Title:   "Setup Guide",
		Content: "Python is used for data analysis",
	}

	_, err := store.UpsertDocument(db, doc1)
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, doc2)
	require.NoError(t, err)

	// Test valid FTS query still works
	summaries, err := store.SearchDocuments(db, "Golang", "", "", 50)
	require.NoError(t, err)
	require.NotNil(t, summaries)
	require.Len(t, summaries, 1)
	assert.Equal(t, "Language Choice", summaries[0].Title)
}

func TestSearchDocumentsReturnsEmptySliceNotNil(t *testing.T) {
	db := testDB(t)

	// Search in empty database
	summaries, err := store.SearchDocuments(db, "nonexistent", "", "", 50)
	require.NoError(t, err)
	// Verify it's an empty slice, not nil
	require.NotNil(t, summaries)
	require.Len(t, summaries, 0)
}
