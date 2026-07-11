package kb

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/store"
)

// ftsTestDB is a package-internal twin of kb_test.go's testDB — this file
// lives in package kb (not kb_test) so it can swap the unexported rebuildFTS
// seam directly.
func ftsTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)

	err = store.ApplySchema(db)
	require.NoError(t, err)

	return db
}

func TestWriteBatch_RebuildFTSCalledExactlyOnce(t *testing.T) {
	orig := rebuildFTS
	var calls int
	rebuildFTS = func(db *sql.DB) error {
		calls++
		return orig(db)
	}
	t.Cleanup(func() { rebuildFTS = orig })

	db := ftsTestDB(t)

	entries := []Entry{
		{Type: "decision", Project: "acme-corp", Title: "fts-entry-alpha", Content: "distincttoken001"},
		{Type: "fact", Project: "acme-corp", Title: "fts-entry-beta", Content: "distincttoken002"},
		{Type: "note", Project: "acme-corp", Title: "fts-entry-gamma", Content: "distincttoken003"},
	}

	_, err := WriteBatch(db, entries, "")
	require.NoError(t, err)

	require.Equal(t, 1, calls, "a 3-entry batch must rebuild FTS exactly once, not once per entry")

	// FTS search for each distinct token must return the correct doc,
	// proving the single rebuild left the index fully consistent.
	for _, e := range entries {
		results, serr := store.SearchDocuments(db, e.Content, nil, nil, nil, 10)
		require.NoError(t, serr)
		require.Len(t, results, 1, "FTS should find exactly one doc for %q", e.Content)
		assert.Equal(t, e.Title, results[0].Title)
	}
}

func TestImportJSON_RebuildFTSCalledExactlyOnce(t *testing.T) {
	orig := rebuildFTS
	var calls int
	rebuildFTS = func(db *sql.DB) error {
		calls++
		return orig(db)
	}
	t.Cleanup(func() { rebuildFTS = orig })

	db := ftsTestDB(t)

	payload := ImportData{
		Documents: []ImportDocument{
			{Type: "decision", Project: "acme-corp", Title: "import-fts-alpha", Content: "importtoken001"},
			{Type: "fact", Project: "acme-corp", Title: "import-fts-beta", Content: "importtoken002"},
			{Type: "note", Project: "acme-corp", Title: "import-fts-gamma", Content: "importtoken003"},
		},
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)

	err = ImportJSON(db, "", data)
	require.NoError(t, err)

	require.Equal(t, 1, calls, "a 3-doc import batch must rebuild FTS exactly once, not once per document")

	// FTS search for each distinct token must return the correct doc,
	// proving the single rebuild left the index fully consistent.
	for _, doc := range payload.Documents {
		results, serr := store.SearchDocuments(db, doc.Content, nil, nil, nil, 10)
		require.NoError(t, serr)
		require.Len(t, results, 1, "FTS should find exactly one doc for %q", doc.Content)
		assert.Equal(t, doc.Title, results[0].Title)
	}
}
