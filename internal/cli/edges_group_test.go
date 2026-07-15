package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/edges"
	"dangernoodle.io/ouroboros/internal/store"
)

// These tests exercise the `edges` noun group's cobra RunE closures
// directly (flag parsing + withDB wiring), since cobra never dispatches
// through rootCmd in the test binary. `edges link`/`edges unlink` share
// the exact linkCmd/unlinkCmd.RunE function value (no duplicated logic),
// and `edges list` shares runLSEdges with `ls edges` — this file proves
// the group wiring and the shared behavior, not new business logic.
//
// Because the RunE closures dispatch through withDB (which opens its own
// *sql.DB against PROJECT_KB_PATH, distinct from testutil.TestDB's
// in-memory connection), setup here goes through store.InitDB against the
// same file path so RunE sees the fixtures.

func setupEdgesFileDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "edges.db")
	t.Setenv("PROJECT_KB_PATH", dbPath)
	db, err := store.InitDB(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func resetEdgesListFlags() {
	edgesListLabelFlag = ""
	edgesListTypeFlag = ""
	edgesListIDFlag = ""
	edgesListJSONFlag = false
}

func TestEdgesCmdGroupWiring(t *testing.T) {
	names := make([]string, 0)
	for _, c := range edgesCmd.Commands() {
		names = append(names, c.Name())
	}
	assert.Contains(t, names, "list")
	assert.Contains(t, names, "link")
	assert.Contains(t, names, "unlink")
}

func TestEdgesLinkAndUnlinkShareRunEWithTopLevel(t *testing.T) {
	// edges link/unlink must be the exact same RunE function value as the
	// top-level commands -- proving they can never drift.
	assert.Equal(t,
		reflect.ValueOf(linkCmd.RunE).Pointer(),
		reflect.ValueOf(edgesLinkCmd.RunE).Pointer(),
		"edges link must reuse linkCmd's RunE, not a duplicate")
	assert.Equal(t,
		reflect.ValueOf(unlinkCmd.RunE).Pointer(),
		reflect.ValueOf(edgesUnlinkCmd.RunE).Pointer(),
		"edges unlink must reuse unlinkCmd's RunE, not a duplicate")

	db := setupEdgesFileDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item a", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item b", "", "", "", "")
	require.NoError(t, err)

	// Round-trip via the cobra RunE closures registered under the edges
	// group, proving the wiring works end-to-end through withDB (the
	// run* helper behavior itself is already covered in edges_test.go).
	edgesLinkCmd.SetOut(&bytes.Buffer{})
	err = edgesLinkCmd.RunE(edgesLinkCmd, []string{"item:AC-1", "blocks", "item:AC-2"})
	require.NoError(t, err)

	list, err := edges.EdgesFor(db, edges.TypeItem, "AC-1")
	require.NoError(t, err)
	require.Len(t, list, 1)

	edgesUnlinkCmd.SetOut(&bytes.Buffer{})
	err = edgesUnlinkCmd.RunE(edgesUnlinkCmd, []string{"item:AC-1", "blocks", "item:AC-2"})
	require.NoError(t, err)

	list, err = edges.EdgesFor(db, edges.TypeItem, "AC-1")
	require.NoError(t, err)
	assert.Empty(t, list)
}

// TestTopLevelLinkUnlinkStillWork asserts that `ouroboros link`/`unlink`
// (back-compat aliases) keep working identically to `edges link`/`unlink`.
func TestTopLevelLinkUnlinkStillWork(t *testing.T) {
	db := setupEdgesFileDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item a", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item b", "", "", "", "")
	require.NoError(t, err)

	linkCmd.SetOut(&bytes.Buffer{})
	err = linkCmd.RunE(linkCmd, []string{"item:AC-1", "blocks", "item:AC-2"})
	require.NoError(t, err)

	list, err := edges.EdgesFor(db, edges.TypeItem, "AC-1")
	require.NoError(t, err)
	require.Len(t, list, 1)

	unlinkCmd.SetOut(&bytes.Buffer{})
	err = unlinkCmd.RunE(unlinkCmd, []string{"item:AC-1", "blocks", "item:AC-2"})
	require.NoError(t, err)

	list, err = edges.EdgesFor(db, edges.TypeItem, "AC-1")
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestEdgesLinkCmdBadLabel(t *testing.T) {
	setupEdgesFileDB(t)

	edgesLinkCmd.SetOut(&bytes.Buffer{})
	err := edgesLinkCmd.RunE(edgesLinkCmd, []string{"item:AC-1", "part-of", "item:AC-2"})
	assert.Error(t, err)
}

