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

func TestRenameProject(t *testing.T) {
	d := testDB(t)

	created, err := backlog.CreateProject(d, "foo", "FO")
	require.NoError(t, err)

	renamed, err := backlog.RenameProject(d, "foo", "bar")
	require.NoError(t, err)

	assert.Equal(t, "bar", renamed.Name)
	assert.Equal(t, created.ID, renamed.ID)
	assert.Equal(t, "FO", renamed.Prefix)

	// Verify GetProjectByName("bar") succeeds
	p, err := backlog.GetProjectByName(d, "bar")
	require.NoError(t, err)
	assert.Equal(t, "bar", p.Name)

	// Verify GetProjectByName("foo") fails
	_, err = backlog.GetProjectByName(d, "foo")
	assert.Error(t, err)
}

func TestRenameProjectCascadesDocuments(t *testing.T) {
	d := testDB(t)

	_, err := backlog.CreateProject(d, "foo", "FO")
	require.NoError(t, err)

	// Insert 2 documents with project='foo'
	_, err = d.Exec("INSERT INTO documents (type, project, title, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		"decision", "foo", "Doc 1", "content 1", "2024-01-01T00:00:00Z", "2024-01-01T00:00:00Z")
	require.NoError(t, err)

	_, err = d.Exec("INSERT INTO documents (type, project, title, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		"decision", "foo", "Doc 2", "content 2", "2024-01-01T00:00:00Z", "2024-01-01T00:00:00Z")
	require.NoError(t, err)

	// Insert 1 document with project='other'
	_, err = d.Exec("INSERT INTO documents (type, project, title, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		"decision", "other", "Doc 3", "content 3", "2024-01-01T00:00:00Z", "2024-01-01T00:00:00Z")
	require.NoError(t, err)

	// Rename
	_, err = backlog.RenameProject(d, "foo", "bar")
	require.NoError(t, err)

	// Verify 2 docs have project='bar'
	var count int
	err = d.QueryRow("SELECT COUNT(*) FROM documents WHERE project = ?", "bar").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Verify doc with project='other' is unchanged
	err = d.QueryRow("SELECT COUNT(*) FROM documents WHERE project = ?", "other").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Verify no docs remain with project='foo'
	err = d.QueryRow("SELECT COUNT(*) FROM documents WHERE project = ?", "foo").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestRenameProjectMissing(t *testing.T) {
	d := testDB(t)

	_, err := backlog.RenameProject(d, "nope", "bar")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "project not found")

	// Verify no projects were created
	projects, err := backlog.ListProjects(d)
	require.NoError(t, err)
	assert.Len(t, projects, 0)
}

func TestRenameProjectCollision(t *testing.T) {
	d := testDB(t)

	_, err := backlog.CreateProject(d, "foo", "FO")
	require.NoError(t, err)

	_, err = backlog.CreateProject(d, "bar", "BA")
	require.NoError(t, err)

	// Try to rename foo to bar (collision)
	_, err = backlog.RenameProject(d, "foo", "bar")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "project already exists")

	// Verify both projects are unchanged
	foo, err := backlog.GetProjectByName(d, "foo")
	require.NoError(t, err)
	assert.Equal(t, "foo", foo.Name)

	bar, err := backlog.GetProjectByName(d, "bar")
	require.NoError(t, err)
	assert.Equal(t, "bar", bar.Name)
}
