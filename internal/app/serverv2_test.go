package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildServerV2Composes verifies buildServerV2 composes without error —
// the smoke test the OU-1 build spec requires. buildServerV2 is dark (not
// wired into app.Serve/cli) this PR.
func TestBuildServerV2Composes(t *testing.T) {
	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)
	assert.NotNil(t, app)
}
