package dashboard

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"

	"dangernoodle.io/ouroboros/internal/backlog"
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

	frags, err := gitSegment(context.Background(), Context{Repo: dir}, db)
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

	frags, err := gitSegment(context.Background(), Context{Repo: dir}, db)
	require.NoError(t, err)
	require.Len(t, frags, 2)

	uncommittedTile, ok := frags[1].(Tile)
	require.True(t, ok)
	assert.Equal(t, "1", uncommittedTile.Value)
}

func TestGitSegment_Worktrees(t *testing.T) {
	dir := initGitRepo(t)
	db := newDashboardTestDB(t)

	wtDir := filepath.Join(t.TempDir(), "wt2")
	runGit(t, dir, "worktree", "add", wtDir, "-b", "wt2")

	frags, err := gitSegment(context.Background(), Context{Repo: dir}, db)
	require.NoError(t, err)
	require.Len(t, frags, 3)

	group, ok := frags[2].(Group)
	require.True(t, ok)
	assert.Equal(t, "git", group.Section)
	assert.Equal(t, "worktrees", group.Title)
	require.Len(t, group.Cards, 2)

	mainCard := group.Cards[0]
	assert.Equal(t, filepath.Base(dir), mainCard.Title)
	assert.Equal(t, "main", mainCard.State)

	wtCard := group.Cards[1]
	assert.Equal(t, "wt2", wtCard.Title)
	assert.Equal(t, "wt2", wtCard.Desc)
	assert.Empty(t, wtCard.State)
}

func TestGitSegment_DetachedWorktree(t *testing.T) {
	dir := initGitRepo(t)
	db := newDashboardTestDB(t)

	wtDir := filepath.Join(t.TempDir(), "wt-detached")
	runGit(t, dir, "worktree", "add", "--detach", wtDir, "HEAD")

	frags, err := gitSegment(context.Background(), Context{Repo: dir}, db)
	require.NoError(t, err)
	require.Len(t, frags, 3)

	group, ok := frags[2].(Group)
	require.True(t, ok)
	require.Len(t, group.Cards, 2)

	mainCard := group.Cards[0]
	assert.Equal(t, "main", mainCard.State)

	detachedCard := group.Cards[1]
	assert.Equal(t, "wt-detached", detachedCard.Title)
	assert.Equal(t, "detached", detachedCard.Desc)
	assert.Empty(t, detachedCard.State)
}

func TestGitSegment_NoLinkedWorktrees(t *testing.T) {
	dir := initGitRepo(t)
	db := newDashboardTestDB(t)

	frags, err := gitSegment(context.Background(), Context{Repo: dir}, db)
	require.NoError(t, err)
	require.Len(t, frags, 2)

	for _, f := range frags {
		_, isGroup := f.(Group)
		assert.False(t, isGroup, "expected no worktrees group with only one worktree")
	}
}

func TestGitSegment_NonGitDir(t *testing.T) {
	dir := t.TempDir()
	db := newDashboardTestDB(t)

	frags, err := gitSegment(context.Background(), Context{Repo: dir}, db)
	require.NoError(t, err)
	assert.Nil(t, frags)
}

func TestGitSegment_UsesCwdWhenRepoEmpty(t *testing.T) {
	dir := initGitRepo(t)
	db := newDashboardTestDB(t)

	frags, err := gitSegment(context.Background(), Context{Cwd: dir}, db)
	require.NoError(t, err)
	assert.Len(t, frags, 2)
}

func TestRoadmapSegment_EmptyProject(t *testing.T) {
	db := newDashboardTestDB(t)

	frags, err := roadmapSegment(context.Background(), Context{Project: ""}, db)
	require.NoError(t, err)
	assert.Nil(t, frags)
}

