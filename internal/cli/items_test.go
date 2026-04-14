package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
)

func TestRunItemsProjectFound(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Fix bug", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runItems(&buf, db, "acme-corp", "open")
	require.NoError(t, err)

	var items []backlog.Item
	require.NoError(t, json.Unmarshal(buf.Bytes(), &items))
	assert.Len(t, items, 1)
	assert.Equal(t, "Fix bug", items[0].Title)
}

func TestRunItemsProjectNotFound(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runItems(&buf, db, "nonexistent", "open")
	require.NoError(t, err) // swallowed
	assert.Equal(t, "[]", strings.TrimSpace(buf.String()))
}

func TestRunItemsStatusDone(t *testing.T) {
	db := newTestDB(t)
	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Open item", "", "")
	require.NoError(t, err)
	item2, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Closed item", "", "")
	require.NoError(t, err)
	_, err = backlog.UpdateItem(db, item2.ID, map[string]string{"status": "done"})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runItems(&buf, db, "acme-corp", "done")
	require.NoError(t, err)

	var items []backlog.Item
	require.NoError(t, json.Unmarshal(buf.Bytes(), &items))
	require.Len(t, items, 1)
	assert.Equal(t, "Closed item", items[0].Title)
}
