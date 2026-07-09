package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/edges"
)

func TestRunLSEdgesList(t *testing.T) {
	db := newTestDB(t)
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

	var buf bytes.Buffer
	require.NoError(t, runLSEdges(&buf, db, "", "", "", false))
	output := buf.String()
	assert.Contains(t, output, "SOURCE")
	assert.Contains(t, output, "item:AC-1")
	assert.Contains(t, output, "blocks")
	assert.Contains(t, output, "relates")
}

func TestRunLSEdgesFilterByLabel(t *testing.T) {
	db := newTestDB(t)
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

	var buf bytes.Buffer
	require.NoError(t, runLSEdges(&buf, db, "blocks", "", "", true))

	var got []edges.Edge
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	require.Len(t, got, 1)
	assert.Equal(t, "blocks", got[0].Label)
}

// TestRunLSEdgesByEndpointWithLabelFilter exercises the --type/--id
// branch's manual per-edge label filter with both a matching and a
// non-matching edge present.
func TestRunLSEdgesByEndpointWithLabelFilter(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item a", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item b", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item c", "", "", "", "")
	require.NoError(t, err)

	_, err = edges.Link(db, edges.TypeItem, "AC-1", "blocks", edges.TypeItem, "AC-2", proj.ID)
	require.NoError(t, err)
	_, err = edges.Link(db, edges.TypeItem, "AC-3", "relates", edges.TypeItem, "AC-2", proj.ID)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, runLSEdges(&buf, db, "blocks", "item", "AC-2", true))

	var got []edges.Edge
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	require.Len(t, got, 1, "only the blocks edge should survive the manual filter, not the relates one")
	assert.Equal(t, "blocks", got[0].Label)
}

func TestRunLSEdgesByEndpoint(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item a", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item b", "", "", "", "")
	require.NoError(t, err)

	_, err = edges.Link(db, edges.TypeItem, "AC-1", "blocks", edges.TypeItem, "AC-2", proj.ID)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, runLSEdges(&buf, db, "", "item", "AC-2", true))

	var got []edges.Edge
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	require.Len(t, got, 1)
	assert.Equal(t, "AC-2", got[0].TargetID)
}

func TestRunLSEdgesEmpty(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	require.NoError(t, runLSEdges(&buf, db, "", "", "", false))
	assert.Contains(t, buf.String(), "SOURCE")
}

func TestRunLSEdgesTypeWithoutIDErrors(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runLSEdges(&buf, db, "", "item", "", false)
	assert.Error(t, err)
}

// TestRunLSEdgesInvalidLabelErrors verifies a bad --label errors in BOTH
// branches: the no-endpoint (ListEdges) path and the --type/--id
// (EdgesFor) path — the latter used to silently filter to empty instead of
// erroring.
func TestRunLSEdgesInvalidLabelErrors(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item a", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item b", "", "", "", "")
	require.NoError(t, err)
	_, err = edges.Link(db, edges.TypeItem, "AC-1", "blocks", edges.TypeItem, "AC-2", proj.ID)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSEdges(&buf, db, "bogus", "", "", false)
	assert.Error(t, err, "bad --label must error in the no-endpoint branch")

	buf.Reset()
	err = runLSEdges(&buf, db, "bogus", "item", "AC-1", false)
	assert.Error(t, err, "bad --label must error in the --type/--id branch, not silently return empty")
}
