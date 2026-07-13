package cli

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dangernoodle-io/mcpkit/host/claudecode/statusline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/store"
)

// isolateStatuslineDB points PROJECT_KB_PATH at a fresh temp SQLite file
// and HOME at a fresh temp dir (so a real developer bootstrap.json never
// overrides the test's db_path), returning the db path.
func isolateStatuslineDB(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("QM_DB_PATH", "")
	dbPath := filepath.Join(t.TempDir(), "kb.db")
	t.Setenv("PROJECT_KB_PATH", dbPath)
	return dbPath
}

// gitDir creates a temp dir containing a .git entry (so projectFromPath
// resolves it) and returns its path.
func gitDir(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	return dir
}

// marketplaceHubDir creates a temp dir shaped like the dangernoodle-marketplace
// hub repo (.git + .claude-plugin/marketplace.json) and returns its path.
func marketplaceHubDir(t *testing.T, name string) string {
	t.Helper()
	dir := gitDir(t, name)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude-plugin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude-plugin", "marketplace.json"), []byte("{}"), 0o644))
	return dir
}

// openStatuslineDB isolates a fresh DB (see isolateStatuslineDB) and opens
// it, returning the *sql.DB for direct project registration in
// statuslineProject unit tests.
func openStatuslineDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := isolateStatuslineDB(t)
	db, err := store.InitDB(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestOuroborosStatuslineProvider_Empty(t *testing.T) {
	isolateStatuslineDB(t)

	segs, err := ouroborosStatuslineProvider().Statusline(context.Background(), statusline.Payload{}, "")
	require.NoError(t, err)
	assert.Nil(t, segs)
}

func TestOuroborosStatuslineProvider_KBOnly(t *testing.T) {
	dbPath := isolateStatuslineDB(t)
	dir := gitDir(t, "ouroboros")

	db, err := store.InitDB(dbPath)
	require.NoError(t, err)
	_, err = backlog.CreateProject(db, "ouroboros", "OUR")
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{Type: "decision", Project: "ouroboros", Title: "Use SQLite"})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{Type: "decision", Project: "ouroboros", Title: "Use PostgreSQL"})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{Type: "fact", Project: "ouroboros", Title: "Database location"})
	require.NoError(t, err)
	require.NoError(t, db.Close())

	segs, err := ouroborosStatuslineProvider().Statusline(context.Background(), statusline.Payload{Cwd: dir}, "")
	require.NoError(t, err)

	line := statusline.Render(segs, statusline.RenderOptions{Plain: true})
	assert.Contains(t, line, "[ouroboros]")
	assert.Contains(t, line, "KB 3")
	assert.Contains(t, line, "2D")
	assert.Contains(t, line, "1F")
	assert.Contains(t, line, "BL 0 open")
}

func TestOuroborosStatuslineProvider_BacklogOnly(t *testing.T) {
	dbPath := isolateStatuslineDB(t)
	dir := gitDir(t, "acme-corp")

	db, err := store.InitDB(dbPath)
	require.NoError(t, err)
	p, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, p.ID, "AC", "P0", "Critical task", "", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, p.ID, "AC", "P1", "High priority task", "", "", "", "")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	segs, err := ouroborosStatuslineProvider().Statusline(context.Background(), statusline.Payload{Cwd: dir}, "")
	require.NoError(t, err)

	line := statusline.Render(segs, statusline.RenderOptions{Plain: true})
	assert.Contains(t, line, "BL 2 open")
	assert.Contains(t, line, "1×P0")
	assert.Contains(t, line, "1×P1")
	assert.Contains(t, line, "KB 0")
}

func TestOuroborosStatuslineProvider_Full(t *testing.T) {
	dbPath := isolateStatuslineDB(t)
	dir := gitDir(t, "ouroboros")

	db, err := store.InitDB(dbPath)
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{Type: "decision", Project: "ouroboros", Title: "Use SQLite"})
	require.NoError(t, err)
	p, err := backlog.CreateProject(db, "ouroboros", "OUR")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, p.ID, "OUR", "P2", "Task 1", "", "", "", "")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	segs, err := ouroborosStatuslineProvider().Statusline(context.Background(), statusline.Payload{Cwd: dir}, "")
	require.NoError(t, err)

	line := statusline.Render(segs, statusline.RenderOptions{Plain: true})
	assert.Equal(t, "ouroboros: [ouroboros] KB 1 (1D) | BL 1 open (1×P2)", line)
}

