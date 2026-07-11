package testutil

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/store"
	_ "modernc.org/sqlite"
)

// TestDB opens an in-memory SQLite database with foreign keys enforced,
// applies the store schema, and registers a t.Cleanup to close it. Shared
// by every package's tests that need a fresh, migrated database.
func TestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	_, err = db.Exec("PRAGMA foreign_keys=ON")
	require.NoError(t, err)
	require.NoError(t, store.ApplySchema(db))
	t.Cleanup(func() { db.Close() })
	return db
}