func TestRoadmapSegment_NoRoadmap(t *testing.T) {
	db := newDashboardTestDB(t)

	frags, err := roadmapSegment(context.Background(), Context{Project: "no-such-project"}, db)
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

	frags, err := roadmapSegment(context.Background(), Context{Project: "acme-corp"}, db)
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
	_, ok = Builtin("github")
	assert.True(t, ok)
	_, ok = Builtin("tickets")
	assert.True(t, ok)
	_, ok = Builtin("kb")
	assert.True(t, ok)
	_, ok = Builtin("nonexistent")
	assert.False(t, ok)

	assert.Equal(t, []string{"git", "github", "kb", "roadmap", "tickets"}, BuiltinNames())
}

// stubGh writes an executable "gh" script to a temp dir and prepends that
// dir to PATH for the duration of the test, so githubSegment shells out to
// the stub instead of the real gh CLI (no network, no auth).
func stubGh(t *testing.T, script string) {
	t.Helper()
	bin := t.TempDir()
	path := filepath.Join(bin, "gh")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755)) //nolint:gosec // test fixture, executable by design
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
}

const stubGhPRListJSON = `#!/bin/sh
cat <<'EOF'
[
  {"number": 12, "title": "add widget support", "author": {"login": "test-user"}},
  {"number": 7, "title": "fix flaky test", "author": {"login": "test-user-two"}}
]
EOF
`

func TestGitHubSegment_OpenPRs(t *testing.T) {
	stubGh(t, stubGhPRListJSON)

	dir := initGitRepo(t)
	runGit(t, dir, "remote", "add", "origin", "https://github.com/acme-corp/widget.git")
	db := newDashboardTestDB(t)

	frags, err := githubSegment(context.Background(), Context{Repo: dir}, db)
	require.NoError(t, err)
	require.Len(t, frags, 2)

	tile, ok := frags[0].(Tile)
	require.True(t, ok)
	assert.Equal(t, "github", tile.Section)
	assert.Equal(t, "open PRs", tile.Label)
	assert.Equal(t, "2", tile.Value)

	group, ok := frags[1].(Group)
	require.True(t, ok)
	assert.Equal(t, "github", group.Section)
	assert.Equal(t, "pull requests", group.Title)
	require.Len(t, group.Cards, 2)
	assert.Equal(t, "#12 add widget support", group.Cards[0].Title)
	assert.Equal(t, "@test-user", group.Cards[0].Desc)
	assert.Equal(t, "#7 fix flaky test", group.Cards[1].Title)
	assert.Equal(t, "@test-user-two", group.Cards[1].Desc)
}

func TestGitHubSegment_NoOpenPRs(t *testing.T) {
	stubGh(t, "#!/bin/sh\necho '[]'\n")

	dir := initGitRepo(t)
	runGit(t, dir, "remote", "add", "origin", "https://github.com/acme-corp/widget.git")
	db := newDashboardTestDB(t)

	frags, err := githubSegment(context.Background(), Context{Repo: dir}, db)
	require.NoError(t, err)
	require.Len(t, frags, 1)

	tile, ok := frags[0].(Tile)
	require.True(t, ok)
	assert.Equal(t, "0", tile.Value)
}

func TestGitHubSegment_NoOriginRemote(t *testing.T) {
	dir := initGitRepo(t)
	db := newDashboardTestDB(t)

	frags, err := githubSegment(context.Background(), Context{Repo: dir}, db)
	require.NoError(t, err)
	assert.Nil(t, frags)
}

func TestGitHubSegment_NonGitHubOrigin(t *testing.T) {
	dir := initGitRepo(t)
	runGit(t, dir, "remote", "add", "origin", "git@gitlab.com:acme-corp/widget.git")
	db := newDashboardTestDB(t)

	frags, err := githubSegment(context.Background(), Context{Repo: dir}, db)
	require.NoError(t, err)
	assert.Nil(t, frags)
}

func TestGitHubSegment_GhFailure(t *testing.T) {
	stubGh(t, "#!/bin/sh\nexit 1\n")

	dir := initGitRepo(t)
	runGit(t, dir, "remote", "add", "origin", "https://github.com/acme-corp/widget.git")
	db := newDashboardTestDB(t)

	frags, err := githubSegment(context.Background(), Context{Repo: dir}, db)
	require.NoError(t, err)
	assert.Nil(t, frags)
}