// TestOuroborosStatuslineProvider_NoCwdAggregates proves an empty
// payload.Cwd never falls back to os.Getwd — it always resolves to the
// aggregate ("entire status") view, summing counts across every project
// (OU-311: restores the fallback regressed by OU-272).
func TestOuroborosStatuslineProvider_NoCwdAggregates(t *testing.T) {
	dbPath := isolateStatuslineDB(t)

	db, err := store.InitDB(dbPath)
	require.NoError(t, err)
	_, err = backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, 1, "AC", "P1", "Some task", "", "", "", "")
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{Type: "decision", Project: "acme-corp", Title: "Use SQLite"})
	require.NoError(t, err)
	require.NoError(t, db.Close())

	segs, err := ouroborosStatuslineProvider().Statusline(context.Background(), statusline.Payload{}, "")
	require.NoError(t, err)

	line := statusline.Render(segs, statusline.RenderOptions{Plain: true})
	assert.NotContains(t, line, "[", "empty cwd must aggregate, never label a project")
	assert.Contains(t, line, "KB 1")
	assert.Contains(t, line, "BL 1 open")
}

// projectName returns p.Name, or "" for nil (aggregate) — mirrors
// buildStatuslineSegments's own nil-means-aggregate convention, for
// asserting statuslineProject's *backlog.Project return in tests.
func projectName(p *backlog.Project) string {
	if p == nil {
		return ""
	}
	return p.Name
}

// TestStatuslineProject_EmptyCwdAggregates covers spec case 1: an empty
// payload.Cwd resolves to nil (aggregate), never falling back to os.Getwd.
func TestStatuslineProject_EmptyCwdAggregates(t *testing.T) {
	db := openStatuslineDB(t)
	assert.Nil(t, statuslineProject(db, statusline.Payload{}))
}

// TestStatuslineProject_MarketplaceHubAggregates covers spec case 2: the
// marketplace hub repo never resolves to a project, even when a project
// sharing its basename is registered (OU-283's guard).
func TestStatuslineProject_MarketplaceHubAggregates(t *testing.T) {
	db := openStatuslineDB(t)
	dir := marketplaceHubDir(t, "dangernoodle-marketplace")
	_, err := backlog.CreateProject(db, "dangernoodle-marketplace", "DNM")
	require.NoError(t, err)

	assert.Nil(t, statuslineProject(db, statusline.Payload{Cwd: dir}))
}

// TestStatuslineProject_RegisteredGitRepoResolves covers spec case 3: a
// cwd inside a registered project's repo resolves to that project.
func TestStatuslineProject_RegisteredGitRepoResolves(t *testing.T) {
	db := openStatuslineDB(t)
	dir := gitDir(t, "acme-corp")
	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	assert.Equal(t, "acme-corp", projectName(statuslineProject(db, statusline.Payload{Cwd: dir})))
}

// TestStatuslineProject_UnregisteredGitRepoAggregates covers spec case 4:
// a cwd inside a repo whose basename is NOT a registered project
// aggregates rather than resolving to that basename. A sibling project IS
// registered, so the candidate list is non-empty and rejection is a
// genuine per-name mismatch, not an empty-DB coincidence.
func TestStatuslineProject_UnregisteredGitRepoAggregates(t *testing.T) {
	db := openStatuslineDB(t)
	_, err := backlog.CreateProject(db, "some-registered-sibling", "SRS")
	require.NoError(t, err)
	dir := gitDir(t, "some-other-repo")

	assert.Nil(t, statuslineProject(db, statusline.Payload{Cwd: dir}))
}

// TestStatuslineProject_WorkspaceRootAggregates covers spec case 5: cwd ==
// Workspace.ProjectDir (the workspace root itself, no subproject segment)
// aggregates.
func TestStatuslineProject_WorkspaceRootAggregates(t *testing.T) {
	db := openStatuslineDB(t)
	pd := t.TempDir()

	payload := statusline.Payload{Cwd: pd, Workspace: statusline.Workspace{ProjectDir: pd}}
	assert.Nil(t, statuslineProject(db, payload))
}

// TestStatuslineProject_RegisteredSubprojectResolves covers spec case 6:
// a single-.claude workspace where cwd is <project_dir>/<subproj> and
// subproj is registered resolves to subproj.
func TestStatuslineProject_RegisteredSubprojectResolves(t *testing.T) {
	db := openStatuslineDB(t)
	pd := t.TempDir()
	subDir := filepath.Join(pd, "breadboard")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	_, err := backlog.CreateProject(db, "breadboard", "BB")
	require.NoError(t, err)

	payload := statusline.Payload{Cwd: subDir, Workspace: statusline.Workspace{ProjectDir: pd}}
	assert.Equal(t, "breadboard", projectName(statuslineProject(db, payload)))
}

