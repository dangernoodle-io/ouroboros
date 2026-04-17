package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"

	"dangernoodle.io/ouroboros/internal/store"
)

func TestReadImportSourceStdin(t *testing.T) {
	got, err := readImportSource(strings.NewReader("payload"), nil)
	require.NoError(t, err)
	assert.Equal(t, "payload", got)

	got, err = readImportSource(strings.NewReader("dash"), []string{"-"})
	require.NoError(t, err)
	assert.Equal(t, "dash", got)
}

func TestReadImportSourceFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in.json")
	require.NoError(t, os.WriteFile(path, []byte("from-file"), 0o600))

	got, err := readImportSource(strings.NewReader(""), []string{path})
	require.NoError(t, err)
	assert.Equal(t, "from-file", got)
}

func TestReadImportSourceFileMissing(t *testing.T) {
	_, err := readImportSource(strings.NewReader(""), []string{"/nonexistent/nope.json"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read file")
}

func TestRunImportError(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runImport(&buf, db, "", "not json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "import")
}

func TestRunImportValidJSON(t *testing.T) {
	db := newTestDB(t)

	content := `{
		"documents": [
			{
				"type": "decision",
				"project": "test-proj",
				"title": "Use PostgreSQL",
				"content": "Decision rationale"
			}
		]
	}`

	var buf bytes.Buffer
	err := runImport(&buf, db, "", content)
	require.NoError(t, err)
	assert.Equal(t, "ok\n", buf.String())

	// Verify document was imported
	docs, err := store.QueryDocuments(db, "", []string{"test-proj"}, "", "", nil, 10)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "Use PostgreSQL", docs[0].Title)
}

func TestRunImportWithDefaultProject(t *testing.T) {
	db := newTestDB(t)

	content := `{
		"documents": [
			{
				"type": "decision",
				"title": "Use PostgreSQL",
				"content": "Decision rationale"
			}
		]
	}`

	var buf bytes.Buffer
	err := runImport(&buf, db, "default-proj", content)
	require.NoError(t, err)
	assert.Equal(t, "ok\n", buf.String())

	// Verify document was imported with default project
	docs, err := store.QueryDocuments(db, "", []string{"default-proj"}, "", "", nil, 10)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "default-proj", docs[0].Project)
}

func TestRunImportMultipleDocuments(t *testing.T) {
	db := newTestDB(t)

	content := `{
		"documents": [
			{
				"type": "decision",
				"project": "test-proj",
				"title": "Decision 1",
				"content": "Content 1"
			},
			{
				"type": "fact",
				"project": "test-proj",
				"title": "Fact 1",
				"content": "Content 2"
			},
			{
				"type": "note",
				"project": "test-proj",
				"title": "Note 1",
				"content": "Content 3"
			}
		]
	}`

	var buf bytes.Buffer
	err := runImport(&buf, db, "", content)
	require.NoError(t, err)
	assert.Equal(t, "ok\n", buf.String())

	// Verify all documents were imported
	docs, err := store.QueryDocuments(db, "", []string{"test-proj"}, "", "", nil, 10)
	require.NoError(t, err)
	require.Len(t, docs, 3)
}

func TestRunImportEmptyContent(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runImport(&buf, db, "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestRunImportInvalidJSON(t *testing.T) {
	db := newTestDB(t)

	content := `{invalid json`

	var buf bytes.Buffer
	err := runImport(&buf, db, "", content)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "import:")
}

func TestRunImportMissingProject(t *testing.T) {
	db := newTestDB(t)

	content := `{
		"documents": [
			{
				"type": "decision",
				"title": "No project doc"
			}
		]
	}`

	var buf bytes.Buffer
	err := runImport(&buf, db, "", content)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing project")
}

func TestRunImportDatabaseError(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Close())

	content := `{
		"documents": [
			{
				"type": "decision",
				"project": "test",
				"title": "Test"
			}
		]
	}`

	var buf bytes.Buffer
	err := runImport(&buf, db, "", content)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "import:")
}

func TestRunImportWithTags(t *testing.T) {
	db := newTestDB(t)

	content := `{
		"documents": [
			{
				"type": "decision",
				"project": "test-proj",
				"title": "Deployment strategy",
				"content": "Use blue-green deployment",
				"tags": ["infrastructure", "deployment"]
			}
		]
	}`

	var buf bytes.Buffer
	err := runImport(&buf, db, "", content)
	require.NoError(t, err)

	// Verify document with tags was imported
	docs, err := store.QueryDocuments(db, "", []string{"test-proj"}, "", "", nil, 10)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "Deployment strategy", docs[0].Title)
	assert.ElementsMatch(t, []string{"infrastructure", "deployment"}, docs[0].Tags)
}

func TestRunImportWithMetadata(t *testing.T) {
	db := newTestDB(t)

	content := `{
		"documents": [
			{
				"type": "decision",
				"project": "test-proj",
				"title": "Test with metadata",
				"content": "Content",
				"metadata": {
					"author": "test-user@example.com",
					"status": "approved"
				}
			}
		]
	}`

	var buf bytes.Buffer
	err := runImport(&buf, db, "", content)
	require.NoError(t, err)

	// Verify document with metadata was imported
	docs, err := store.QueryDocuments(db, "", []string{"test-proj"}, "", "", nil, 10)
	require.NoError(t, err)
	require.Len(t, docs, 1)

	// Fetch full document to check metadata
	doc, err := store.GetDocument(db, docs[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "test-user@example.com", doc.Metadata["author"])
	assert.Equal(t, "approved", doc.Metadata["status"])
}

func TestRunImportArrayFormat(t *testing.T) {
	db := newTestDB(t)

	// Test if input starting with [ is properly handled
	content := `[
		{
			"type": "decision",
			"project": "test-proj",
			"title": "Array format test"
		}
	]`

	var buf bytes.Buffer
	// Array format should be treated as invalid (not wrapped in {documents: [...]})
	err := runImport(&buf, db, "", content)
	require.Error(t, err)
}

func TestRunImportUnsupportedFormat(t *testing.T) {
	db := newTestDB(t)

	content := "This is plain text, not JSON"

	var buf bytes.Buffer
	err := runImport(&buf, db, "", content)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
}
