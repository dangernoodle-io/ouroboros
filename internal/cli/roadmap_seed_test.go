package cli

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/store"
)

func TestRunRoadmapSeedBucketsAndCopiesAxes(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "now item", "desc", "", "wifi", "AC-1")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P2", "next item", "", "", "core", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P4", "parked item", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runRoadmapSeed(&buf, db, "acme-corp", "", "", "open", false)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "added 3, skipped 0, replaced 0")

	var showBuf bytes.Buffer
	require.NoError(t, runRoadmapShow(&showBuf, db, "acme-corp", "", "", ""))
	assert.Contains(t, showBuf.String(), "now item")
	assert.Contains(t, showBuf.String(), "wifi")
}

func TestRunRoadmapSeedPriorityFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "in scope", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P3", "out of scope", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runRoadmapSeed(&buf, db, "acme-corp", "P1", "", "open", false)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "added 1, skipped 0, replaced 0")
}

func TestRunRoadmapSeedComponentFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "wifi item", "", "", "wifi", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "core item", "", "", "core", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runRoadmapSeed(&buf, db, "acme-corp", "", "wifi", "open", false)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "added 1, skipped 0, replaced 0")
}

func TestRunRoadmapSeedStatusFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "done item", "", "", "", "")
	require.NoError(t, err)
	require.NoError(t, backlog.MarkDone(db, item.ID))
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "open item", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runRoadmapSeed(&buf, db, "acme-corp", "", "", "done", false)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "added 1, skipped 0, replaced 0")
}

func TestRunRoadmapSeedAdditiveDedupThenReplace(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "will change", "", "", "", "")
	require.NoError(t, err)

	var buf1 bytes.Buffer
	require.NoError(t, runRoadmapSeed(&buf1, db, "acme-corp", "", "", "open", false))
	assert.Contains(t, buf1.String(), "added 1, skipped 0, replaced 0")

	// Re-running additively must skip the already-seeded item.
	var buf2 bytes.Buffer
	require.NoError(t, runRoadmapSeed(&buf2, db, "acme-corp", "", "", "open", false))
	assert.Contains(t, buf2.String(), "added 0, skipped 1, replaced 0")

	// Priority changed; --replace should re-bucket it.
	_, err = backlog.UpdateItem(db, item.ID, map[string]string{"priority": "P2"})
	require.NoError(t, err)

	var buf3 bytes.Buffer
	require.NoError(t, runRoadmapSeed(&buf3, db, "acme-corp", "", "", "open", true))
	assert.Contains(t, buf3.String(), "added 0, skipped 0, replaced 1")

	var showBuf bytes.Buffer
	require.NoError(t, runRoadmapShow(&showBuf, db, "acme-corp", "", "", ""))
	assert.Contains(t, showBuf.String(), "will change")
}

func TestRunRoadmapSeedEmptyBacklogNoOp(t *testing.T) {
	db := newTestDB(t)
	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runRoadmapSeed(&buf, db, "acme-corp", "", "", "open", false)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "added 0, skipped 0, replaced 0")
}

