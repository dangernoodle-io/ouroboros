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
	decisions, err := store.QueryDocuments(testdb, []string{"decision"}, []string{"acme-corp"}, nil, "", nil, 50)
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	assert.Equal(t, "Use PostgreSQL", decisions[0].Title)

	// Verify fact imported
	facts, err := store.QueryDocuments(testdb, []string{"fact"}, []string{"acme-corp"}, nil, "", nil, 50)
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
	decisions, err := store.QueryDocuments(testdb, []string{"decision"}, []string{"acme-corp"}, nil, "", nil, 50)
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	assert.Equal(t, "acme-corp", decisions[0].Project)

	// Verify fact used default project
	facts, err := store.QueryDocuments(testdb, []string{"fact"}, []string{"acme-corp"}, nil, "", nil, 50)
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
	docs, err := store.QueryDocuments(testdb, []string{"decision"}, []string{"acme-corp"}, nil, "", nil, 50)
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
	docs1, err := store.QueryDocuments(testdb1, nil, []string{"acme-corp"}, nil, "", nil, 500)
	require.NoError(t, err)

	docs2, err := store.QueryDocuments(testdb2, nil, []string{"acme-corp"}, nil, "", nil, 500)
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

	docs, err := store.QueryDocuments(testdb, []string{"decision"}, []string{"acme-corp"}, nil, "", nil, 50)
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
	docs, qerr := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 50)
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
		results, serr := store.SearchDocuments(db, e.Content, nil, nil, nil, 10)
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
	docs1, err := store.QueryDocuments(testdb, []string{"decision"}, []string{"acme-corp"}, nil, "", nil, 50)
	require.NoError(t, err)
	assert.Len(t, docs1, 1)

	docs2, err := store.QueryDocuments(testdb, []string{"decision"}, []string{"other-proj"}, nil, "", nil, 50)
	require.NoError(t, err)
	assert.Len(t, docs2, 1)

	facts1, err := store.QueryDocuments(testdb, []string{"fact"}, []string{"acme-corp"}, nil, "", nil, 50)
	require.NoError(t, err)
	assert.Len(t, facts1, 1)

	facts2, err := store.QueryDocuments(testdb, []string{"fact"}, []string{"other-proj"}, nil, "", nil, 50)
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
	docs, qerr := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 50)
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

	docs, qerr := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 50)
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

	docs, qerr := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 50)
	require.NoError(t, qerr)
	assert.Empty(t, docs)
}

func TestWriteBatchIntraBatchDedup(t *testing.T) {
	db := testDB(t)

	// 3 entries: index 0 and 2 share the same key; index 2 (last-wins) should survive.
	entries := []kb.Entry{
		{Type: "decision", Project: "acme-corp", Category: "arch", Title: "cache-strategy", Content: "first version"},
		{Type: "decision", Project: "acme-corp", Category: "arch", Title: "db-choice", Content: "use postgres"},
		{Type: "decision", Project: "acme-corp", Category: "arch", Title: "cache-strategy", Content: "last version wins"},
	}

	results, err := kb.WriteBatch(db, entries, "")
	require.NoError(t, err)

	// Only 2 distinct keys → 2 results
	require.Len(t, results, 2)

	// Verify only 2 rows stored
	docs, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 50)
	require.NoError(t, err)
	require.Len(t, docs, 2)

	// Find the cache-strategy doc and verify index 2's content won
	var cacheDoc *store.DocumentSummary
	for i := range docs {
		if docs[i].Title == "cache-strategy" {
			cacheDoc = &docs[i]
			break
		}
	}
	require.NotNil(t, cacheDoc, "cache-strategy doc not found")

	full, err := store.GetDocument(db, cacheDoc.ID)
	require.NoError(t, err)
	assert.Equal(t, "last version wins", full.Content)
}

// TestWriteBatch_ConflictDetected verifies that overwriting with different content
// sets Conflict=true and PreviousExcerpt on the affected result.
func TestWriteBatch_ConflictDetected(t *testing.T) {
	db := testDB(t)

	// First write
	_, err := kb.WriteBatch(db, []kb.Entry{
		{Type: "decision", Project: "acme-corp", Title: "stable-entry", Content: "first content"},
	}, "")
	require.NoError(t, err)

	// Second write with different content — should signal conflict
	results, err := kb.WriteBatch(db, []kb.Entry{
		{Type: "decision", Project: "acme-corp", Title: "stable-entry", Content: "second content"},
	}, "")
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.True(t, results[0].Conflict)
	assert.NotEmpty(t, results[0].PreviousExcerpt)
	assert.Contains(t, results[0].PreviousExcerpt, "first content")
}

