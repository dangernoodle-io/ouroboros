package app

import (
	"database/sql"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/store"
)

// db is the package-level in-memory SQLite handle every test in this
// package shares, opened once in TestMain and reset per-test via
// resetDB/resetAllDB rather than reopened.
var db *sql.DB

func TestMain(m *testing.M) {
	var err error
	db, err = sql.Open("sqlite", ":memory:")
	if err != nil {
		panic(err)
	}
	if err = store.ApplySchema(db); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// resetDB clears the kb-only tables (documents, edges) between tests.
func resetDB(t *testing.T) {
	t.Helper()
	_, err := db.Exec("DELETE FROM documents")
	require.NoError(t, err)
	_, err = db.Exec("DELETE FROM edges")
	require.NoError(t, err)
	require.NoError(t, store.RebuildFTS(db))
}

// resetAllDB clears every table a backlog/roadmap test might have touched.
func resetAllDB(t *testing.T) {
	t.Helper()
	_, err := db.Exec("DELETE FROM items")
	require.NoError(t, err)
	_, err = db.Exec("DELETE FROM plans")
	require.NoError(t, err)
	_, err = db.Exec("DELETE FROM projects")
	require.NoError(t, err)
	_, err = db.Exec("DELETE FROM config")
	require.NoError(t, err)
	_, err = db.Exec("DELETE FROM documents")
	require.NoError(t, err)
	_, err = db.Exec("DELETE FROM edges")
	require.NoError(t, err)
	_, err = db.Exec("DELETE FROM item_id_aliases")
	require.NoError(t, err)
}
