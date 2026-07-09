package cli

import (
	"bytes"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/dashboard"
)

func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v: %s", args, out)
}

func setupDashboardDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "dashboard.db")
	t.Setenv("PROJECT_KB_PATH", path)
	return path
}

// ── dashboard segment ────────────────────────────────────────────────────────

func TestDashboardSegmentCmd_UnknownBuiltin(t *testing.T) {
	setupDashboardDB(t)

	dashboardSegmentCmd.SetIn(strings.NewReader(""))
	dashboardSegmentCmd.SetOut(&bytes.Buffer{})
	err := dashboardSegmentCmd.RunE(dashboardSegmentCmd, []string{"nonexistent"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown builtin")
}

func TestDashboardSegmentCmd_EmptyProjectRoadmap(t *testing.T) {
	setupDashboardDB(t)

	var buf bytes.Buffer
	dashboardSegmentCmd.SetIn(strings.NewReader(`{"schema":1,"now":"2026-07-08T00:00:00Z"}`))
	dashboardSegmentCmd.SetOut(&buf)
	err := dashboardSegmentCmd.RunE(dashboardSegmentCmd, []string{"roadmap"})
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestDashboardSegmentCmd_EmptyStdinAutoDetect(t *testing.T) {
	setupDashboardDB(t)

	var buf bytes.Buffer
	dashboardSegmentCmd.SetIn(strings.NewReader(""))
	dashboardSegmentCmd.SetOut(&buf)
	err := dashboardSegmentCmd.RunE(dashboardSegmentCmd, []string{"roadmap"})
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestDashboardSegmentCmd_BadContextJSON(t *testing.T) {
	setupDashboardDB(t)

	dashboardSegmentCmd.SetIn(strings.NewReader(`{not json`))
	dashboardSegmentCmd.SetOut(&bytes.Buffer{})
	err := dashboardSegmentCmd.RunE(dashboardSegmentCmd, []string{"roadmap"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse context")
}

func TestDashboardSegmentCmd_GitEmitsFragments(t *testing.T) {
	setupDashboardDB(t)

	dir := t.TempDir()
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "config", "user.email", "test-user@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test User")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o644))
	runGitCmd(t, dir, "add", "f.txt")
	runGitCmd(t, dir, "commit", "-m", "init")

	var buf bytes.Buffer
	dashboardSegmentCmd.SetIn(strings.NewReader(`{"schema":1,"now":"2026-07-08T00:00:00Z","repo":"` + dir + `"}`))
	dashboardSegmentCmd.SetOut(&buf)
	err := dashboardSegmentCmd.RunE(dashboardSegmentCmd, []string{"git"})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, `"type":"tile"`)
	assert.Contains(t, output, `"label":"branch"`)
}

// ── dashboard refresh ────────────────────────────────────────────────────────

func resetDashboardRefreshFlag() {
	dashboardRefreshForce = false
}

func TestDashboardRefreshCmd_DisabledByDefault(t *testing.T) {
	resetDashboardRefreshFlag()
	dbPath := setupDashboardDB(t)

	var buf bytes.Buffer
	dashboardRefreshCmd.SetOut(&buf)
	err := dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.NoError(t, err)
	assert.Equal(t, "dashboard disabled\n", buf.String())

	_, statErr := os.Stat(filepath.Join(filepath.Dir(dbPath), "dashboard.ndjson"))
	assert.True(t, os.IsNotExist(statErr), "refresh must not write an output file while disabled")
}

func TestDashboardRefreshCmd_DisabledWinsOverForce(t *testing.T) {
	resetDashboardRefreshFlag()
	setupDashboardDB(t)

	dashboardRefreshForce = true
	defer resetDashboardRefreshFlag()

	var buf bytes.Buffer
	dashboardRefreshCmd.SetOut(&buf)
	err := dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.NoError(t, err)
	assert.Equal(t, "dashboard disabled\n", buf.String())
}

func TestDashboardRefreshCmd_EnabledForceWritesOutput(t *testing.T) {
	resetDashboardRefreshFlag()
	dbPath := setupDashboardDB(t)

	require.NoError(t, withDB(func(db *sql.DB) error {
		return backlog.SetConfig(db, dashboard.KeyEnabled, "true")
	}))

	dashboardRefreshForce = true
	defer resetDashboardRefreshFlag()

	var buf bytes.Buffer
	dashboardRefreshCmd.SetOut(&buf)
	err := dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "refreshed")

	outPath := filepath.Join(filepath.Dir(dbPath), "dashboard.ndjson")
	_, statErr := os.Stat(outPath)
	require.NoError(t, statErr)
}

func TestDashboardRefreshCmd_CooldownBlocksThenForceAllows(t *testing.T) {
	resetDashboardRefreshFlag()
	setupDashboardDB(t)

	require.NoError(t, withDB(func(db *sql.DB) error {
		if err := backlog.SetConfig(db, dashboard.KeyEnabled, "true"); err != nil {
			return err
		}
		if err := backlog.SetConfig(db, dashboard.KeyCooldown, "60s"); err != nil {
			return err
		}
		return backlog.SetConfig(db, dashboard.KeyLastRefresh, time.Now().UTC().Format(time.RFC3339))
	}))

	var buf bytes.Buffer
	dashboardRefreshCmd.SetOut(&buf)
	err := dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "within cooldown")

	dashboardRefreshForce = true
	defer resetDashboardRefreshFlag()

	var buf2 bytes.Buffer
	dashboardRefreshCmd.SetOut(&buf2)
	err = dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf2.String(), "refreshed")
}

