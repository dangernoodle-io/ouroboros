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

func TestRunProjectCreate(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runProjectCreate(&buf, db, "acme-corp")
	require.NoError(t, err)

	var proj backlog.Project
	require.NoError(t, json.Unmarshal(buf.Bytes(), &proj))
	assert.Equal(t, "acme-corp", proj.Name)
	assert.Equal(t, "AC", proj.Prefix)
	assert.NotZero(t, proj.ID)
}

func TestRunProjectCreateInvalidName(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runProjectCreate(&buf, db, "AB")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid project name")
}

func TestRunProjectGet(t *testing.T) {
	db := newTestDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runProjectGet(&buf, db, "acme-corp")
	require.NoError(t, err)

	var proj backlog.Project
	require.NoError(t, json.Unmarshal(buf.Bytes(), &proj))
	assert.Equal(t, "acme-corp", proj.Name)
	assert.Equal(t, "AC", proj.Prefix)
}

func TestRunProjectGetNotFound(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runProjectGet(&buf, db, "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project not found")
}

func TestRunProjectList(t *testing.T) {
	db := newTestDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.CreateProject(db, "test-proj", "TE")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runProjectList(&buf, db)
	require.NoError(t, err)

	var projects []backlog.Project
	require.NoError(t, json.Unmarshal(buf.Bytes(), &projects))
	assert.Len(t, projects, 2)
}

func TestRunProjectListEmpty(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runProjectList(&buf, db)
	require.NoError(t, err)
	assert.Equal(t, "[]", strings.TrimSpace(buf.String()))
}

func TestRunProjectRename(t *testing.T) {
	db := newTestDB(t)

	_, err := backlog.CreateProject(db, "old-name", "OL")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runProjectRename(&buf, db, "old-name", "new-name", "")
	require.NoError(t, err)

	var proj backlog.Project
	require.NoError(t, json.Unmarshal(buf.Bytes(), &proj))
	assert.Equal(t, "new-name", proj.Name)
}

func TestRunProjectRenameInvalidNewName(t *testing.T) {
	db := newTestDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runProjectRename(&buf, db, "acme-corp", "AB", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid project name")
}

func TestRunProjectDeleteEmpty(t *testing.T) {
	db := newTestDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runProjectDelete(&buf, db, "acme-corp", false, "")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "acme-corp")
}

func TestRunProjectDeleteBlocked(t *testing.T) {
	db := newTestDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Task", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runProjectDelete(&buf, db, "acme-corp", false, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "items")
}

func TestRunProjectDeleteForce(t *testing.T) {
	db := newTestDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Task", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runProjectDelete(&buf, db, "acme-corp", true, "")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "acme-corp")
}

func TestRunProjectDeleteReassign(t *testing.T) {
	db := newTestDB(t)

	src, err := backlog.CreateProject(db, "src-proj", "SP")
	require.NoError(t, err)
	_, err = backlog.CreateProject(db, "dst-proj", "DP")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, src.ID, src.Prefix, "P1", "Task", "", "", "", "")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runProjectDelete(&buf, db, "src-proj", false, "dst-proj")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "src-proj")
}

func TestRunProjectDeleteMutex(t *testing.T) {
	db := newTestDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runProjectDelete(&buf, db, "acme-corp", true, "other-proj")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestRunProjectCreatePrefixExhausted(t *testing.T) {
	db := newTestDB(t)

	// Exhaust all prefixes starting with A so DerivePrefix fails for "alpha"
	_, err := backlog.CreateProject(db, "alpha-00", "AL")
	require.NoError(t, err)
	for i := 1; i <= 9; i++ {
		name := "alpha-0" + string(rune('0'+i))
		prefix := "A" + string(rune('0'+i))
		_, err = backlog.CreateProject(db, name, prefix)
		require.NoError(t, err)
	}

	var buf bytes.Buffer
	err = runProjectCreate(&buf, db, "alpha-xyz")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project create")
}

func TestRunProjectRenameNotFound(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runProjectRename(&buf, db, "nonexistent", "new-name", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project rename")
}

func TestRunProjectDeleteNotFound(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runProjectDelete(&buf, db, "nonexistent", false, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project delete")
}

func TestRunProjectRenameNewPrefix(t *testing.T) {
	db := newTestDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	proj, err := backlog.AddItem(db, 1, "AC", "P1", "task", "", "", "", "")
	require.NoError(t, err)
	assert.Equal(t, "AC-1", proj.ID)

	var buf bytes.Buffer
	err = runProjectRename(&buf, db, "acme-corp", "", "XX")
	require.NoError(t, err)

	var renamed backlog.Project
	require.NoError(t, json.Unmarshal(buf.Bytes(), &renamed))
	assert.Equal(t, "acme-corp", renamed.Name)
	assert.Equal(t, "XX", renamed.Prefix)

	// Item should have new ID
	item, err := backlog.GetItem(db, "XX-1")
	require.NoError(t, err)
	assert.Equal(t, "XX-1", item.ID)
}

func TestRunProjectRenameNoFlags(t *testing.T) {
	db := newTestDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runProjectRename(&buf, db, "acme-corp", "", "")
	require.Error(t, err)
}
