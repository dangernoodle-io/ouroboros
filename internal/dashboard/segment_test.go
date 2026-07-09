package dashboard

import (
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"

	"dangernoodle.io/ouroboros/internal/roadmap"
	"dangernoodle.io/ouroboros/internal/store"
)

func newDashboardTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	require.NoError(t, store.ApplySchema(db))
	t.Cleanup(func() { db.Close() })
	return db
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v: %s", args, out)
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test-user@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "commit.gpgsign", "false")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0o644))
	runGit(t, dir, "add", "file.txt")
	runGit(t, dir, "commit", "-m", "initial commit")
	return dir
}

func TestGitSegment_Branch(t *testing.T) {
	dir := initGitRepo(t)
	db := newDashboardTestDB(t)

	frags, err := gitSegment(Context{Repo: dir}, db)
	require.NoError(t, err)
	require.Len(t, frags, 2)

	branchTile, ok := frags[0].(Tile)
	require.True(t, ok)
	assert.Equal(t, "branch", branchTile.Label)
	assert.NotEmpty(t, branchTile.Value)

	uncommittedTile, ok := frags[1].(Tile)
	require.True(t, ok)
	assert.Equal(t, "uncommitted", uncommittedTile.Label)
	assert.Equal(t, "0", uncommittedTile.Value)
}

func TestGitSegment_UncommittedChange(t *testing.T) {
	dir := initGitRepo(t)
	db := newDashboardTestDB(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("changed"), 0o644))

	frags, err := gitSegment(Context{Repo: dir}, db)
	require.NoError(t, err)
	require.Len(t, frags, 2)

	uncommittedTile, ok := frags[1].(Tile)
	require.True(t, ok)
	assert.Equal(t, "1", uncommittedTile.Value)
}

func TestGitSegment_NonGitDir(t *testing.T) {
	dir := t.TempDir()
	db := newDashboardTestDB(t)

	frags, err := gitSegment(Context{Repo: dir}, db)
	require.NoError(t, err)
	assert.Nil(t, frags)
}

func TestGitSegment_UsesCwdWhenRepoEmpty(t *testing.T) {
	dir := initGitRepo(t)
	db := newDashboardTestDB(t)

	frags, err := gitSegment(Context{Cwd: dir}, db)
	require.NoError(t, err)
	assert.Len(t, frags, 2)
}

func TestRoadmapSegment_EmptyProject(t *testing.T) {
	db := newDashboardTestDB(t)

	frags, err := roadmapSegment(Context{Project: ""}, db)
	require.NoError(t, err)
	assert.Nil(t, frags)
}

func TestRoadmapSegment_NoRoadmap(t *testing.T) {
	db := newDashboardTestDB(t)

	frags, err := roadmapSegment(Context{Project: "no-such-project"}, db)
	require.NoError(t, err)
	assert.Nil(t, frags)
}

func TestRoadmapSegment_SectionCounts(t *testing.T) {
	db := newDashboardTestDB(t)

	require.NoError(t, roadmap.Mutate(db, "acme-corp", func(rm *roadmap.Roadmap) error {
		_, err := roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "task one"})
		if err != nil {
			return err
		}
		_, err = roadmap.AddItem(rm, roadmap.SectionNow, roadmap.Item{Title: "task two"})
		if err != nil {
			return err
		}
		_, err = roadmap.AddItem(rm, roadmap.SectionNext, roadmap.Item{Title: "task three"})
		return err
	}))

	frags, err := roadmapSegment(Context{Project: "acme-corp"}, db)
	require.NoError(t, err)
	require.Len(t, frags, 2)

	byLabel := map[string]string{}
	for _, f := range frags {
		tile, ok := f.(Tile)
		require.True(t, ok)
		byLabel[tile.Label] = tile.Value
	}
	assert.Equal(t, "2", byLabel["now"])
	assert.Equal(t, "1", byLabel["next"])
}

func TestBuiltinAndBuiltinNames(t *testing.T) {
	_, ok := Builtin("git")
	assert.True(t, ok)
	_, ok = Builtin("roadmap")
	assert.True(t, ok)
	_, ok = Builtin("nonexistent")
	assert.False(t, ok)

	assert.Equal(t, []string{"git", "roadmap"}, BuiltinNames())
}
