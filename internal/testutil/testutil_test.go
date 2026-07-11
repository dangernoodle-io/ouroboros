package testutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSkipUnlessAcc_Skipped(t *testing.T) {
	t.Setenv("ACC_OUROBOROS", "")

	SkipUnlessAcc(t)

	require.Fail(t, "expected SkipUnlessAcc to skip when ACC_OUROBOROS is unset")
}

func TestSkipUnlessAcc_Runs(t *testing.T) {
	t.Setenv("ACC_OUROBOROS", "1")

	SkipUnlessAcc(t)

	require.True(t, true, "execution should continue past SkipUnlessAcc")
}

// TestTestDB verifies TestDB opens a migrated, foreign-key-enforced
// in-memory database (t.Cleanup registers the close, run by the testing
// package after this test returns).
func TestTestDB(t *testing.T) {
	db := TestDB(t)

	var fkEnforced int
	require.NoError(t, db.QueryRow("PRAGMA foreign_keys").Scan(&fkEnforced))
	require.Equal(t, 1, fkEnforced, "foreign key enforcement should be on")

	require.NoError(t, db.Ping(), "schema-applied db should be usable")
}
