package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/edges"
)

func TestParseEdgeRef(t *testing.T) {
	typ, id, err := parseEdgeRef("item:BB-9")
	require.NoError(t, err)
	assert.Equal(t, "item", typ)
	assert.Equal(t, "BB-9", id)

	typ, id, err = parseEdgeRef("kb:123")
	require.NoError(t, err)
	assert.Equal(t, "kb", typ)
	assert.Equal(t, "123", id)

	_, _, err = parseEdgeRef("no-colon")
	assert.Error(t, err)

	_, _, err = parseEdgeRef("doc:5")
	assert.Error(t, err)

	_, _, err = parseEdgeRef("item:")
	assert.Error(t, err)
}

func TestRunLinkAndUnlink(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item a", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item b", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLink(&buf, db, "item:AC-1", "blocks", "item:AC-2")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "linked item:AC-1 blocks item:AC-2")

	list, err := edges.EdgesFor(db, edges.TypeItem, "AC-1")
	require.NoError(t, err)
	require.Len(t, list, 1)

	buf.Reset()
	err = runUnlink(&buf, db, "item:AC-1", "blocks", "item:AC-2")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "unlinked item:AC-1 blocks item:AC-2")

	list, err = edges.EdgesFor(db, edges.TypeItem, "AC-1")
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestRunUnlinkNoMatch(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runUnlink(&buf, db, "item:AC-1", "blocks", "item:AC-2")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "no edge")
}

func TestRunLinkInvalidLabel(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runLink(&buf, db, "item:AC-1", "part-of", "item:AC-2")
	assert.Error(t, err)
}

func TestRunLinkInvalidRef(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runLink(&buf, db, "bogus", "blocks", "item:AC-2")
	assert.Error(t, err)
}

func TestRunLinkInvalidDstRef(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runLink(&buf, db, "item:AC-1", "blocks", "bogus")
	assert.Error(t, err)
}

func TestRunUnlinkInvalidRef(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runUnlink(&buf, db, "bogus", "blocks", "item:AC-2")
	assert.Error(t, err)

	err = runUnlink(&buf, db, "item:AC-1", "blocks", "bogus")
	assert.Error(t, err)
}