func TestEdgesUnlinkCmdBadRef(t *testing.T) {
	setupEdgesFileDB(t)

	edgesUnlinkCmd.SetOut(&bytes.Buffer{})
	err := edgesUnlinkCmd.RunE(edgesUnlinkCmd, []string{"bogus", "blocks", "item:AC-2"})
	assert.Error(t, err)
}

func TestEdgesListCmd(t *testing.T) {
	db := setupEdgesFileDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item a", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item b", "", "", "", "")
	require.NoError(t, err)
	_, err = edges.Link(db, edges.TypeItem, "AC-1", "blocks", edges.TypeItem, "AC-2", proj.ID)
	require.NoError(t, err)
	_, err = edges.Link(db, edges.TypeItem, "AC-2", "relates", edges.TypeItem, "AC-1", proj.ID)
	require.NoError(t, err)

	resetEdgesListFlags()
	var buf bytes.Buffer
	edgesListCmd.SetOut(&buf)
	require.NoError(t, edgesListCmd.RunE(edgesListCmd, nil))
	output := buf.String()
	assert.Contains(t, output, "SOURCE")
	assert.Contains(t, output, "item:AC-1")
	assert.Contains(t, output, "blocks")
	assert.Contains(t, output, "relates")
}

func TestEdgesListCmdJSONAndLabelFilter(t *testing.T) {
	db := setupEdgesFileDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item a", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item b", "", "", "", "")
	require.NoError(t, err)
	_, err = edges.Link(db, edges.TypeItem, "AC-1", "blocks", edges.TypeItem, "AC-2", proj.ID)
	require.NoError(t, err)
	_, err = edges.Link(db, edges.TypeItem, "AC-2", "relates", edges.TypeItem, "AC-1", proj.ID)
	require.NoError(t, err)

	resetEdgesListFlags()
	edgesListLabelFlag = "blocks"
	edgesListJSONFlag = true
	var buf bytes.Buffer
	edgesListCmd.SetOut(&buf)
	require.NoError(t, edgesListCmd.RunE(edgesListCmd, nil))

	var got []edges.Edge
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	require.Len(t, got, 1)
	assert.Equal(t, "blocks", got[0].Label)
}

func TestEdgesListCmdByEndpoint(t *testing.T) {
	db := setupEdgesFileDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item a", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item b", "", "", "", "")
	require.NoError(t, err)
	_, err = edges.Link(db, edges.TypeItem, "AC-1", "blocks", edges.TypeItem, "AC-2", proj.ID)
	require.NoError(t, err)

	resetEdgesListFlags()
	edgesListTypeFlag = "item"
	edgesListIDFlag = "AC-2"
	edgesListJSONFlag = true
	var buf bytes.Buffer
	edgesListCmd.SetOut(&buf)
	require.NoError(t, edgesListCmd.RunE(edgesListCmd, nil))

	var got []edges.Edge
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	require.Len(t, got, 1)
	assert.Equal(t, "AC-2", got[0].TargetID)
}

func TestEdgesListCmdTypeWithoutIDErrors(t *testing.T) {
	setupEdgesFileDB(t)

	resetEdgesListFlags()
	edgesListTypeFlag = "item"
	var buf bytes.Buffer
	edgesListCmd.SetOut(&buf)
	err := edgesListCmd.RunE(edgesListCmd, nil)
	assert.Error(t, err)
}
