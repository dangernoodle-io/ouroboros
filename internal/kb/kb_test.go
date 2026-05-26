package kb_test

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/kb"
	"dangernoodle.io/ouroboros/internal/store"
)

func testDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)

	err = store.ApplySchema(db)
	require.NoError(t, err)

	return db
}

func TestExportMarkdownEmpty(t *testing.T) {
	testdb := testDB(t)

	markdown, err := kb.ExportMarkdown(testdb, []string{}, "")
	require.NoError(t, err)

	// Verify header is present
	assert.Contains(t, markdown, "# Knowledge Base Export")
	assert.Contains(t, markdown, "All Projects")
	assert.Contains(t, markdown, "_No documents found._")
}

func TestExportMarkdownWithData(t *testing.T) {
	testdb := testDB(t)

	// Insert test data
	_, err := store.UpsertDocument(testdb, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Use PostgreSQL",
		Content: "ACID compliance",
		Tags:    []string{"database", "architecture"},
	})
	require.NoError(t, err)

	_, err = store.UpsertDocument(testdb, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Containerize services",
		Content: "Deploy with Docker",
		Tags:    []string{"infrastructure"},
	})
	require.NoError(t, err)

	_, err = store.UpsertDocument(testdb, store.Document{
		Type:     "fact",
		Project:  "acme-corp",
		Category: "config",
		Title:    "db-host",
		Content:  "prod.acme-corp.example.com",
	})
	require.NoError(t, err)

	// Export
	markdown, err := kb.ExportMarkdown(testdb, []string{"acme-corp"}, "")
	require.NoError(t, err)

	// Verify content sections
	assert.Contains(t, markdown, "Use PostgreSQL")
	assert.Contains(t, markdown, "ACID compliance")
	assert.Contains(t, markdown, "database, architecture")
	assert.Contains(t, markdown, "Containerize services")
	assert.Contains(t, markdown, "infrastructure")
	assert.Contains(t, markdown, "db-host")
	assert.Contains(t, markdown, "prod.acme-corp.example.com")
	assert.Contains(t, markdown, "Project: acme-corp")
}

func TestExportMarkdownProjectFilter(t *testing.T) {
	testdb := testDB(t)

	// Insert data for two projects
	_, err := store.UpsertDocument(testdb, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Decision 1",
	})
	require.NoError(t, err)

	_, err = store.UpsertDocument(testdb, store.Document{
		Type:    "decision",
		Project: "other-proj",
		Title:   "Decision 2",
	})
	require.NoError(t, err)

	// Export for specific project
	markdown, err := kb.ExportMarkdown(testdb, []string{"acme-corp"}, "")
	require.NoError(t, err)

	// Verify only acme-corp decision is present
	assert.Contains(t, markdown, "Decision 1")
	assert.NotContains(t, markdown, "Decision 2")
	assert.Contains(t, markdown, "Project: acme-corp")
}

func TestExportMarkdownTypeFilter(t *testing.T) {
	testdb := testDB(t)

	// Insert different types
	_, err := store.UpsertDocument(testdb, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Decision 1",
	})
	require.NoError(t, err)

	_, err = store.UpsertDocument(testdb, store.Document{
		Type:    "fact",
		Project: "acme-corp",
		Title:   "Fact 1",
	})
	require.NoError(t, err)

	// Export only decisions
	markdown, err := kb.ExportMarkdown(testdb, []string{}, "decision")
	require.NoError(t, err)

	assert.Contains(t, markdown, "Decision 1")
	assert.NotContains(t, markdown, "Fact 1")
	assert.Contains(t, markdown, "Type: decision")
}

func TestImportJSON(t *testing.T) {
	testdb := testDB(t)

	// Create import payload
	payload := kb.ImportData{
		Documents: []kb.ImportDocument{
			{
				Type:    "decision",
				Project: "acme-corp",
				Title:   "Use PostgreSQL",
				Content: "ACID compliance",
				Tags:    []string{"database", "architecture"},
			},
			{
				Type:     "fact",
				Project:  "acme-corp",
				Category: "config",
				Title:    "db-host",
				Content:  "prod.acme-corp.example.com",
			},
		},
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	// Import
	err = kb.ImportJSON(testdb, "", data)
	require.NoError(t, err)

	// Verify decision imported
	decisions, err := store.QueryDocuments(testdb, "decision", []string{"acme-corp"}, "", "", nil, 50)
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	assert.Equal(t, "Use PostgreSQL", decisions[0].Title)

	// Verify fact imported
	facts, err := store.QueryDocuments(testdb, "fact", []string{"acme-corp"}, "", "", nil, 50)
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, "db-host", facts[0].Title)
}

