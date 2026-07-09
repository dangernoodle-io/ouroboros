package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
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

func resetDashboardFlags() {
	dashboardRefreshForce = false
	dashboardRefreshView = ""
	dashboardRefreshAll = false
	dashboardRefreshProjects = ""
	dashboardViewSetProjects = ""
	dashboardViewSetCooldown = ""
	dashboardViewSetOutput = ""
	dashboardProjectSetSegments = ""
}

func enableDashboard(t *testing.T) {
	t.Helper()
	require.NoError(t, withDB(func(db *sql.DB) error {
		return backlog.SetConfig(db, dashboard.KeyEnabled, "true")
	}))
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

// ── dashboard view ───────────────────────────────────────────────────────────

func TestDashboardViewSet_RequiresProjects(t *testing.T) {
	resetDashboardFlags()
	setupDashboardDB(t)

	dashboardViewSetCmd.SetOut(&bytes.Buffer{})
	err := dashboardViewSetCmd.RunE(dashboardViewSetCmd, []string{"miner"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--projects")
}

func TestDashboardViewSet_ListRm(t *testing.T) {
	resetDashboardFlags()
	setupDashboardDB(t)

	dashboardViewSetProjects = "breadboard,TaipanMiner"
	defer resetDashboardFlags()

	var setBuf bytes.Buffer
	dashboardViewSetCmd.SetOut(&setBuf)
	require.NoError(t, dashboardViewSetCmd.RunE(dashboardViewSetCmd, []string{"miner"}))
	assert.Contains(t, setBuf.String(), "set view miner (2 projects)")

	var listBuf bytes.Buffer
	dashboardViewListCmd.SetOut(&listBuf)
	require.NoError(t, dashboardViewListCmd.RunE(dashboardViewListCmd, []string{}))
	assert.Contains(t, listBuf.String(), "view miner: [breadboard, TaipanMiner]")
	assert.Contains(t, listBuf.String(), "monitored projects (2): TaipanMiner, breadboard")

	var rmBuf bytes.Buffer
	dashboardViewRmCmd.SetOut(&rmBuf)
	require.NoError(t, dashboardViewRmCmd.RunE(dashboardViewRmCmd, []string{"miner"}))
	assert.Contains(t, rmBuf.String(), "removed view miner")

	var listBuf2 bytes.Buffer
	dashboardViewListCmd.SetOut(&listBuf2)
	require.NoError(t, dashboardViewListCmd.RunE(dashboardViewListCmd, []string{}))
	assert.Contains(t, listBuf2.String(), "no views configured")
}

func TestDashboardViewSet_WithCooldownAndOutput(t *testing.T) {
	resetDashboardFlags()
	setupDashboardDB(t)

	dashboardViewSetProjects = "breadboard"
	dashboardViewSetCooldown = "5m"
	dashboardViewSetOutput = "/tmp/custom-view.ndjson"
	defer resetDashboardFlags()

	dashboardViewSetCmd.SetOut(&bytes.Buffer{})
	require.NoError(t, dashboardViewSetCmd.RunE(dashboardViewSetCmd, []string{"solo"}))

	require.NoError(t, withDB(func(db *sql.DB) error {
		v, ok, err := dashboard.GetView(db, "solo")
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "5m", v.Cooldown)
		assert.Equal(t, "/tmp/custom-view.ndjson", v.Output)
		return nil
	}))
}

// ── dashboard project ────────────────────────────────────────────────────────

func TestDashboardProjectSet(t *testing.T) {
	resetDashboardFlags()
	setupDashboardDB(t)

	dashboardProjectSetSegments = `[{"id":"git","builtin":"git"}]`
	defer resetDashboardFlags()

	var buf bytes.Buffer
	dashboardProjectSetCmd.SetOut(&buf)
	require.NoError(t, dashboardProjectSetCmd.RunE(dashboardProjectSetCmd, []string{"breadboard"}))
	assert.Contains(t, buf.String(), "set project breadboard (1 segments)")
}

func TestDashboardProjectSet_InvalidJSON(t *testing.T) {
	resetDashboardFlags()
	setupDashboardDB(t)

	dashboardProjectSetSegments = `not json`
	defer resetDashboardFlags()

	dashboardProjectSetCmd.SetOut(&bytes.Buffer{})
	err := dashboardProjectSetCmd.RunE(dashboardProjectSetCmd, []string{"breadboard"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse segments")
}

// ── dashboard status ─────────────────────────────────────────────────────────

func TestDashboardStatus_Empty(t *testing.T) {
	resetDashboardFlags()
	setupDashboardDB(t)

	var buf bytes.Buffer
	dashboardStatusCmd.SetOut(&buf)
	require.NoError(t, dashboardStatusCmd.RunE(dashboardStatusCmd, []string{}))
	assert.Contains(t, buf.String(), "dashboard.enabled: false")
	assert.Contains(t, buf.String(), "no views configured")
	assert.Contains(t, buf.String(), "monitored projects (0)")
}

func TestDashboardStatus_WithSegmentOverrideNote(t *testing.T) {
	resetDashboardFlags()
	setupDashboardDB(t)

	require.NoError(t, withDB(func(db *sql.DB) error {
		if err := dashboard.SetView(db, dashboard.View{Name: "miner", Projects: []string{"breadboard"}}); err != nil {
			return err
		}
		return dashboard.SetProjectConfig(db, "breadboard", dashboard.ProjectConfig{
			Segments: []dashboard.SegmentSpec{{ID: "git", Builtin: "git"}},
		})
	}))

	var buf bytes.Buffer
	dashboardStatusCmd.SetOut(&buf)
	require.NoError(t, dashboardStatusCmd.RunE(dashboardStatusCmd, []string{}))
	assert.Contains(t, buf.String(), "segment overrides: breadboard")
}

// ── dashboard refresh ────────────────────────────────────────────────────────

func TestDashboardRefreshCmd_DisabledByDefault(t *testing.T) {
	resetDashboardFlags()
	dbPath := setupDashboardDB(t)

	var buf bytes.Buffer
	dashboardRefreshCmd.SetOut(&buf)
	err := dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.NoError(t, err)
	assert.Equal(t, "dashboard disabled\n", buf.String())

	_, statErr := os.Stat(filepath.Join(filepath.Dir(dbPath), "dashboards"))
	assert.True(t, os.IsNotExist(statErr), "refresh must not write any output while disabled")
}

func TestDashboardRefreshCmd_DisabledWinsOverForce(t *testing.T) {
	resetDashboardFlags()
	setupDashboardDB(t)

	dashboardRefreshForce = true
	defer resetDashboardFlags()

	var buf bytes.Buffer
	dashboardRefreshCmd.SetOut(&buf)
	err := dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.NoError(t, err)
	assert.Equal(t, "dashboard disabled\n", buf.String())
}

func TestDashboardRefreshCmd_DisabledWinsOverProjects(t *testing.T) {
	resetDashboardFlags()
	dbPath := setupDashboardDB(t)

	dashboardRefreshProjects = "foo,bar"
	defer resetDashboardFlags()

	var buf bytes.Buffer
	dashboardRefreshCmd.SetOut(&buf)
	err := dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.NoError(t, err)
	assert.Equal(t, "dashboard disabled\n", buf.String())

	_, statErr := os.Stat(filepath.Join(filepath.Dir(dbPath), "dashboards", "_adhoc.ndjson"))
	assert.True(t, os.IsNotExist(statErr), "refresh must not write an ad-hoc output file while disabled")
}

func TestDashboardRefreshCmd_NoViewsConfigured(t *testing.T) {
	resetDashboardFlags()
	setupDashboardDB(t)
	enableDashboard(t)

	var buf bytes.Buffer
	dashboardRefreshCmd.SetOut(&buf)
	err := dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "no views configured")
}

func TestDashboardRefreshCmd_ViewForceWritesOutput(t *testing.T) {
	resetDashboardFlags()
	dbPath := setupDashboardDB(t)
	enableDashboard(t)

	dir := t.TempDir()
	runGitCmd(t, dir, "init")

	require.NoError(t, withDB(func(db *sql.DB) error {
		return dashboard.SetView(db, dashboard.View{Name: "miner", Projects: []string{"breadboard"}})
	}))

	dashboardRefreshView = "miner"
	dashboardRefreshForce = true
	defer resetDashboardFlags()

	var buf bytes.Buffer
	dashboardRefreshCmd.SetOut(&buf)
	err := dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "refreshed view miner (1 projects)")

	outPath := filepath.Join(filepath.Dir(dbPath), "dashboards", "miner.ndjson")
	data, statErr := os.ReadFile(outPath)
	require.NoError(t, statErr)
	assert.Contains(t, string(data), `"project":"breadboard"`)
}

func TestDashboardRefreshCmd_UnknownView(t *testing.T) {
	resetDashboardFlags()
	setupDashboardDB(t)
	enableDashboard(t)

	dashboardRefreshView = "nonexistent"
	defer resetDashboardFlags()

	dashboardRefreshCmd.SetOut(&bytes.Buffer{})
	err := dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown view")
}

func TestDashboardRefreshCmd_MutuallyExclusiveFlags(t *testing.T) {
	resetDashboardFlags()
	setupDashboardDB(t)
	enableDashboard(t)

	dashboardRefreshView = "miner"
	dashboardRefreshAll = true
	defer resetDashboardFlags()

	dashboardRefreshCmd.SetOut(&bytes.Buffer{})
	err := dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestDashboardRefreshCmd_AllRefreshesEveryView(t *testing.T) {
	resetDashboardFlags()
	dbPath := setupDashboardDB(t)
	enableDashboard(t)

	require.NoError(t, withDB(func(db *sql.DB) error {
		if err := dashboard.SetView(db, dashboard.View{Name: "miner", Projects: []string{"breadboard"}}); err != nil {
			return err
		}
		return dashboard.SetView(db, dashboard.View{Name: "taipan", Projects: []string{"taipan-cli"}})
	}))

	dashboardRefreshAll = true
	dashboardRefreshForce = true
	defer resetDashboardFlags()

	var buf bytes.Buffer
	dashboardRefreshCmd.SetOut(&buf)
	err := dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.NoError(t, err)

	_, err1 := os.Stat(filepath.Join(filepath.Dir(dbPath), "dashboards", "miner.ndjson"))
	_, err2 := os.Stat(filepath.Join(filepath.Dir(dbPath), "dashboards", "taipan.ndjson"))
	require.NoError(t, err1)
	require.NoError(t, err2)
}

func TestDashboardRefreshCmd_AllToleratesOneBadView(t *testing.T) {
	resetDashboardFlags()
	dbPath := setupDashboardDB(t)
	enableDashboard(t)

	require.NoError(t, withDB(func(db *sql.DB) error {
		if err := dashboard.SetView(db, dashboard.View{Name: "miner", Projects: []string{"breadboard"}}); err != nil {
			return err
		}
		return backlog.SetConfig(db, dashboard.ViewKeyPrefix+"bad", "{not json")
	}))

	dashboardRefreshAll = true
	dashboardRefreshForce = true
	defer resetDashboardFlags()

	var buf bytes.Buffer
	dashboardRefreshCmd.SetOut(&buf)
	err := dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "warning: some views failed to load")
	assert.Contains(t, buf.String(), "refreshed view miner")

	_, statErr := os.Stat(filepath.Join(filepath.Dir(dbPath), "dashboards", "miner.ndjson"))
	require.NoError(t, statErr)
}

func TestDashboardRefreshCmd_AdhocProjectsAlwaysRuns(t *testing.T) {
	resetDashboardFlags()
	dbPath := setupDashboardDB(t)
	enableDashboard(t)

	dashboardRefreshProjects = "foo,bar"
	defer resetDashboardFlags()

	var buf bytes.Buffer
	dashboardRefreshCmd.SetOut(&buf)
	err := dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "refreshed view _adhoc (2 projects)")

	outPath := filepath.Join(filepath.Dir(dbPath), "dashboards", "_adhoc.ndjson")
	_, statErr := os.Stat(outPath)
	require.NoError(t, statErr)

	// Ad-hoc refresh never gates on cooldown/runtime: running twice in a row
	// (no --force) still runs both times.
	var buf2 bytes.Buffer
	dashboardRefreshCmd.SetOut(&buf2)
	err = dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf2.String(), "refreshed view _adhoc")
}