// TestWriteBatch_NoConflictSameContent verifies no conflict when same content is re-put.
func TestWriteBatch_NoConflictSameContent(t *testing.T) {
	db := testDB(t)

	entry := kb.Entry{Type: "fact", Project: "acme-corp", Title: "idempotent-entry", Content: "stable content"}

	_, err := kb.WriteBatch(db, []kb.Entry{entry}, "")
	require.NoError(t, err)

	results, err := kb.WriteBatch(db, []kb.Entry{entry}, "")
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.False(t, results[0].Conflict)
	assert.Empty(t, results[0].PreviousExcerpt)
}

// strPtr and stringsPtr are small helpers for building EntryUpdate literals.
func strPtr(s string) *string { return &s }

// TestUpdateBatch_RetitleInPlace_NoNewRow is the core bug repro: an
// id-addressed title-only update must retitle the existing row, not create
// a new one under the new natural key.
func TestUpdateBatch_RetitleInPlace_NoNewRow(t *testing.T) {
	db := testDB(t)

	created, err := kb.WriteBatch(db, []kb.Entry{
		{Type: "decision", Project: "acme-corp", Title: "oldtitlexyz", Content: "original content"},
	}, "")
	require.NoError(t, err)
	require.Len(t, created, 1)
	id := created[0].ID

	results, err := kb.UpdateBatch(db, []kb.EntryUpdate{
		{ID: id, Title: strPtr("newtitlexyz")},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "updated", results[0].Action)
	assert.Equal(t, "newtitlexyz", results[0].Title)

	// Exactly one row for this project — no duplicate created.
	docs, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 50)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, id, docs[0].ID)
	assert.Equal(t, "newtitlexyz", docs[0].Title)

	// Content preserved (partial update touched title only).
	full, err := store.GetDocument(db, id)
	require.NoError(t, err)
	assert.Equal(t, "original content", full.Content)

	// FTS finds it under the new title, not the old one.
	byNew, err := store.SearchDocuments(db, "newtitlexyz", nil, nil, nil, 10)
	require.NoError(t, err)
	require.Len(t, byNew, 1)
	byOld, err := store.SearchDocuments(db, "oldtitlexyz", nil, nil, nil, 10)
	require.NoError(t, err)
	assert.Empty(t, byOld)
}

// TestUpdateBatch_PartialUpdate_ContentPreserved verifies an update that
// touches only some fields leaves the rest untouched.
func TestUpdateBatch_PartialUpdate_ContentPreserved(t *testing.T) {
	db := testDB(t)

	created, err := kb.WriteBatch(db, []kb.Entry{
		{Type: "fact", Project: "acme-corp", Category: "config", Title: "db-host", Content: "prod.example.com", Notes: "keep me"},
	}, "")
	require.NoError(t, err)
	id := created[0].ID

	_, err = kb.UpdateBatch(db, []kb.EntryUpdate{
		{ID: id, Content: strPtr("staging.example.com")},
	})
	require.NoError(t, err)

	full, err := store.GetDocument(db, id)
	require.NoError(t, err)
	assert.Equal(t, "staging.example.com", full.Content)
	assert.Equal(t, "db-host", full.Title)
	assert.Equal(t, "keep me", full.Notes)
	assert.Equal(t, "config", full.Category)
}