func TestDashboardRefreshCmd_UnknownBuiltinSegmentDropped(t *testing.T) {
	resetDashboardRefreshFlag()
	setupDashboardDB(t)

	require.NoError(t, withDB(func(db *sql.DB) error {
		if err := backlog.SetConfig(db, dashboard.KeyEnabled, "true"); err != nil {
			return err
		}
		return backlog.SetConfig(db, dashboard.KeySegments, `[{"id":"bogus","builtin":"bogus"}]`)
	}))

	dashboardRefreshForce = true
	defer resetDashboardRefreshFlag()

	var buf bytes.Buffer
	dashboardRefreshCmd.SetOut(&buf)
	err := dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "refreshed 1 segments, 0 fragments (1 dropped)")
}

func TestDashboardRefreshCmd_ExecSegmentUnsupported(t *testing.T) {
	resetDashboardRefreshFlag()
	setupDashboardDB(t)

	require.NoError(t, withDB(func(db *sql.DB) error {
		if err := backlog.SetConfig(db, dashboard.KeyEnabled, "true"); err != nil {
			return err
		}
		return backlog.SetConfig(db, dashboard.KeySegments, `[{"id":"custom","exec":["echo","hi"]}]`)
	}))

	dashboardRefreshForce = true
	defer resetDashboardRefreshFlag()

	var buf bytes.Buffer
	dashboardRefreshCmd.SetOut(&buf)
	err := dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "1 dropped")
}

func TestDashboardRefreshCmd_InvalidSegmentsJSON(t *testing.T) {
	resetDashboardRefreshFlag()
	setupDashboardDB(t)

	require.NoError(t, withDB(func(db *sql.DB) error {
		if err := backlog.SetConfig(db, dashboard.KeyEnabled, "true"); err != nil {
			return err
		}
		return backlog.SetConfig(db, dashboard.KeySegments, `not json`)
	}))

	dashboardRefreshForce = true
	defer resetDashboardRefreshFlag()

	dashboardRefreshCmd.SetOut(&bytes.Buffer{})
	err := dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse segments")
}

func TestDashboardRefreshCmd_ExplicitOutputPath(t *testing.T) {
	resetDashboardRefreshFlag()
	dir := t.TempDir()
	t.Setenv("PROJECT_KB_PATH", filepath.Join(dir, "dashboard.db"))

	outPath := filepath.Join(dir, "custom.ndjson")
	require.NoError(t, withDB(func(db *sql.DB) error {
		if err := backlog.SetConfig(db, dashboard.KeyEnabled, "true"); err != nil {
			return err
		}
		return backlog.SetConfig(db, dashboard.KeyOutput, outPath)
	}))

	dashboardRefreshForce = true
	defer resetDashboardRefreshFlag()

	var buf bytes.Buffer
	dashboardRefreshCmd.SetOut(&buf)
	err := dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), outPath)

	_, statErr := os.Stat(outPath)
	require.NoError(t, statErr)
}

func TestResolveDashboardContext(t *testing.T) {
	ctx := resolveDashboardContext()
	assert.Equal(t, 1, ctx.Schema)
	assert.NotEmpty(t, ctx.Now)
}
