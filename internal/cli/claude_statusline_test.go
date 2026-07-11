package cli

import (
	"bytes"
	"context"
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

func TestOuroborosStatuslineProvider_NoCwdFallsBackToGetwd(t *testing.T) {
	isolateStatuslineDB(t)

	// No payload.Cwd and no .git in the process cwd (a Go test's working
	// dir is the package source dir, which is inside the ouroboros repo —
	// so this actually resolves to "ouroboros" via os.Getwd's git-root
	// walk-up). Assert only that it doesn't error and doesn't panic.
	_, err := ouroborosStatuslineProvider().Statusline(context.Background(), statusline.Payload{}, "")
	require.NoError(t, err)
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
	_, err = store.UpsertDocument(db, store.Document{Type: "decision", Project: "ouroboros", Title: "Use SQLite"})
	require.NoError(t, err)
	require.NoError(t, db.Close())

	provider := claudeProvider()
	cmds := provider.Commands()
	require.Len(t, cmds, 1)
	assert.Equal(t, "claude", cmds[0].Use)

	statuslineCmd, _, err := cmds[0].Find([]string{"statusline"})
	require.NoError(t, err)

	var out bytes.Buffer
	statuslineCmd.SetOut(&out)
	statuslineCmd.SetIn(strings.NewReader(`{"session_id":"abc","cwd":"` + dir + `"}`))

	require.NoError(t, statuslineCmd.RunE(statuslineCmd, nil))
	assert.Contains(t, out.String(), "KB 1")
	assert.Contains(t, out.String(), "[ouroboros]")
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
