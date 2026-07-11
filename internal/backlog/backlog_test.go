package backlog_test

import (
	"database/sql"
	"testing"

	"dangernoodle.io/ouroboros/internal/testutil"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	return testutil.TestDB(t)
}