func TestRunRoadmapSeedProjectNotFound(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runRoadmapSeed(&buf, db, "nope", "", "", "open", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRunRoadmapSeedInvalidPriority(t *testing.T) {
	db := newTestDB(t)
	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runRoadmapSeed(&buf, db, "acme-corp", "bogus", "", "open", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --priority")
}

func TestRunRoadmapSeedInvalidStatus(t *testing.T) {
	db := newTestDB(t)
	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runRoadmapSeed(&buf, db, "acme-corp", "", "", "opne", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --status")
}

func TestRoadmapSeedCmdRequiresBacklogFlag(t *testing.T) {
	roadmapSeedBacklog = false
	err := roadmapSeedCmd.RunE(roadmapSeedCmd, []string{"acme-corp"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--backlog is required")
}

func resetRoadmapSeedVars() {
	roadmapSeedBacklog = false
	roadmapSeedPriority = ""
	roadmapSeedComponent = ""
	roadmapSeedStatus = "open"
	roadmapSeedReplace = false
}

// TestRoadmapSeedCmd_Success exercises the RunE closure's withDB wiring
// (rather than calling runRoadmapSeed directly, like the other tests in this
// file), since that closure never executes otherwise: cobra never dispatches
// through rootCmd in the test binary.
func TestRoadmapSeedCmd_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roadmap.db")
	t.Setenv("PROJECT_KB_PATH", path)

	seedDB, err := store.InitDB(path)
	require.NoError(t, err)
	proj, err := backlog.CreateProject(seedDB, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(seedDB, proj.ID, proj.Prefix, "P0", "cmd item", "", "", "", "")
	require.NoError(t, err)
	require.NoError(t, seedDB.Close())

	resetRoadmapSeedVars()
	roadmapSeedBacklog = true
	t.Cleanup(resetRoadmapSeedVars)

	roadmapSeedCmd.SetOut(&bytes.Buffer{})
	err = roadmapSeedCmd.RunE(roadmapSeedCmd, []string{"acme-corp"})
	require.NoError(t, err)
}

// TestMaxPriorityNumEmpty covers the empty-input branch of maxPriorityNum,
// which the CLI-level runRoadmapSeed path never reaches directly (it only
// calls maxPriorityNum when --priority is non-empty).
func TestMaxPriorityNumEmpty(t *testing.T) {
	n, err := maxPriorityNum("")
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestMaxPriorityNumMalformedNumber(t *testing.T) {
	_, err := maxPriorityNum("PA")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --priority")
}

// TestMaxPriorityNumAcceptsWhitespaceCaseAndMultiDigit locks maxPriorityNum's
// pre-refactor behavior: it already TrimSpace'd before validating, so
// leading/trailing whitespace, lowercase "p", and multi-digit numbers were
// (and must remain) ACCEPTED, not rejected — delegating to
// backlog.ParsePriority must not silently loosen or tighten this.
func TestMaxPriorityNumAcceptsWhitespaceCaseAndMultiDigit(t *testing.T) {
	cases := []struct {
		in    string
		wantN int
	}{
		{" P3", 3},
		{"P3 ", 3},
		{"  P3  ", 3},
		{"p3", 3},
		{"P10", 10},
	}
	for _, tc := range cases {
		n, err := maxPriorityNum(tc.in)
		require.NoError(t, err, "input %q should be accepted", tc.in)
		assert.Equal(t, tc.wantN, n, "input %q", tc.in)
	}
}

// TestMaxPriorityNumRejectsInternalWhitespace locks a rejection: whitespace
// BETWEEN the "P" and the digits (not merely surrounding the whole flag) is
// malformed both before and after the refactor.
func TestMaxPriorityNumRejectsInternalWhitespace(t *testing.T) {
	_, err := maxPriorityNum("P 3")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --priority")
}

// TestRunRoadmapSeedDefaultStatus covers runRoadmapSeed's default-status
// assignment (status == "" -> "open"), reachable only via a direct call:
// the CLI flag default is already "open", so the cobra path never passes
// through with an empty string.
func TestRunRoadmapSeedDefaultStatus(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "open item", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runRoadmapSeed(&buf, db, "acme-corp", "", "", "", false)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "added 1, skipped 0, replaced 0")
}

// TestRunRoadmapSeedListItemsError forces backlog.ListItems to fail (items
// table dropped after the project lookup succeeds) to hit the "list
// backlog" error-wrapping branch.
func TestRunRoadmapSeedListItemsError(t *testing.T) {
	db := newTestDB(t)
	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = db.Exec("DROP TABLE items")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runRoadmapSeed(&buf, db, "acme-corp", "", "", "open", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list backlog")
}

// TestRunRoadmapSeedMutateError forces roadmap.Mutate to fail (documents
// table dropped, so loadTx can't find the roadmap doc storage) to hit the
// Mutate/Seed error-propagation branch (the earlier nit fix).
func TestRunRoadmapSeedMutateError(t *testing.T) {
	db := newTestDB(t)
	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = db.Exec("DROP TABLE documents")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runRoadmapSeed(&buf, db, "acme-corp", "", "", "open", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "roadmap seed:")
	assert.Contains(t, err.Error(), "mutate roadmap:")
}
