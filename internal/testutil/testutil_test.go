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
