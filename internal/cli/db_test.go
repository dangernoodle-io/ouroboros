package cli

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"
)

func TestWithDB_HappyPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PROJECT_KB_PATH", filepath.Join(dir, "test.db"))

	called := false
	err := withDB(func(db *sql.DB) error {
		called = true
		var n int
		return db.QueryRow("SELECT 1").Scan(&n)
	})

	require.NoError(t, err)
	assert.True(t, called)
}

func TestWithDB_OpenError(t *testing.T) {
	// Point at a directory — SQLite will fail when executing the PRAGMA.
	dir := t.TempDir()
	t.Setenv("PROJECT_KB_PATH", dir)

	err := withDB(func(_ *sql.DB) error { return nil })

	require.Error(t, err)
	assert.Contains(t, err.Error(), "open database")
}

func TestWithDB_FnError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PROJECT_KB_PATH", filepath.Join(dir, "test.db"))

	sentinel := errors.New("fn error")
	err := withDB(func(_ *sql.DB) error {
		return sentinel
	})

	require.Error(t, err)
	assert.Equal(t, sentinel, err)
}
