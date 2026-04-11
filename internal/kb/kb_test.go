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

	markdown, err := kb.ExportMarkdown(testdb, "", "")
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
	markdown, err := kb.ExportMarkdown(testdb, "acme-corp", "")
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
	markdown, err := kb.ExportMarkdown(testdb, "acme-corp", "")
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
	markdown, err := kb.ExportMarkdown(testdb, "", "decision")
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
	decisions, err := store.QueryDocuments(testdb, "decision", "acme-corp", "", "", nil, 50)
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	assert.Equal(t, "Use PostgreSQL", decisions[0].Title)

	// Verify fact imported
	facts, err := store.QueryDocuments(testdb, "fact", "acme-corp", "", "", nil, 50)
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
	decisions, err := store.QueryDocuments(testdb, "decision", "acme-corp", "", "", nil, 50)
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	assert.Equal(t, "acme-corp", decisions[0].Project)

	// Verify fact used default project
	facts, err := store.QueryDocuments(testdb, "fact", "acme-corp", "", "", nil, 50)
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
	docs, err := store.QueryDocuments(testdb, "decision", "acme-corp", "", "", nil, 50)
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
	markdown, err := kb.ExportMarkdown(testdb1, "acme-corp", "")
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
	docs1, err := store.QueryDocuments(testdb1, "", "acme-corp", "", "", nil, 500)
	require.NoError(t, err)

	docs2, err := store.QueryDocuments(testdb2, "", "acme-corp", "", "", nil, 500)
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

	docs, err := store.QueryDocuments(testdb, "decision", "acme-corp", "", "", nil, 50)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "Test Decision", docs[0].Title)
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
	docs1, err := store.QueryDocuments(testdb, "decision", "acme-corp", "", "", nil, 50)
	require.NoError(t, err)
	assert.Len(t, docs1, 1)

	docs2, err := store.QueryDocuments(testdb, "decision", "other-proj", "", "", nil, 50)
	require.NoError(t, err)
	assert.Len(t, docs2, 1)

	facts1, err := store.QueryDocuments(testdb, "fact", "acme-corp", "", "", nil, 50)
	require.NoError(t, err)
	assert.Len(t, facts1, 1)

	facts2, err := store.QueryDocuments(testdb, "fact", "other-proj", "", "", nil, 50)
	require.NoError(t, err)
	assert.Len(t, facts2, 1)
}