// TestStatuslineProject_UnregisteredSubprojectAggregates covers spec case
// 7: cwd is <project_dir>/<subproj> but subproj is NOT registered — the
// mixed-registration case aggregates. A sibling project IS registered, so
// the candidate list is non-empty and rejection is a genuine per-name
// mismatch, not an empty-DB coincidence.
func TestStatuslineProject_UnregisteredSubprojectAggregates(t *testing.T) {
	db := openStatuslineDB(t)
	_, err := backlog.CreateProject(db, "some-registered-sibling", "SRS")
	require.NoError(t, err)
	pd := t.TempDir()
	subDir := filepath.Join(pd, "unregistered-subproj")
	require.NoError(t, os.MkdirAll(subDir, 0o755))

	payload := statusline.Payload{Cwd: subDir, Workspace: statusline.Workspace{ProjectDir: pd}}
	assert.Nil(t, statuslineProject(db, payload))
}

// TestStatuslineProject_SubprojectWinsOverGitRootBasename covers spec case
// 8: when both a registered project_dir-subproject AND a registered
// git-root basename apply (and differ), the project_dir subproject
// candidate wins (tried first). pd itself is the git root (outer repo);
// cwd is a subdirectory under it with no .git of its own, so the git-root
// candidate is pd's basename while the workspace candidate is the
// subproject segment.
func TestStatuslineProject_SubprojectWinsOverGitRootBasename(t *testing.T) {
	db := openStatuslineDB(t)
	pd := filepath.Join(t.TempDir(), "outer-repo")
	require.NoError(t, os.MkdirAll(filepath.Join(pd, ".git"), 0o755))
	subDir := filepath.Join(pd, "subproj-name")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	_, err := backlog.CreateProject(db, "subproj-name", "SP")
	require.NoError(t, err)
	_, err = backlog.CreateProject(db, "outer-repo", "OR")
	require.NoError(t, err)

	payload := statusline.Payload{Cwd: subDir, Workspace: statusline.Workspace{ProjectDir: pd}}
	assert.Equal(t, "subproj-name", projectName(statuslineProject(db, payload)))
}

func TestTypeAbbrev(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"decision", "D"},
		{"fact", "F"},
		{"note", "N"},
		{"plan", "P"},
		{"relation", "R"},
		{"", "?"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, typeAbbrev(tt.input))
		})
	}
}

func TestPrioritySegment(t *testing.T) {
	tests := []struct {
		priority string
		wantText string
		wantCol  string
		wantDim  bool
	}{
		{"P0", "2×P0", "1", false},
		{"P1", "3×P1", "3", false},
		{"P2", "4×P2", "6", false},
		{"P3", "1×P3", "", true},
		{"P6", "1×P6", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.priority, func(t *testing.T) {
			count := 1
			switch tt.priority {
			case "P0":
				count = 2
			case "P1":
				count = 3
			case "P2":
				count = 4
			}
			seg := prioritySegment(backlog.PriorityCount{Priority: tt.priority, Count: count})
			assert.Equal(t, tt.wantText, seg.Text)
			assert.Equal(t, tt.wantCol, seg.Color)
			assert.Equal(t, tt.wantDim, seg.Dim)
		})
	}
}

func TestBuildStatuslineSegments_ProjectBeforeKB(t *testing.T) {
	segs := buildStatuslineSegments(
		"project-a", 2, []store.TypeCount{{Type: "decision", Count: 2}},
		1, []backlog.PriorityCount{{Priority: "P1", Count: 1}},
	)

	line := statusline.Render(segs, statusline.RenderOptions{Plain: true})
	assert.True(t, strings.Index(line, "[project-a]") < strings.Index(line, "KB"), "project should appear before KB")
}

func TestBuildStatuslineSegments_NoProjectBracketWhenEmpty(t *testing.T) {
	segs := buildStatuslineSegments(
		"", 2, []store.TypeCount{{Type: "decision", Count: 2}},
		1, []backlog.PriorityCount{{Priority: "P1", Count: 1}},
	)

	line := statusline.Render(segs, statusline.RenderOptions{Plain: true})
	assert.True(t, strings.HasPrefix(line, "ouroboros: KB"), "without project, should have 'ouroboros: KB' prefix")
	assert.NotContains(t, line, "[")
}