func TestImportJSONDefaultProject(t *testing.T) {
	testdb := testDB(t)

	// Create import payload with items missing project field
	payload := kb.ImportData{
		Documents: []kb.ImportDocument{
			{
				Type:  "decision",
				Title: "Decision 1",
			},
			{
				Type:     "fact",
				Category: "config",
				Title:    "setting",
				Content:  "value",
			},
		},
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	// Import with default project
	err = kb.ImportJSON(testdb, "acme-corp", data)
	require.NoError(t, err)

	// Verify decision used default project
	decisions, err := store.QueryDocuments(testdb, "decision", []string{"acme-corp"}, "", "", nil, 50)
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	assert.Equal(t, "acme-corp", decisions[0].Project)

	// Verify fact used default project
	facts, err := store.QueryDocuments(testdb, "fact", []string{"acme-corp"}, "", "", nil, 50)
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, "acme-corp", facts[0].Project)
}

func TestImportJSONMissingProject(t *testing.T) {
	testdb := testDB(t)

	// Create payload with missing project and no default
	payload := kb.ImportData{
		Documents: []kb.ImportDocument{
			{
				Type:  "decision",
				Title: "Decision without project",
			},
		},
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	// Import should fail
	err = kb.ImportJSON(testdb, "", data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing project")
}

func TestImportDataAutoDetectJSON(t *testing.T) {
	testdb := testDB(t)

	// Create JSON string
	payload := kb.ImportData{
		Documents: []kb.ImportDocument{
			{
				Type:    "decision",
				Project: "acme-corp",
				Title:   "Decision 1",
			},
		},
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)

	// Import with auto-detection
	err = kb.Import(testdb, "", string(data))
	require.NoError(t, err)

	// Verify imported
	docs, err := store.QueryDocuments(testdb, "decision", []string{"acme-corp"}, "", "", nil, 50)
	require.NoError(t, err)
	require.Len(t, docs, 1)
}

func TestImportDataUnsupportedFormat(t *testing.T) {
	testdb := testDB(t)

	// Try to import markdown
	markdown := `# Decisions

## Decision 1
Summary: Test decision`

	err := kb.Import(testdb, "", markdown)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
	assert.Contains(t, err.Error(), "JSON")
}

func TestImportDataEmpty(t *testing.T) {
	testdb := testDB(t)

	err := kb.Import(testdb, "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestExportImportRoundTrip(t *testing.T) {
	testdb1 := testDB(t)
	testdb2 := testDB(t)

	// Insert data into db1
	_, err := store.UpsertDocument(testdb1, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Use PostgreSQL",
		Content: "ACID compliance",
		Tags:    []string{"database"},
	})
	require.NoError(t, err)

	_, err = store.UpsertDocument(testdb1, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Docker deployment",
		Content: "Container orchestration",
		Tags:    []string{"infrastructure"},
	})
	require.NoError(t, err)

	_, err = store.UpsertDocument(testdb1, store.Document{
		Type:     "fact",
		Project:  "acme-corp",
		Category: "config",
		Title:    "db-host",
		Content:  "prod.acme-corp.example.com",
	})
	require.NoError(t, err)

	// Export markdown from db1 (verify it works)
	markdown, err := kb.ExportMarkdown(testdb1, []string{"acme-corp"}, "")
	require.NoError(t, err)
	assert.NotEmpty(t, markdown)
	assert.Contains(t, markdown, "Use PostgreSQL")
	assert.Contains(t, markdown, "Docker deployment")

	// Manually create JSON with same data for import into db2
	importPayload := kb.ImportData{
		Documents: []kb.ImportDocument{
			{
				Type:    "decision",
				Project: "acme-corp",
				Title:   "Use PostgreSQL",
				Content: "ACID compliance",
				Tags:    []string{"database"},
			},
			{
				Type:    "decision",
				Project: "acme-corp",
				Title:   "Docker deployment",
				Content: "Container orchestration",
				Tags:    []string{"infrastructure"},
			},
			{
				Type:     "fact",
				Project:  "acme-corp",
				Category: "config",
				Title:    "db-host",
				Content:  "prod.acme-corp.example.com",
			},
		},
	}

	data, err := json.Marshal(importPayload)
	require.NoError(t, err)

	// Import into db2
	err = kb.ImportJSON(testdb2, "", data)
	require.NoError(t, err)

	// Verify counts match between databases
	docs1, err := store.QueryDocuments(testdb1, "", []string{"acme-corp"}, "", "", nil, 500)
	require.NoError(t, err)

	docs2, err := store.QueryDocuments(testdb2, "", []string{"acme-corp"}, "", "", nil, 500)
	require.NoError(t, err)

	assert.Equal(t, len(docs1), len(docs2))
	assert.Equal(t, 3, len(docs2))
}

func TestImportJSONWhitespace(t *testing.T) {
	testdb := testDB(t)

	// JSON with extra whitespace
	jsonStr := `
	{
		"documents": [
			{
				"type": "decision",
				"project": "acme-corp",
				"title": "Test Decision"
			}
		]
	}`

	err := kb.Import(testdb, "", jsonStr)
	require.NoError(t, err)

	docs, err := store.QueryDocuments(testdb, "decision", []string{"acme-corp"}, "", "", nil, 50)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "Test Decision", docs[0].Title)
}

