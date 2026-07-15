package cli

import (
	"database/sql"
	"testing"

	"dangernoodle.io/ouroboros/internal/testutil"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testutil.TestDB(t)
}
