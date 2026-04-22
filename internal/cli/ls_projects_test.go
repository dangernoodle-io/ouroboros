package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
)

func TestLSProjectsList(t *testing.T) {
	db := newTestDB(t)
	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.CreateProject(db, "other-project", "OP")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSProjects(&buf, db, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "PREFIX")
	assert.Contains(t, output, "acme-corp")
	assert.Contains(t, output, "AC")
	assert.Contains(t, output, "other-project")
	assert.Contains(t, output, "OP")
}

func TestLSProjectsListJSON(t *testing.T) {
	db := newTestDB(t)
	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.CreateProject(db, "other-project", "OP")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSProjects(&buf, db, true)
	require.NoError(t, err)

	var projects []backlog.Project
	err = json.Unmarshal(buf.Bytes(), &projects)
	require.NoError(t, err)
	require.Len(t, projects, 2)

	names := []string{projects[0].Name, projects[1].Name}
	assert.Contains(t, names, "acme-corp")
	assert.Contains(t, names, "other-project")
}

func TestLSProjectsEmpty(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runLSProjects(&buf, db, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "PREFIX")
}

func TestLSProjectsOrdering(t *testing.T) {
	db := newTestDB(t)
	_, err := backlog.CreateProject(db, "zebra-project", "ZP")
	require.NoError(t, err)
	_, err = backlog.CreateProject(db, "alpha-project", "AP")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runLSProjects(&buf, db, true)
	require.NoError(t, err)

	var projects []backlog.Project
	err = json.Unmarshal(buf.Bytes(), &projects)
	require.NoError(t, err)

	// Should be ordered by name alphabetically
	assert.Equal(t, "alpha-project", projects[0].Name)
	assert.Equal(t, "zebra-project", projects[1].Name)
}
