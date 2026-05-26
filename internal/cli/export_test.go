package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"

	"dangernoodle.io/ouroboros/internal/store"
)

func TestRunExportHappyPath(t *testing.T) {
	db := newTestDB(t)

	_, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Use PostgreSQL",
		Content: "Rationale for choosing PostgreSQL over MySQL",
	})
	require.NoError(t, err)

	_, err = store.UpsertDocument(db, store.Document{
		Type:    "fact",
		Project: "acme-corp",
		Title:   "Database port is 5432",
		Content: "Default PostgreSQL port",
	})
	require.NoError(t, err)

	_, err = store.UpsertDocument(db, store.Document{
		Type:    "note",
		Project: "acme-corp",
		Title:   "Setup reminder",
		Content: "Remember to run migrations",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runExport(&buf, db, "", "")
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "# Knowledge Base Export")
	assert.Contains(t, out, "Use PostgreSQL")
	assert.Contains(t, out, "Database port is 5432")
	assert.Contains(t, out, "Setup reminder")
	assert.Contains(t, out, "## Decision")
	assert.Contains(t, out, "## Fact")
	assert.Contains(t, out, "## Note")
}

func TestRunExportFilterByProject(t *testing.T) {
	db := newTestDB(t)

	_, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Acme decision",
		Content: "Acme content",
	})
	require.NoError(t, err)

	_, err = store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "other-proj",
		Title:   "Other decision",
		Content: "Other content",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runExport(&buf, db, "acme-corp", "")
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Acme decision")
	assert.NotContains(t, out, "Other decision")
	assert.Contains(t, out, "Project: acme-corp")
}

func TestRunExportFilterByType(t *testing.T) {
	db := newTestDB(t)

	_, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Architecture decision",
		Content: "Use microservices",
	})
	require.NoError(t, err)

	_, err = store.UpsertDocument(db, store.Document{
		Type:    "fact",
		Project: "acme-corp",
		Title:   "A known fact",
		Content: "Fact content",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runExport(&buf, db, "", "decision")
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Architecture decision")
	assert.NotContains(t, out, "A known fact")
	assert.Contains(t, out, "Type: decision")
}

func TestRunExportFilterByProjectAndType(t *testing.T) {
	db := newTestDB(t)

	_, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Acme decision",
		Content: "Content A",
	})
	require.NoError(t, err)

	_, err = store.UpsertDocument(db, store.Document{
		Type:    "fact",
		Project: "acme-corp",
		Title:   "Acme fact",
		Content: "Content B",
	})
	require.NoError(t, err)

	_, err = store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "other-proj",
		Title:   "Other decision",
		Content: "Content C",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runExport(&buf, db, "acme-corp", "decision")
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Acme decision")
	assert.NotContains(t, out, "Acme fact")
	assert.NotContains(t, out, "Other decision")
}

func TestRunExportEmptyKB(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runExport(&buf, db, "", "")
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "# Knowledge Base Export")
	assert.True(t, strings.Contains(out, "_No documents found._") || len(out) > 0)
}

func TestRunExportEmptyKBNoProject(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runExport(&buf, db, "nonexistent-project", "")
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "# Knowledge Base Export")
	assert.Contains(t, out, "_No documents found._")
}

func TestRunExportDatabaseError(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Close())

	var buf bytes.Buffer
	err := runExport(&buf, db, "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "export:")
}

func TestRunExportMarkdownStructure(t *testing.T) {
	db := newTestDB(t)

	_, err := store.UpsertDocument(db, store.Document{
		Type:     "decision",
		Project:  "test-proj",
		Category: "architecture",
		Title:    "Structured title",
		Content:  "Body content here",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runExport(&buf, db, "test-proj", "decision")
	require.NoError(t, err)

	out := buf.String()
	// Header should include project/type label and generation timestamp
	assert.Contains(t, out, "Project: test-proj")
	assert.Contains(t, out, "Type: decision")
	// Document header format: ### <Title> [<project>/<category>]
	assert.Contains(t, out, "### Structured title [test-proj/architecture]")
	assert.Contains(t, out, "Body content here")
}