func TestDashboardRefreshCmd_CooldownBlocksThenForceAllows(t *testing.T) {
	resetDashboardFlags()
	setupDashboardDB(t)
	enableDashboard(t)

	require.NoError(t, withDB(func(db *sql.DB) error {
		if err := dashboard.SetView(db, dashboard.View{Name: "miner", Projects: []string{"breadboard"}, Cooldown: "60s"}); err != nil {
			return err
		}
		return dashboard.SetViewLastRefresh(db, "miner", time.Now().UTC().Format(time.RFC3339))
	}))

	dashboardRefreshView = "miner"
	defer resetDashboardFlags()

	var buf bytes.Buffer
	dashboardRefreshCmd.SetOut(&buf)
	err := dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "within cooldown")

	dashboardRefreshForce = true
	defer func() { dashboardRefreshForce = false }()

	var buf2 bytes.Buffer
	dashboardRefreshCmd.SetOut(&buf2)
	err = dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf2.String(), "refreshed")
}

func TestDashboardRefreshCmd_UnknownBuiltinSegmentDropped(t *testing.T) {
	resetDashboardFlags()
	setupDashboardDB(t)
	enableDashboard(t)

	require.NoError(t, withDB(func(db *sql.DB) error {
		if err := dashboard.SetView(db, dashboard.View{Name: "miner", Projects: []string{"breadboard"}}); err != nil {
			return err
		}
		return dashboard.SetProjectConfig(db, "breadboard", dashboard.ProjectConfig{
			Segments: []dashboard.SegmentSpec{{ID: "bogus", Builtin: "bogus"}},
		})
	}))

	dashboardRefreshView = "miner"
	dashboardRefreshForce = true
	defer resetDashboardFlags()

	var buf bytes.Buffer
	dashboardRefreshCmd.SetOut(&buf)
	err := dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "0 fragments (1 dropped)")
}

