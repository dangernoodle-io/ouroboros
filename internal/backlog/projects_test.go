package backlog_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
)

func TestCreateProject(t *testing.T) {
	d := testDB(t)

	p, err := backlog.CreateProject(d, "acme-corp", "AC")
	require.NoError(t, err)

	assert.NotZero(t, p.ID)
	assert.Equal(t, "acme-corp", p.Name)
	assert.Equal(t, "AC", p.Prefix)
	assert.NotEmpty(t, p.Created)
}

func TestCreateProjectDuplicate(t *testing.T) {
	d := testDB(t)

	_, err := backlog.CreateProject(d, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.CreateProject(d, "acme-corp", "AC2")
	assert.Error(t, err)
}

func TestListProjects(t *testing.T) {
	d := testDB(t)

	_, err := backlog.CreateProject(d, "zebra-inc", "ZI")
	require.NoError(t, err)

	_, err = backlog.CreateProject(d, "acme-corp", "AC")
	require.NoError(t, err)

	projects, err := backlog.ListProjects(d)
	require.NoError(t, err)

	assert.Len(t, projects, 2)
	assert.Equal(t, "acme-corp", projects[0].Name)
	assert.Equal(t, "zebra-inc", projects[1].Name)
}

func TestGetProjectByName(t *testing.T) {
	d := testDB(t)

	created, err := backlog.CreateProject(d, "acme-corp", "AC")
	require.NoError(t, err)

	p, err := backlog.GetProjectByName(d, "acme-corp")
	require.NoError(t, err)

	assert.Equal(t, created.ID, p.ID)
	assert.Equal(t, "acme-corp", p.Name)
}

func TestGetProjectByNameNotFound(t *testing.T) {
	d := testDB(t)

	_, err := backlog.GetProjectByName(d, "nonexistent")
	assert.Error(t, err)
}

func TestGetProjectByID(t *testing.T) {
	d := testDB(t)

	created, err := backlog.CreateProject(d, "acme-corp", "AC")
	require.NoError(t, err)

	p, err := backlog.GetProjectByID(d, created.ID)
	require.NoError(t, err)

	assert.Equal(t, created.ID, p.ID)
	assert.Equal(t, "acme-corp", p.Name)
	assert.Equal(t, "AC", p.Prefix)
}

func TestGetProjectByIDNotFound(t *testing.T) {
	d := testDB(t)

	_, err := backlog.GetProjectByID(d, 9999)
	assert.Error(t, err)
}
