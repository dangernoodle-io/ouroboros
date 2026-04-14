package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"

	"dangernoodle.io/ouroboros/internal/store"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	require.NoError(t, store.ApplySchema(db))
	t.Cleanup(func() { db.Close() })
	return db
}

func TestRunQueryByProject(t *testing.T) {
	db := newTestDB(t)
	_, err := store.UpsertDocument(db, store.Document{
		Type: "decision", Project: "acme-corp", Title: "Use PostgreSQL",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type: "decision", Project: "other", Title: "Use MySQL",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runQuery(&buf, db, "acme-corp", "", "", 10)
	require.NoError(t, err)

	var summaries []store.DocumentSummary
	require.NoError(t, json.Unmarshal(buf.Bytes(), &summaries))
	assert.Len(t, summaries, 1)
	assert.Equal(t, "Use PostgreSQL", summaries[0].Title)
}

func TestRunQueryByType(t *testing.T) {
	db := newTestDB(t)
	_, err := store.UpsertDocument(db, store.Document{
		Type: "decision", Project: "acme-corp", Title: "Decision 1",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type: "fact", Project: "acme-corp", Title: "Fact 1",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runQuery(&buf, db, "", "decision", "", 10)
	require.NoError(t, err)

	var summaries []store.DocumentSummary
	require.NoError(t, json.Unmarshal(buf.Bytes(), &summaries))
	assert.Len(t, summaries, 1)
	assert.Equal(t, "decision", summaries[0].Type)
}

func TestRunQuerySearch(t *testing.T) {
	db := newTestDB(t)
	_, err := store.UpsertDocument(db, store.Document{
		Type: "decision", Project: "acme-corp", Title: "Use PostgreSQL", Content: "Performance benefits",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type: "fact", Project: "acme-corp", Title: "Database Type", Content: "PostgreSQL",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runQuery(&buf, db, "", "", "PostgreSQL", 10)
	require.NoError(t, err)

	var summaries []store.DocumentSummary
	require.NoError(t, json.Unmarshal(buf.Bytes(), &summaries))
	assert.GreaterOrEqual(t, len(summaries), 1)
}

func TestRunQueryLimit(t *testing.T) {
	db := newTestDB(t)
	for i := 0; i < 5; i++ {
		_, err := store.UpsertDocument(db, store.Document{
			Type: "decision", Project: "acme-corp", Title: fmt.Sprintf("Doc %d", i),
		})
		require.NoError(t, err)
	}

	var buf bytes.Buffer
	err := runQuery(&buf, db, "acme-corp", "", "", 3)
	require.NoError(t, err)

	var summaries []store.DocumentSummary
	require.NoError(t, json.Unmarshal(buf.Bytes(), &summaries))
	assert.Len(t, summaries, 3)
}