func TestGitHubSegment_NonGitDir(t *testing.T) {
	dir := t.TempDir()
	db := newDashboardTestDB(t)

	frags, err := githubSegment(context.Background(), Context{Repo: dir}, db)
	require.NoError(t, err)
	assert.Nil(t, frags)
}

func TestGitHubSegment_MalformedJSON(t *testing.T) {
	stubGh(t, "#!/bin/sh\necho 'not-json'\n")

	dir := initGitRepo(t)
	runGit(t, dir, "remote", "add", "origin", "https://github.com/acme-corp/widget.git")
	db := newDashboardTestDB(t)

	frags, err := githubSegment(context.Background(), Context{Repo: dir}, db)
	require.NoError(t, err)
	assert.Nil(t, frags)
}

func TestGitHubSegment_ResolvesDirFromCwd(t *testing.T) {
	stubGh(t, stubGhPRListJSON)

	dir := initGitRepo(t)
	runGit(t, dir, "remote", "add", "origin", "https://github.com/acme-corp/widget.git")
	db := newDashboardTestDB(t)

	t.Chdir(dir)

	frags, err := githubSegment(context.Background(), Context{}, db)
	require.NoError(t, err)
	require.Len(t, frags, 2)

	tile, ok := frags[0].(Tile)
	require.True(t, ok)
	assert.Equal(t, "2", tile.Value)

	group, ok := frags[1].(Group)
	require.True(t, ok)
	require.Len(t, group.Cards, 2)
}

func TestTicketsSegment_EmptyProject(t *testing.T) {
	db := newDashboardTestDB(t)

	frags, err := ticketsSegment(context.Background(), Context{Project: ""}, db)
	require.NoError(t, err)
	assert.Nil(t, frags)
}

func TestTicketsSegment_UnknownProject(t *testing.T) {
	db := newDashboardTestDB(t)

	frags, err := ticketsSegment(context.Background(), Context{Project: "no-such-project"}, db)
	require.NoError(t, err)
	assert.Nil(t, frags)
}

func TestTicketsSegment_NoOpenItems(t *testing.T) {
	db := newDashboardTestDB(t)
	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	frags, err := ticketsSegment(context.Background(), Context{Project: "acme-corp"}, db)
	require.NoError(t, err)
	require.Len(t, frags, 1)

	tile, ok := frags[0].(Tile)
	require.True(t, ok)
	assert.Equal(t, "tickets", tile.Section)
	assert.Equal(t, "open", tile.Label)
	assert.Equal(t, "0", tile.Value)
}

func TestTicketsSegment_RecentTickets(t *testing.T) {
	db := newDashboardTestDB(t)
	p, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	// Seed 6 open items with distinct, ascending created times, plus one
	// done item that must never surface.
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var ids []string
	for i := 0; i < 6; i++ {
		item, err := backlog.AddItem(db, p.ID, "AC", "P2", fmt.Sprintf("open task %d", i), "", "", "", "")
		require.NoError(t, err)
		ids = append(ids, item.ID)
		created := base.Add(time.Duration(i) * time.Hour).Format(time.RFC3339)
		_, err = db.Exec("UPDATE items SET created = ? WHERE id = ?", created, item.ID)
		require.NoError(t, err)
	}

	doneItem, err := backlog.AddItem(db, p.ID, "AC", "P1", "done task", "", "", "", "")
	require.NoError(t, err)
	require.NoError(t, backlog.MarkDone(db, doneItem.ID))

	frags, err := ticketsSegment(context.Background(), Context{Project: "acme-corp"}, db)
	require.NoError(t, err)
	require.Len(t, frags, 2)

	tile, ok := frags[0].(Tile)
	require.True(t, ok)
	assert.Equal(t, "tickets", tile.Section)
	assert.Equal(t, "open", tile.Label)
	assert.Equal(t, "6", tile.Value)

	group, ok := frags[1].(Group)
	require.True(t, ok)
	assert.Equal(t, "tickets", group.Section)
	assert.Equal(t, "recent tickets", group.Title)
	require.Len(t, group.Cards, 5)

	// Newest-first: item index 5 (last created) down to index 1.
	for rank, wantIdx := range []int{5, 4, 3, 2, 1} {
		wantID := ids[wantIdx]
		assert.Equal(t, wantID+" open task "+strconv.Itoa(wantIdx), group.Cards[rank].Title)
		assert.Equal(t, "P2", group.Cards[rank].Desc)
	}
}