func TestDashboardRefreshCmd_ExecSegmentUnsupported(t *testing.T) {
	resetDashboardFlags()
	setupDashboardDB(t)
	enableDashboard(t)

	require.NoError(t, withDB(func(db *sql.DB) error {
		if err := dashboard.SetView(db, dashboard.View{Name: "miner", Projects: []string{"breadboard"}}); err != nil {
			return err
		}
		return dashboard.SetProjectConfig(db, "breadboard", dashboard.ProjectConfig{
			Segments: []dashboard.SegmentSpec{{ID: "custom", Exec: []string{"echo", "hi"}}},
		})
	}))

	dashboardRefreshView = "miner"
	dashboardRefreshForce = true
	defer resetDashboardFlags()

	var buf bytes.Buffer
	dashboardRefreshCmd.SetOut(&buf)
	err := dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "1 dropped")
}

func TestDashboardRefreshCmd_ExplicitOutputPath(t *testing.T) {
	resetDashboardFlags()
	dir := t.TempDir()
	t.Setenv("PROJECT_KB_PATH", filepath.Join(dir, "dashboard.db"))
	enableDashboard(t)

	outPath := filepath.Join(dir, "custom.ndjson")
	require.NoError(t, withDB(func(db *sql.DB) error {
		return dashboard.SetView(db, dashboard.View{Name: "miner", Projects: []string{"breadboard"}, Output: outPath})
	}))

	dashboardRefreshView = "miner"
	dashboardRefreshForce = true
	defer resetDashboardFlags()

	var buf bytes.Buffer
	dashboardRefreshCmd.SetOut(&buf)
	err := dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), outPath)

	_, statErr := os.Stat(outPath)
	require.NoError(t, statErr)
}

func TestDashboardRefreshCmd_ProjectFragmentsCarryProject(t *testing.T) {
	resetDashboardFlags()
	dbPath := setupDashboardDB(t)
	enableDashboard(t)

	require.NoError(t, withDB(func(db *sql.DB) error {
		return dashboard.SetView(db, dashboard.View{Name: "miner", Projects: []string{"breadboard"}})
	}))

	dashboardRefreshView = "miner"
	dashboardRefreshForce = true
	defer resetDashboardFlags()

	dashboardRefreshCmd.SetOut(&bytes.Buffer{})
	require.NoError(t, dashboardRefreshCmd.RunE(dashboardRefreshCmd, []string{}))

	outPath := filepath.Join(filepath.Dir(dbPath), "dashboards", "miner.ndjson")
	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	// roadmap segment emits nothing for a project with no roadmap; assert
	// the file is at least valid (may be empty) and, if non-empty, every
	// line carries the project attribution.
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var frag map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &frag))
		assert.Equal(t, "breadboard", frag["project"])
	}
}

func TestResolveDashboardContext(t *testing.T) {
	ctx := resolveDashboardContext()
	assert.Equal(t, 1, ctx.Schema)
	assert.NotEmpty(t, ctx.Now)
}