// TestUpdateBatch_NonexistentID_Errors verifies a clear error, not a silent
// no-op or a new row, when the id doesn't exist.
func TestUpdateBatch_NonexistentID_Errors(t *testing.T) {
	db := testDB(t)

	results, err := kb.UpdateBatch(db, []kb.EntryUpdate{
		{ID: 999999, Title: strPtr("does not matter")},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "999999")
	assert.Nil(t, results)
}

// TestUpdateBatch_ValidationFailure_EmptyTitle verifies a provided-but-empty
// required field is rejected, not silently accepted.
func TestUpdateBatch_ValidationFailure_EmptyTitle(t *testing.T) {
	db := testDB(t)

	created, err := kb.WriteBatch(db, []kb.Entry{
		{Type: "note", Project: "acme-corp", Title: "keep-me", Content: "c"},
	}, "")
	require.NoError(t, err)
	id := created[0].ID

	_, err = kb.UpdateBatch(db, []kb.EntryUpdate{
		{ID: id, Title: strPtr("")},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "title cannot be empty")

	// Original row untouched.
	full, err := store.GetDocument(db, id)
	require.NoError(t, err)
	assert.Equal(t, "keep-me", full.Title)
}

// TestUpdateBatch_AllFields exercises every EntryUpdate field through the
// full UpdateBatch path (validation + store update + PutResult).
func TestUpdateBatch_AllFields(t *testing.T) {
	db := testDB(t)

	created, err := kb.WriteBatch(db, []kb.Entry{
		{Type: "fact", Project: "acme-corp", Title: "multi-field-batch", Content: "v1"},
	}, "")
	require.NoError(t, err)
	id := created[0].ID

	tags := []string{"x", "y"}
	meta := map[string]string{"k": "v"}
	results, err := kb.UpdateBatch(db, []kb.EntryUpdate{
		{
			ID:       id,
			Type:     strPtr("decision"),
			Project:  strPtr("other-proj"),
			Category: strPtr("arch"),
			Title:    strPtr("multi-field-batch-renamed"),
			Content:  strPtr("v2"),
			Notes:    strPtr("notes here"),
			Tags:     &tags,
			Metadata: &meta,
		},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "multi-field-batch-renamed", results[0].Title)

	full, err := store.GetDocument(db, id)
	require.NoError(t, err)
	assert.Equal(t, "decision", full.Type)
	assert.Equal(t, "other-proj", full.Project)
	assert.Equal(t, "arch", full.Category)
	assert.Equal(t, "v2", full.Content)
	assert.Equal(t, "notes here", full.Notes)
	assert.ElementsMatch(t, tags, full.Tags)
	assert.Equal(t, meta, full.Metadata)
}

// TestUpdateBatch_BeginTxError exercises UpdateBatch's db.BeginTx() error
// path (closed DB), mirroring the store package's *_BeginError tests.
func TestUpdateBatch_BeginTxError(t *testing.T) {
	db := testDB(t)
	require.NoError(t, db.Close())

	results, err := kb.UpdateBatch(db, []kb.EntryUpdate{{ID: 1, Title: strPtr("x")}})
	require.Error(t, err)
	assert.Nil(t, results)
}

// TestUpdateBatch_Empty_NoResults mirrors TestWriteBatch_Empty_NoResults.
func TestUpdateBatch_Empty_NoResults(t *testing.T) {
	db := testDB(t)

	results, err := kb.UpdateBatch(db, []kb.EntryUpdate{})
	require.NoError(t, err)
	assert.Empty(t, results)
}

// TestWriteAndUpdateBatch_MixedHappyPath verifies a single call can mix a
// create (id absent) and an update (id present), both committing together.
func TestWriteAndUpdateBatch_MixedHappyPath(t *testing.T) {
	db := testDB(t)

	seeded, err := kb.WriteBatch(db, []kb.Entry{
		{Type: "fact", Project: "acme-corp", Title: "combined-existing", Content: "c1"},
	}, "")
	require.NoError(t, err)
	id := seeded[0].ID

	creates, updates, err := kb.WriteAndUpdateBatch(db,
		[]kb.Entry{{Type: "note", Project: "acme-corp", Title: "combined-new", Content: "c2"}},
		[]kb.EntryUpdate{{ID: id, Title: strPtr("combined-existing-renamed")}},
		"",
	)
	require.NoError(t, err)
	require.Len(t, creates, 1)
	require.Len(t, updates, 1)
	assert.Equal(t, "combined-new", creates[0].Title)
	assert.Equal(t, "combined-existing-renamed", updates[0].Title)

	docs, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 50)
	require.NoError(t, err)
	require.Len(t, docs, 2)
}

// TestWriteAndUpdateBatch_PartialFailure_RollsBackCreate is the core
// atomicity repro: a create alongside a failing update in the same call
// must not persist the create.
func TestWriteAndUpdateBatch_PartialFailure_RollsBackCreate(t *testing.T) {
	db := testDB(t)

	creates, updates, err := kb.WriteAndUpdateBatch(db,
		[]kb.Entry{{Type: "fact", Project: "acme-corp", Title: "should-not-persist", Content: "c1"}},
		[]kb.EntryUpdate{{ID: 999999, Title: strPtr("does not matter")}},
		"",
	)
	require.Error(t, err)
	assert.Nil(t, creates)
	assert.Nil(t, updates)

	docs, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 50)
	require.NoError(t, err)
	assert.Empty(t, docs, "create in the same failed call must not persist")
}

// TestWriteAndUpdateBatch_EmptyBoth verifies no-op on empty entries/updates.
func TestWriteAndUpdateBatch_EmptyBoth(t *testing.T) {
	db := testDB(t)

	creates, updates, err := kb.WriteAndUpdateBatch(db, nil, nil, "")
	require.NoError(t, err)
	assert.Empty(t, creates)
	assert.Empty(t, updates)
}

// TestWriteAndUpdateBatch_CreateValidationFailure_AbortsBeforeTx verifies a
// bad create entry fails validation before any DB interaction, leaving the
// update side untouched too.
func TestWriteAndUpdateBatch_CreateValidationFailure_AbortsBeforeTx(t *testing.T) {
	db := testDB(t)

	seeded, err := kb.WriteBatch(db, []kb.Entry{
		{Type: "fact", Project: "acme-corp", Title: "untouched", Content: "c1"},
	}, "")
	require.NoError(t, err)
	id := seeded[0].ID

	_, _, err = kb.WriteAndUpdateBatch(db,
		[]kb.Entry{{Type: "", Project: "acme-corp", Title: "invalid-entry", Content: "bad"}},
		[]kb.EntryUpdate{{ID: id, Title: strPtr("should-not-apply")}},
		"",
	)
	require.Error(t, err)

	full, err := store.GetDocument(db, id)
	require.NoError(t, err)
	assert.Equal(t, "untouched", full.Title)
}

// TestWriteAndUpdateBatch_UpdateValidationFailure_AbortsBeforeTx verifies a
// bad update entry (e.g. an explicit empty title) fails validateUpdates
// before any DB interaction — a valid create in the same call must not
// persist either, since validation happens before BeginTx.
func TestWriteAndUpdateBatch_UpdateValidationFailure_AbortsBeforeTx(t *testing.T) {
	db := testDB(t)

	seeded, err := kb.WriteBatch(db, []kb.Entry{
		{Type: "fact", Project: "acme-corp", Title: "untouched-by-update", Content: "c1"},
	}, "")
	require.NoError(t, err)
	id := seeded[0].ID

	creates, updates, err := kb.WriteAndUpdateBatch(db,
		[]kb.Entry{{Type: "fact", Project: "acme-corp", Title: "should-not-persist", Content: "c2"}},
		[]kb.EntryUpdate{{ID: id, Title: strPtr("")}},
		"",
	)
	require.Error(t, err)
	assert.Nil(t, creates)
	assert.Nil(t, updates)

	docs, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 50)
	require.NoError(t, err)
	require.Len(t, docs, 1, "only the pre-seeded doc should exist; the create must not persist")
	assert.Equal(t, "untouched-by-update", docs[0].Title)
}

