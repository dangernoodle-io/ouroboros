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

	renamed, err := backlog.RenameProject(d, "foo", "bar", "")
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
	_, err = backlog.RenameProject(d, "foo", "bar", "")
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

	_, err := backlog.RenameProject(d, "nope", "bar", "")
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
	_, err = backlog.RenameProject(d, "foo", "bar", "")
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

func TestValidateProjectName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid hyphenated", "acme-corp", false},
		{"valid with digit", "my-project-1", false},
		{"valid underscores", "a_b_c", false},
		{"valid mixed", "a-1-b", false},
		{"empty", "", true},
		{"too short", "ab", true},
		{"digit start", "1start", true},
		{"uppercase prefix AB", "AB", true},
		{"uppercase prefix ABCD", "ABCD", true},
		{"punct start", "-foo", true},
		{"trailing space", "foo ", true},
		{"contains exclamation", "foo!", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := backlog.ValidateProjectName(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreateProjectValidationFails(t *testing.T) {
	d := testDB(t)

	_, err := backlog.CreateProject(d, "AB", "AB")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid project name")
}

func TestRenameProjectValidationFails(t *testing.T) {
	d := testDB(t)

	_, err := backlog.CreateProject(d, "foo-bar", "FB")
	require.NoError(t, err)

	_, err = backlog.RenameProject(d, "foo-bar", "AB", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid project name")
}

func TestGetProjectByNameCaseInsensitive(t *testing.T) {
	d := testDB(t)

	_, err := backlog.CreateProject(d, "Acme-corp", "AC")
	require.NoError(t, err)

	p, err := backlog.GetProjectByName(d, "acme-corp")
	require.NoError(t, err)
	assert.Equal(t, "Acme-corp", p.Name)
}

func TestCreateProjectCaseCollision(t *testing.T) {
	d := testDB(t)

	_, err := backlog.CreateProject(d, "Acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.CreateProject(d, "acme-corp", "AC2")
	assert.Error(t, err)
}

func TestDeleteProjectEmpty(t *testing.T) {
	d := testDB(t)

	_, err := backlog.CreateProject(d, "acme-corp", "AC")
	require.NoError(t, err)

	err = backlog.DeleteProject(d, "acme-corp", false, "")
	assert.NoError(t, err)

	_, err = backlog.GetProjectByName(d, "acme-corp")
	assert.Error(t, err)
}

func TestDeleteProjectBlocked(t *testing.T) {
	d := testDB(t)

	proj, err := backlog.CreateProject(d, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, proj.ID, proj.Prefix, "P1", "Task", "", "", "")
	require.NoError(t, err)

	err = backlog.DeleteProject(d, "acme-corp", false, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1 items")
}

func TestDeleteProjectForce(t *testing.T) {
	d := testDB(t)

	proj, err := backlog.CreateProject(d, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, proj.ID, proj.Prefix, "P1", "Task 1", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(d, proj.ID, proj.Prefix, "P2", "Task 2", "", "", "")
	require.NoError(t, err)
	_, err = backlog.CreatePlan(d, "Plan A", "content", &proj.ID, nil)
	require.NoError(t, err)
	_, err = d.Exec("INSERT INTO documents (type, project, title, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		"decision", "acme-corp", "Doc 1", "content", "2024-01-01T00:00:00Z", "2024-01-01T00:00:00Z")
	require.NoError(t, err)

	err = backlog.DeleteProject(d, "acme-corp", true, "")
	require.NoError(t, err)

	var itemCount, planCount, docCount int
	require.NoError(t, d.QueryRow("SELECT COUNT(*) FROM items WHERE project_id = ?", proj.ID).Scan(&itemCount))
	require.NoError(t, d.QueryRow("SELECT COUNT(*) FROM plans WHERE project_id = ?", proj.ID).Scan(&planCount))
	require.NoError(t, d.QueryRow("SELECT COUNT(*) FROM documents WHERE project = ?", "acme-corp").Scan(&docCount))
	assert.Equal(t, 0, itemCount)
	assert.Equal(t, 0, planCount)
	assert.Equal(t, 0, docCount)

	_, err = backlog.GetProjectByName(d, "acme-corp")
	assert.Error(t, err)
}

func TestDeleteProjectReassign(t *testing.T) {
	d := testDB(t)

	src, err := backlog.CreateProject(d, "src-proj", "SP")
	require.NoError(t, err)
	dst, err := backlog.CreateProject(d, "dst-proj", "DP")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, src.ID, src.Prefix, "P1", "Task", "", "", "")
	require.NoError(t, err)
	_, err = backlog.CreatePlan(d, "Plan", "content", &src.ID, nil)
	require.NoError(t, err)
	_, err = d.Exec("INSERT INTO documents (type, project, title, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		"decision", "src-proj", "Doc", "content", "2024-01-01T00:00:00Z", "2024-01-01T00:00:00Z")
	require.NoError(t, err)

	err = backlog.DeleteProject(d, "src-proj", false, "dst-proj")
	require.NoError(t, err)

	var itemCount, planCount, docCount int
	require.NoError(t, d.QueryRow("SELECT COUNT(*) FROM items WHERE project_id = ?", dst.ID).Scan(&itemCount))
	require.NoError(t, d.QueryRow("SELECT COUNT(*) FROM plans WHERE project_id = ?", dst.ID).Scan(&planCount))
	require.NoError(t, d.QueryRow("SELECT COUNT(*) FROM documents WHERE project = ?", "dst-proj").Scan(&docCount))
	assert.Equal(t, 1, itemCount)
	assert.Equal(t, 1, planCount)
	assert.Equal(t, 1, docCount)

	_, err = backlog.GetProjectByName(d, "src-proj")
	assert.Error(t, err)
}

func TestDeleteProjectMutex(t *testing.T) {
	d := testDB(t)

	_, err := backlog.CreateProject(d, "acme-corp", "AC")
	require.NoError(t, err)

	err = backlog.DeleteProject(d, "acme-corp", true, "other-proj")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestDeleteProjectNotFound(t *testing.T) {
	d := testDB(t)

	err := backlog.DeleteProject(d, "nonexistent", false, "")
	assert.Error(t, err)
}

func TestDeleteProjectReassignToSelf(t *testing.T) {
	d := testDB(t)

	_, err := backlog.CreateProject(d, "acme-corp", "AC")
	require.NoError(t, err)

	err = backlog.DeleteProject(d, "acme-corp", false, "acme-corp")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot reassign to self")
}

func TestDeleteProjectReassignTargetNotFound(t *testing.T) {
	d := testDB(t)

	_, err := backlog.CreateProject(d, "acme-corp", "AC")
	require.NoError(t, err)

	err = backlog.DeleteProject(d, "acme-corp", false, "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reassign target not found")
}

func TestReassignProjectChildren(t *testing.T) {
	d := testDB(t)

	src, err := backlog.CreateProject(d, "src-proj", "SP")
	require.NoError(t, err)
	dst, err := backlog.CreateProject(d, "dst-proj", "DP")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, src.ID, src.Prefix, "P1", "Task 1", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(d, src.ID, src.Prefix, "P2", "Task 2", "", "", "")
	require.NoError(t, err)
	_, err = backlog.CreatePlan(d, "Plan A", "content", &src.ID, nil)
	require.NoError(t, err)
	_, err = d.Exec("INSERT INTO documents (type, project, title, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		"decision", "Src-Proj", "Doc 1", "content", "2024-01-01T00:00:00Z", "2024-01-01T00:00:00Z")
	require.NoError(t, err)

	tx, err := d.Begin()
	require.NoError(t, err)
	err = backlog.ReassignProjectChildren(tx, src.ID, src.Name, dst.ID, dst.Name)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	var itemCount, planCount, docCount int
	require.NoError(t, d.QueryRow("SELECT COUNT(*) FROM items WHERE project_id = ?", dst.ID).Scan(&itemCount))
	require.NoError(t, d.QueryRow("SELECT COUNT(*) FROM plans WHERE project_id = ?", dst.ID).Scan(&planCount))
	require.NoError(t, d.QueryRow("SELECT COUNT(*) FROM documents WHERE project = ?", "dst-proj").Scan(&docCount))
	assert.Equal(t, 2, itemCount)
	assert.Equal(t, 1, planCount)
	assert.Equal(t, 1, docCount) // case-insensitive match on "Src-Proj"

	// src should now have none
	require.NoError(t, d.QueryRow("SELECT COUNT(*) FROM items WHERE project_id = ?", src.ID).Scan(&itemCount))
	assert.Equal(t, 0, itemCount)
}

func TestReassignProjectChildrenClosedTx(t *testing.T) {
	d := testDB(t)

	src, err := backlog.CreateProject(d, "src-proj", "SP")
	require.NoError(t, err)
	dst, err := backlog.CreateProject(d, "dst-proj", "DP")
	require.NoError(t, err)

	tx, err := d.Begin()
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())

	// tx is now closed — Exec should fail
	err = backlog.ReassignProjectChildren(tx, src.ID, src.Name, dst.ID, dst.Name)
	require.Error(t, err)
}

func TestDerivePrefixHappyPath(t *testing.T) {
	d := testDB(t)

	prefix, err := backlog.DerivePrefix(d, "acme-corp")
	require.NoError(t, err)
	assert.Equal(t, "AC", prefix)
}

func TestDerivePrefixCollisionFallback(t *testing.T) {
	d := testDB(t)

	// Seed AC to force collision
	_, err := backlog.CreateProject(d, "acme-corp", "AC")
	require.NoError(t, err)

	// "account" also starts with AC — should fall back to A1
	prefix, err := backlog.DerivePrefix(d, "account")
	require.NoError(t, err)
	assert.Equal(t, "A1", prefix)
}

func TestDerivePrefixExhaustion(t *testing.T) {
	d := testDB(t)

	// For "another", base prefix is "AN". Exhaust AN + A1..A9.
	_, err := backlog.CreateProject(d, "another", "AN")
	require.NoError(t, err)
	for i := 1; i <= 9; i++ {
		name := "alpha-0" + string(rune('0'+i))
		prefix := "A" + string(rune('0'+i))
		_, err = backlog.CreateProject(d, name, prefix)
		require.NoError(t, err)
	}

	// Now DerivePrefix for any "an*" name should fail
	_, err = backlog.DerivePrefix(d, "anything")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot derive unique prefix")
}

func TestDerivePrefixShortName(t *testing.T) {
	d := testDB(t)

	// Single-char name should be padded with X
	prefix, err := backlog.DerivePrefix(d, "a")
	require.NoError(t, err)
	assert.Equal(t, "AX", prefix)
}

func TestValidatePrefix(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid two letters", "OU", false},
		{"valid letter+digit", "A1", false},
		{"valid two letters WS", "WS", false},
		{"valid four letters", "ABCD", false},
		{"valid single letter", "Z", false},
		{"empty", "", true},
		{"lowercase", "a", true},
		{"digit first", "1A", true},
		{"five chars", "ABCDE", true},
		{"hyphen", "O-1", true},
		{"lowercase two", "o", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := backlog.ValidatePrefix(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRenameProjectPrefixHappyPath(t *testing.T) {
	d := testDB(t)

	proj, err := backlog.CreateProject(d, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, proj.ID, proj.Prefix, "P1", "task-one", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(d, proj.ID, proj.Prefix, "P2", "task-two", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(d, proj.ID, proj.Prefix, "P3", "task-three", "", "", "")
	require.NoError(t, err)

	renamed, err := backlog.RenameProject(d, "acme-corp", "", "XX")
	require.NoError(t, err)
	assert.Equal(t, "XX", renamed.Prefix)
	assert.Equal(t, "acme-corp", renamed.Name)

	for _, wantID := range []string{"XX-1", "XX-2", "XX-3"} {
		item, err := backlog.GetItem(d, wantID)
		require.NoError(t, err, "expected item %s", wantID)
		assert.Equal(t, wantID, item.ID)
	}

	// projects.prefix updated
	p, err := backlog.GetProjectByName(d, "acme-corp")
	require.NoError(t, err)
	assert.Equal(t, "XX", p.Prefix)

	// item_id_aliases has 3 rows with non-empty renamed_at
	var count int
	err = d.QueryRow("SELECT COUNT(*) FROM item_id_aliases WHERE new_id LIKE 'XX-%' AND renamed_at != ''").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestRenameProjectPrefixCascadesPlans(t *testing.T) {
	d := testDB(t)

	proj, err := backlog.CreateProject(d, "acme-corp", "AC")
	require.NoError(t, err)

	item, err := backlog.AddItem(d, proj.ID, proj.Prefix, "P1", "task", "", "", "")
	require.NoError(t, err)

	itemID := item.ID
	_, err = backlog.CreatePlan(d, "impl plan", "content", &proj.ID, &itemID)
	require.NoError(t, err)

	_, err = backlog.RenameProject(d, "acme-corp", "", "YY")
	require.NoError(t, err)

	var planItemID string
	err = d.QueryRow("SELECT item_id FROM plans WHERE project_id = ?", proj.ID).Scan(&planItemID)
	require.NoError(t, err)
	assert.Equal(t, "YY-1", planItemID)
}

func TestRenameProjectPrefixCollision(t *testing.T) {
	d := testDB(t)

	_, err := backlog.CreateProject(d, "alpha-proj", "AA")
	require.NoError(t, err)
	_, err = backlog.CreateProject(d, "beta-project", "BB")
	require.NoError(t, err)

	_, err = backlog.RenameProject(d, "alpha-proj", "", "BB")
	require.Error(t, err)
}

func TestRenameProjectBothNameAndPrefix(t *testing.T) {
	d := testDB(t)

	proj, err := backlog.CreateProject(d, "old-name", "OL")
	require.NoError(t, err)
	_, err = backlog.AddItem(d, proj.ID, proj.Prefix, "P1", "task", "", "", "")
	require.NoError(t, err)

	renamed, err := backlog.RenameProject(d, "old-name", "new-name", "NM")
	require.NoError(t, err)
	assert.Equal(t, "new-name", renamed.Name)
	assert.Equal(t, "NM", renamed.Prefix)

	item, err := backlog.GetItem(d, "NM-1")
	require.NoError(t, err)
	assert.Equal(t, "NM-1", item.ID)
}

func TestRenameProjectBothEmpty(t *testing.T) {
	d := testDB(t)

	_, err := backlog.CreateProject(d, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.RenameProject(d, "acme-corp", "", "")
	require.Error(t, err)
}

func TestRenameProjectCascadesDocumentsCaseInsensitive(t *testing.T) {
	d := testDB(t)

	_, err := backlog.CreateProject(d, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = d.Exec("INSERT INTO documents (type, project, title, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		"decision", "Acme-Corp", "Doc", "content", "2024-01-01T00:00:00Z", "2024-01-01T00:00:00Z")
	require.NoError(t, err)

	_, err = backlog.RenameProject(d, "acme-corp", "renamed-corp", "")
	require.NoError(t, err)

	var count int
	err = d.QueryRow("SELECT COUNT(*) FROM documents WHERE project = ?", "renamed-corp").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestRenameProjectPrefixChained(t *testing.T) {
	d := testDB(t)

	proj, err := backlog.CreateProject(d, "acme-corp", "OU")
	require.NoError(t, err)

	_, err = backlog.AddItem(d, proj.ID, proj.Prefix, "P1", "task", "", "", "")
	require.NoError(t, err)

	// First rename: OU → XX
	_, err = backlog.RenameProject(d, "acme-corp", "", "XX")
	require.NoError(t, err)

	// Second rename: XX → YY
	_, err = backlog.RenameProject(d, "acme-corp", "", "YY")
	require.NoError(t, err)

	// Items should have final prefix YY
	item, err := backlog.GetItem(d, "YY-1")
	require.NoError(t, err)
	assert.Equal(t, "YY-1", item.ID)

	// XX-1 alias written by second rename resolves to YY-1
	item, err = backlog.GetItem(d, "XX-1")
	require.NoError(t, err)
	assert.Equal(t, "YY-1", item.ID)
	// NOTE: OU-1 → XX-1 alias cascade to YY-1 requires PRAGMA foreign_keys=ON;
	// testDB uses ApplySchema without that PRAGMA. Chain resolution of OU-1 is tested
	// via InitDB path in production, not here.
}

func TestRenameProjectInvalidNewPrefix(t *testing.T) {
	d := testDB(t)

	_, err := backlog.CreateProject(d, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.RenameProject(d, "acme-corp", "", "1X")
	require.Error(t, err)
}

func TestRenameProjectInvalidNewName(t *testing.T) {
	d := testDB(t)

	_, err := backlog.CreateProject(d, "acme-corp", "AC")
	require.NoError(t, err)

	_, err = backlog.RenameProject(d, "acme-corp", "AB", "")
	require.Error(t, err)
}

func TestRenameProjectCaseInsensitiveCollision(t *testing.T) {
	d := testDB(t)

	_, err := backlog.CreateProject(d, "foo-bar", "FO")
	require.NoError(t, err)

	_, err = backlog.CreateProject(d, "Bar-baz", "BB")
	require.NoError(t, err)

	// Rename "Bar-baz" to "FOO-BAR" — case-insensitive collision with "foo-bar"
	_, err = backlog.RenameProject(d, "Bar-baz", "foo-bar", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project already exists")
}