// TestClaudeStatusline_WireDecodesStdinAndRenders exercises the full
// mcpkit seam end to end: build the provider's command tree, locate
// `statusline`, feed it real stdin JSON, and confirm the rendered line.
func TestClaudeStatusline_WireDecodesStdinAndRenders(t *testing.T) {
	dbPath := isolateStatuslineDB(t)
	dir := gitDir(t, "ouroboros")

	db, err := store.InitDB(dbPath)
	require.NoError(t, err)
	p, err := backlog.CreateProject(db, "ouroboros", "OUR")
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{Type: "decision", Project: "ouroboros", Title: "Use SQLite"})
	require.NoError(t, err)
	_, err = backlog.AddItem(db, p.ID, "OUR", "P2", "Task 1", "", "", "", "")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	provider := claudeProvider()
	mounts := provider.Mounts()
	require.Len(t, mounts, 1)
	assert.Equal(t, "claude", mounts[0].Cmd.Use)

	statuslineCmd, _, err := mounts[0].Cmd.Find([]string{"statusline"})
	require.NoError(t, err)

	var out bytes.Buffer
	statuslineCmd.SetOut(&out)
	statuslineCmd.SetIn(strings.NewReader(`{"session_id":"abc","cwd":"` + dir + `"}`))

	require.NoError(t, statuslineCmd.RunE(statuslineCmd, nil))
	assert.Contains(t, out.String(), "KB 1")
	assert.Contains(t, out.String(), "[ouroboros]")
	// OU-314: the wired command must force the ANSI profile so priority
	// color escapes survive non-TTY (pipe) stdout — the default termenv
	// resolution would strip these to plain text (Ascii profile).
	assert.Contains(t, out.String(), "\x1b[", "wired statusline must emit ANSI escapes (WithForceProfile)")
	assert.Contains(t, out.String(), "\x1b[36m", "P2 priority segment must render cyan (color \"6\")")
}

// TestClaudeStatusline_PlainFlagNoEscapes proves --plain renders no ANSI
// escapes even with colored priority segments present.
func TestClaudeStatusline_PlainFlagNoEscapes(t *testing.T) {
	dbPath := isolateStatuslineDB(t)
	dir := gitDir(t, "acme-corp")

	db, err := store.InitDB(dbPath)
	require.NoError(t, err)
	p, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, p.ID, "AC", "P0", "Critical bug", "", "", "", "")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	statuslineCmd := statusline.Command(ouroborosStatuslineProvider())
	require.NoError(t, statuslineCmd.Flags().Set("plain", "true"))

	var out bytes.Buffer
	statuslineCmd.SetOut(&out)
	statuslineCmd.SetIn(strings.NewReader(`{"cwd":"` + dir + `"}`))

	require.NoError(t, statuslineCmd.RunE(statuslineCmd, nil))
	assert.NotContains(t, out.String(), "\033[", "plain output must contain no ANSI escapes")
	assert.Contains(t, out.String(), "BL 1 open")
}

// TestClaudeStatusline_EmptyDBYieldsNoOutput proves an empty KB+backlog
// renders nothing, matching the old print-nothing behavior.
func TestClaudeStatusline_EmptyDBYieldsNoOutput(t *testing.T) {
	isolateStatuslineDB(t)
	dir := gitDir(t, "empty-project")

	statuslineCmd := statusline.Command(ouroborosStatuslineProvider())

	var out bytes.Buffer
	statuslineCmd.SetOut(&out)
	statuslineCmd.SetIn(strings.NewReader(`{"cwd":"` + dir + `"}`))

	require.NoError(t, statuslineCmd.RunE(statuslineCmd, nil))
	assert.Empty(t, out.String())
}

// TestOuroborosTopLevelStatuslineCommand_Removed pins that the old
// top-level `statusline` command tree no longer exists on rootCmd — the
// migration target for OU-272.
func TestOuroborosTopLevelStatuslineCommand_Removed(t *testing.T) {
	_, _, err := rootCmd.Find([]string{"statusline"})
	assert.Error(t, err)
}

// TestRootCmd_ClaudeStatuslineMounted confirms `ouroboros claude
// statusline` is reachable from the real rootCmd (the wiring in root.go),
// not just from a freshly-built provider.
func TestRootCmd_ClaudeStatuslineMounted(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"claude", "statusline"})
	require.NoError(t, err)
	assert.Equal(t, "statusline", cmd.Use)
}