// TestWriteAndUpdateBatch_BeginTxError exercises the db.BeginTx() error
// path (closed DB).
func TestWriteAndUpdateBatch_BeginTxError(t *testing.T) {
	db := testDB(t)
	require.NoError(t, db.Close())

	creates, updates, err := kb.WriteAndUpdateBatch(db,
		[]kb.Entry{{Type: "fact", Project: "acme-corp", Title: "x", Content: "c"}},
		nil, "")
	require.Error(t, err)
	assert.Nil(t, creates)
	assert.Nil(t, updates)
}

// TestValidateEntryUpdate covers each ValidateEntryUpdate rejection branch.
func TestValidateEntryUpdate(t *testing.T) {
	cases := []struct {
		name    string
		update  kb.EntryUpdate
		wantErr string
	}{
		{"empty type", kb.EntryUpdate{Type: strPtr("")}, "type cannot be empty"},
		{"invalid type", kb.EntryUpdate{Type: strPtr("bogus")}, "invalid type"},
		{"empty project", kb.EntryUpdate{Project: strPtr("")}, "project cannot be empty"},
		{"empty title", kb.EntryUpdate{Title: strPtr("")}, "title cannot be empty"},
		{"empty content", kb.EntryUpdate{Content: strPtr("")}, "content cannot be empty"},
		{"content too long", kb.EntryUpdate{Content: strPtr(string(make([]rune, kb.ContentMaxLen+1)))}, "exceeds"},
		{"valid, no error", kb.EntryUpdate{Type: strPtr("decision"), Title: strPtr("ok")}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := kb.ValidateEntryUpdate(tc.update)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// TestWriteBatch_ConflictExcerptTruncated verifies long previous content is truncated to 100 chars.
func TestWriteBatch_ConflictExcerptTruncated(t *testing.T) {
	db := testDB(t)

	longContent := string(make([]byte, 200))
	for i := range longContent {
		_ = i
	}
	longContent = "abcdefghij" // will build a 200-char string below
	for len(longContent) < 200 {
		longContent += "x"
	}

	_, err := kb.WriteBatch(db, []kb.Entry{
		{Type: "note", Project: "acme-corp", Title: "trunc-test", Content: longContent},
	}, "")
	require.NoError(t, err)

	results, err := kb.WriteBatch(db, []kb.Entry{
		{Type: "note", Project: "acme-corp", Title: "trunc-test", Content: "different content"},
	}, "")
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.True(t, results[0].Conflict)
	assert.LessOrEqual(t, len(results[0].PreviousExcerpt), 100)
}