func TestTicketsSegment_ListItemsError(t *testing.T) {
	db := newDashboardTestDB(t)
	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	// Break the backlog read after the project lookup succeeds: dropping
	// the items table forces ListItems to error.
	_, err = db.Exec("DROP TABLE items")
	require.NoError(t, err)

	frags, err := ticketsSegment(context.Background(), Context{Project: "acme-corp"}, db)
	require.NoError(t, err)
	assert.Nil(t, frags)
}

func TestKBSegment_EmptyProject(t *testing.T) {
	db := newDashboardTestDB(t)

	frags, err := kbSegment(context.Background(), Context{Project: ""}, db)
	require.NoError(t, err)
	assert.Nil(t, frags)
}

func TestKBSegment_UnknownProject(t *testing.T) {
	db := newDashboardTestDB(t)

	frags, err := kbSegment(context.Background(), Context{Project: "no-such-project"}, db)
	require.NoError(t, err)
	assert.Nil(t, frags)
}

func TestKBSegment_EntryCount(t *testing.T) {
	db := newDashboardTestDB(t)
	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	docs := []store.Document{
		{Type: "decision", Project: "acme-corp", Title: "api-design", Content: "RESTful API"},
		{Type: "fact", Project: "acme-corp", Title: "endpoint", Content: "api.example.com"},
		{Type: "note", Project: "acme-corp", Title: "meeting", Content: "Q2 planning"},
		{Type: "fact", Project: "other-corp", Title: "other-fact", Content: "unrelated"},
	}
	for _, doc := range docs {
		_, err := store.UpsertDocument(db, doc)
		require.NoError(t, err)
	}

	frags, err := kbSegment(context.Background(), Context{Project: "acme-corp"}, db)
	require.NoError(t, err)
	require.Len(t, frags, 1)

	tile, ok := frags[0].(Tile)
	require.True(t, ok)
	assert.Equal(t, "kb", tile.Section)
	assert.Equal(t, "entries", tile.Label)
	assert.Equal(t, "3", tile.Value)
}

func TestKBSegment_CountDocumentsError(t *testing.T) {
	db := newDashboardTestDB(t)
	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	// Break the KB read after the project lookup succeeds: dropping the
	// documents table forces CountDocumentsByType to error.
	_, err = db.Exec("DROP TABLE documents")
	require.NoError(t, err)

	frags, err := kbSegment(context.Background(), Context{Project: "acme-corp"}, db)
	require.NoError(t, err)
	assert.Nil(t, frags)
}

func TestParseGitHubRepo(t *testing.T) {
	cases := []struct {
		name   string
		remote string
		want   string
		wantOk bool
	}{
		{"https", "https://github.com/acme-corp/widget.git", "acme-corp/widget", true},
		{"https-no-suffix", "https://github.com/acme-corp/widget", "acme-corp/widget", true},
		{"ssh-scp", "git@github.com:acme-corp/widget.git", "acme-corp/widget", true},
		{"ssh-url", "ssh://git@github.com/acme-corp/widget.git", "acme-corp/widget", true},
		{"dotted-repo-https", "https://github.com/dangernoodle-io/notarizedbyape.com.git", "dangernoodle-io/notarizedbyape.com", true},
		{"dotted-repo-ssh-scp", "git@github.com:dangernoodle-io/notarizedbyape.com.git", "dangernoodle-io/notarizedbyape.com", true},
		{"non-github", "git@gitlab.com:acme-corp/widget.git", "", false},
		{"garbage", "not a url at all", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseGitHubRepo(tc.remote)
			assert.Equal(t, tc.wantOk, ok)
			if tc.wantOk {
				assert.Equal(t, tc.want, got)
			}
		})
	}
}
