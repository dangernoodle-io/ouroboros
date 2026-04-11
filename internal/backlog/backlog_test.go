package backlog_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/store"
	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	require.NoError(t, store.ApplySchema(db))
	t.Cleanup(func() { db.Close() })
	return db
}