func TestWriteBatch_HappyPath_ReturnsAllResults(t *testing.T) {
	db := testDB(t)

	entries := []kb.Entry{
		{Type: "decision", Project: "acme-corp", Title: "batch-entry-1", Content: "first decision"},
		{Type: "fact", Project: "acme-corp", Title: "batch-entry-2", Content: "second fact"},
		{Type: "note", Project: "acme-corp", Title: "batch-entry-3", Content: "third note"},
	}

	results, err := kb.WriteBatch(db, entries, "")
	require.NoError(t, err)
	require.Len(t, results, 3)

	for i, r := range results {
		assert.Greater(t, r.ID, int64(0))
		assert.Equal(t, "created", r.Action)
		assert.Equal(t, entries[i].Title, r.Title)
	}
}

func TestWriteBatch_Empty_NoResults(t *testing.T) {
	db := testDB(t)

	results, err := kb.WriteBatch(db, []kb.Entry{}, "")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestWriteBatch_ValidationFailureAbortsAll(t *testing.T) {
	db := testDB(t)

	entries := []kb.Entry{
		{Type: "decision", Project: "acme-corp", Title: "valid-entry", Content: "ok"},
		// Missing type — should fail validation
		{Type: "", Project: "acme-corp", Title: "invalid-entry", Content: "bad"},
	}

	results, err := kb.WriteBatch(db, entries, "")
	require.Error(t, err)
	assert.Nil(t, results)

	// Neither entry should be in DB
	docs, qerr := store.QueryDocuments(db, "", []string{"acme-corp"}, "", "", nil, 50)
	require.NoError(t, qerr)
	assert.Empty(t, docs)
}

func TestWriteBatch_RebuildFTSCalledOnce(t *testing.T) {
	db := testDB(t)

	entries := []kb.Entry{
		{Type: "decision", Project: "acme-corp", Title: "fts-entry-alpha", Content: "distincttoken001"},
		{Type: "fact", Project: "acme-corp", Title: "fts-entry-beta", Content: "distincttoken002"},
		{Type: "note", Project: "acme-corp", Title: "fts-entry-gamma", Content: "distincttoken003"},
	}

	_, err := kb.WriteBatch(db, entries, "")
	require.NoError(t, err)

	// FTS search for each distinct token must return the correct doc,
	// proving RebuildFTS ran after commit (not mid-tx, where it would fail).
	for _, e := range entries {
		results, serr := store.SearchDocuments(db, e.Content, "", nil, 10)
		require.NoError(t, serr)
		require.Len(t, results, 1, "FTS should find exactly one doc for %q", e.Content)
		assert.Equal(t, e.Title, results[0].Title)
	}
}

func TestImportMultipleProjects(t *testing.T) {
	testdb := testDB(t)

	// Create import payload with multiple projects
	payload := kb.ImportData{
		Documents: []kb.ImportDocument{
			{
				Type:    "decision",
				Project: "acme-corp",
				Title:   "Decision 1",
			},
			{
				Type:    "decision",
				Project: "other-proj",
				Title:   "Decision 2",
			},
			{
				Type:     "fact",
				Project:  "acme-corp",
				Category: "config",
				Title:    "key1",
				Content:  "value1",
			},
			{
				Type:     "fact",
				Project:  "other-proj",
				Category: "config",
				Title:    "key2",
				Content:  "value2",
			},
		},
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	err = kb.ImportJSON(testdb, "", data)
	require.NoError(t, err)

	// Verify both projects have data
	docs1, err := store.QueryDocuments(testdb, "decision", []string{"acme-corp"}, "", "", nil, 50)
	require.NoError(t, err)
	assert.Len(t, docs1, 1)

	docs2, err := store.QueryDocuments(testdb, "decision", []string{"other-proj"}, "", "", nil, 50)
	require.NoError(t, err)
	assert.Len(t, docs2, 1)

	facts1, err := store.QueryDocuments(testdb, "fact", []string{"acme-corp"}, "", "", nil, 50)
	require.NoError(t, err)
	assert.Len(t, facts1, 1)

	facts2, err := store.QueryDocuments(testdb, "fact", []string{"other-proj"}, "", "", nil, 50)
	require.NoError(t, err)
	assert.Len(t, facts2, 1)
}

// TestImportJSON_AtomicRollback verifies that a mid-import failure leaves no rows persisted.
// A SQLite trigger is installed to abort on the second insert, simulating a partial failure.
func TestImportJSON_AtomicRollback(t *testing.T) {
	db := testDB(t)

	// Trigger that aborts any insert when at least one document already exists in the tx.
	_, err := db.Exec(`
		CREATE TRIGGER test_fail_second_insert
		BEFORE INSERT ON documents
		WHEN (SELECT COUNT(*) FROM documents) >= 1
		BEGIN SELECT RAISE(ABORT, 'test: intentional second-insert failure'); END
	`)
	require.NoError(t, err)

	payload := kb.ImportData{
		Documents: []kb.ImportDocument{
			{Type: "decision", Project: "acme-corp", Title: "first-doc", Content: "ok"},
			{Type: "fact", Project: "acme-corp", Title: "second-doc", Content: "triggers abort"},
		},
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)

	err = kb.ImportJSON(db, "", data)
	require.Error(t, err, "import must fail when second insert aborts")

	// Drop the trigger so we can query cleanly.
	_, dropErr := db.Exec("DROP TRIGGER test_fail_second_insert")
	require.NoError(t, dropErr)

	// No rows must have been committed — the transaction must have rolled back.
	docs, qerr := store.QueryDocuments(db, "", []string{"acme-corp"}, "", "", nil, 50)
	require.NoError(t, qerr)
	assert.Empty(t, docs, "no documents should persist after a rolled-back import")
}

// TestImportJSON_ValidationFailureAbortsBeforeTx verifies that pre-tx validation
// (missing title) fails before any rows are written.
func TestImportJSON_ValidationFailureAbortsBeforeTx(t *testing.T) {
	db := testDB(t)

	payload := kb.ImportData{
		Documents: []kb.ImportDocument{
			{Type: "decision", Project: "acme-corp", Title: "valid-doc", Content: "ok"},
			// Missing title — triggers pre-tx validation error.
			{Type: "fact", Project: "acme-corp", Title: "", Content: "no title"},
		},
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)

	err = kb.ImportJSON(db, "", data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing title")

	docs, qerr := store.QueryDocuments(db, "", []string{"acme-corp"}, "", "", nil, 50)
	require.NoError(t, qerr)
	assert.Empty(t, docs)
}

// TestImportJSON_InvalidJSON verifies that malformed JSON returns an unmarshal error.
func TestImportJSON_InvalidJSON(t *testing.T) {
	db := testDB(t)

	err := kb.ImportJSON(db, "", []byte(`{not valid json`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal JSON")
}

// TestImportJSON_MissingType verifies that a document with empty type fails validation.
func TestImportJSON_MissingType(t *testing.T) {
	db := testDB(t)

	payload := kb.ImportData{
		Documents: []kb.ImportDocument{
			{Type: "", Project: "acme-corp", Title: "no-type-doc", Content: "content"},
		},
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)

	err = kb.ImportJSON(db, "", data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing type")

	docs, qerr := store.QueryDocuments(db, "", []string{"acme-corp"}, "", "", nil, 50)
	require.NoError(t, qerr)
	assert.Empty(t, docs)
}
