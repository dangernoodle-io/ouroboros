package dashboard

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/store"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := store.InitDB(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestDefaultSegments(t *testing.T) {
	segs := DefaultSegments()
	require.Len(t, segs, 2)
	assert.Equal(t, "git", segs[0].ID)
	assert.Equal(t, "git", segs[0].Builtin)
	assert.Equal(t, "roadmap", segs[1].ID)
	assert.Equal(t, "roadmap", segs[1].Builtin)
}

func TestParseSegments(t *testing.T) {
	specs, err := ParseSegments(`[{"id":"git","builtin":"git"},{"id":"custom","exec":["echo","hi"]}]`)
	require.NoError(t, err)
	require.Len(t, specs, 2)
	assert.Equal(t, "git", specs[0].Builtin)
	assert.Equal(t, []string{"echo", "hi"}, specs[1].Exec)
}

func TestParseSegments_Invalid(t *testing.T) {
	_, err := ParseSegments(`not json`)
	require.Error(t, err)
}

func TestSegmentSpec_IsEnabled(t *testing.T) {
	trueVal := true
	falseVal := false

	assert.True(t, SegmentSpec{}.IsEnabled())
	assert.True(t, SegmentSpec{Enabled: &trueVal}.IsEnabled())
	assert.False(t, SegmentSpec{Enabled: &falseVal}.IsEnabled())
}

func TestParseCooldown(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want time.Duration
	}{
		{"empty defaults to 60s", "", 60 * time.Second},
		{"invalid defaults to 60s", "not-a-duration", 60 * time.Second},
		{"floors below minimum", "5s", MinCooldown},
		{"passes through valid value above floor", "2m", 2 * time.Minute},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ParseCooldown(tc.in))
		})
	}
}

// ── views ────────────────────────────────────────────────────────────────────

func TestSetViewGetView_RoundTrip(t *testing.T) {
	db := newTestDB(t)

	v := View{Name: "miner", Projects: []string{"breadboard", "notarizedbyape.com"}, Cooldown: "5m", Output: "/tmp/out.ndjson"}
	require.NoError(t, SetView(db, v))

	got, ok, err := GetView(db, "miner")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "miner", got.Name)
	assert.Equal(t, []string{"breadboard", "notarizedbyape.com"}, got.Projects)
	assert.Equal(t, "5m", got.Cooldown)
	assert.Equal(t, "/tmp/out.ndjson", got.Output)
}

func TestSetView_RejectsUnsafeNames(t *testing.T) {
	db := newTestDB(t)

	tests := []struct {
		name string
		view string
	}{
		{"path traversal", "../x"},
		{"path separator", "a/b"},
		{"backslash", `a\b`},
		{"empty", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := SetView(db, View{Name: tc.view, Projects: []string{"breadboard"}})
			require.Error(t, err)

			_, ok, getErr := GetView(db, tc.view)
			require.NoError(t, getErr)
			assert.False(t, ok)
		})
	}
}

func TestSetView_AcceptsNormalName(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, SetView(db, View{Name: "miner", Projects: []string{"breadboard"}}))

	_, ok, err := GetView(db, "miner")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestGetView_NotFound(t *testing.T) {
	db := newTestDB(t)

	_, ok, err := GetView(db, "nonexistent")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestListViews_SortedByName(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, SetView(db, View{Name: "taipan", Projects: []string{"taipan-cli"}}))
	require.NoError(t, SetView(db, View{Name: "miner", Projects: []string{"breadboard"}}))

	views, err := ListViews(db)
	require.NoError(t, err)
	require.Len(t, views, 2)
	assert.Equal(t, "miner", views[0].Name)
	assert.Equal(t, "taipan", views[1].Name)
}

func TestListViews_MalformedBlobJoinsErrorButKeepsGoodViews(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, SetView(db, View{Name: "good", Projects: []string{"breadboard"}}))
	require.NoError(t, backlog.SetConfig(db, ViewKeyPrefix+"bad", "not json"))

	views, err := ListViews(db)
	require.Error(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, "good", views[0].Name)
	assert.Contains(t, err.Error(), "bad")
}

func TestListViews_SkipsNonViewConfigKeys(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, SetView(db, View{Name: "miner", Projects: []string{"breadboard"}}))
	require.NoError(t, backlog.SetConfig(db, KeyEnabled, "true"))

	views, err := ListViews(db)
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, "miner", views[0].Name)
}

func TestGetView_MalformedBlob(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, backlog.SetConfig(db, ViewKeyPrefix+"bad", "not json"))

	_, ok, err := GetView(db, "bad")
	require.Error(t, err)
	assert.False(t, ok)
}

func TestGetProjectConfig_MalformedBlob(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, backlog.SetConfig(db, ProjectKeyPrefix+"proj", "not json"))

	_, err := GetProjectConfig(db, "proj")
	require.Error(t, err)
}

func TestEffectiveSegments_MalformedProjectBlob(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, backlog.SetConfig(db, ProjectKeyPrefix+"proj", "not json"))

	_, err := EffectiveSegments(db, "proj")
	require.Error(t, err)
}

func TestGetViewRuntime_MalformedBlob(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, backlog.SetConfig(db, RuntimeKeyPrefix+"miner", "not json"))

	_, err := GetViewRuntime(db, "miner")
	require.Error(t, err)
}

func TestRemoveView(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, SetView(db, View{Name: "miner", Projects: []string{"breadboard"}}))
	require.NoError(t, RemoveView(db, "miner"))

	_, ok, err := GetView(db, "miner")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestMonitoredProjects_UnionDedupSort(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, SetView(db, View{Name: "miner", Projects: []string{"breadboard", "TaipanMiner"}}))
	require.NoError(t, SetView(db, View{Name: "taipan", Projects: []string{"taipan-cli", "TaipanMiner"}}))

	projects, err := MonitoredProjects(db)
	require.NoError(t, err)
	assert.Equal(t, []string{"TaipanMiner", "breadboard", "taipan-cli"}, projects)
}

func TestMonitoredProjects_NoViews(t *testing.T) {
	db := newTestDB(t)

	projects, err := MonitoredProjects(db)
	require.NoError(t, err)
	assert.Empty(t, projects)
}

// ── per-project config ───────────────────────────────────────────────────────

func TestGetSetProjectConfig_RoundTrip(t *testing.T) {
	db := newTestDB(t)

	pc := ProjectConfig{Segments: []SegmentSpec{{ID: "git", Builtin: "git"}}}
	require.NoError(t, SetProjectConfig(db, "notarizedbyape.com", pc))

	got, err := GetProjectConfig(db, "notarizedbyape.com")
	require.NoError(t, err)
	assert.Equal(t, pc, got)
}

func TestGetProjectConfig_Absent(t *testing.T) {
	db := newTestDB(t)

	got, err := GetProjectConfig(db, "nonexistent")
	require.NoError(t, err)
	assert.Equal(t, ProjectConfig{}, got)
}

func TestEffectiveSegments_Precedence(t *testing.T) {
	db := newTestDB(t)

	// No override, no global -> default.
	segs, err := EffectiveSegments(db, "proj")
	require.NoError(t, err)
	assert.Equal(t, DefaultSegments(), segs)

	// Global set, no per-project override -> global.
	require.NoError(t, backlog.SetConfig(db, KeySegments, `[{"id":"roadmap","builtin":"roadmap"}]`))
	segs, err = EffectiveSegments(db, "proj")
	require.NoError(t, err)
	require.Len(t, segs, 1)
	assert.Equal(t, "roadmap", segs[0].ID)

	// Per-project override -> wins over global.
	require.NoError(t, SetProjectConfig(db, "proj", ProjectConfig{Segments: []SegmentSpec{{ID: "git", Builtin: "git"}}}))
	segs, err = EffectiveSegments(db, "proj")
	require.NoError(t, err)
	require.Len(t, segs, 1)
	assert.Equal(t, "git", segs[0].ID)
}

func TestResolveRepo(t *testing.T) {
	tests := []struct {
		name    string
		project string
		repo    string
		root    string
		want    string
	}{
		{"override wins over workspace root", "proj", "/explicit/repo", "/ws", "/explicit/repo"},
		{"no override, root set -> root/project", "proj", "", "/ws", filepath.Join("/ws", "proj")},
		{"neither set -> empty", "proj", "", "", ""},
		{"dotted project name joins under root", "notarizedbyape.com", "", "/ws", filepath.Join("/ws", "notarizedbyape.com")},
		{"path traversal project name -> not joined", "../evil", "", "/ws", ""},
		{"path separator project name -> not joined", "a/b", "", "/ws", ""},
		{"backslash project name -> not joined", `a\b`, "", "/ws", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t)
			if tc.repo != "" {
				require.NoError(t, SetProjectConfig(db, tc.project, ProjectConfig{Repo: tc.repo}))
			}
			if tc.root != "" {
				require.NoError(t, backlog.SetConfig(db, KeyWorkspaceRoot, tc.root))
			}

			got, err := ResolveRepo(db, tc.project)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestResolveRepo_NoWorkspaceRootConfigured(t *testing.T) {
	db := newTestDB(t)

	got, err := ResolveRepo(db, "proj")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestResolveRepo_WorkspaceRootExplicitlyEmpty(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, backlog.SetConfig(db, KeyWorkspaceRoot, ""))

	got, err := ResolveRepo(db, "proj")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestSetProjectConfig_RepoAndSegmentsIndependent(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, SetProjectConfig(db, "proj", ProjectConfig{Segments: []SegmentSpec{{ID: "git", Builtin: "git"}}}))
	got, err := GetProjectConfig(db, "proj")
	require.NoError(t, err)
	require.Len(t, got.Segments, 1)
	assert.Empty(t, got.Repo)

	got.Repo = "/repos/proj"
	require.NoError(t, SetProjectConfig(db, "proj", got))

	got, err = GetProjectConfig(db, "proj")
	require.NoError(t, err)
	assert.Equal(t, "/repos/proj", got.Repo)
	require.Len(t, got.Segments, 1)
	assert.Equal(t, "git", got.Segments[0].ID)
}

// ── output paths ─────────────────────────────────────────────────────────────

func TestOutputDir_DefaultVsExplicit(t *testing.T) {
	db := newTestDB(t)

	assert.Equal(t, filepath.Join("/db/dir", "dashboards"), OutputDir(db, "/db/dir/kb.db"))

	require.NoError(t, backlog.SetConfig(db, KeyOutputDir, "/custom/output"))
	assert.Equal(t, "/custom/output", OutputDir(db, "/db/dir/kb.db"))
}

func TestViewOutputPath_DefaultVsExplicit(t *testing.T) {
	db := newTestDB(t)

	v := View{Name: "miner", Projects: []string{"breadboard"}}
	assert.Equal(t, filepath.Join("/db/dir", "dashboards", "miner.ndjson"), ViewOutputPath(db, v, "/db/dir/kb.db"))

	v.Output = "/explicit/path.ndjson"
	assert.Equal(t, "/explicit/path.ndjson", ViewOutputPath(db, v, "/db/dir/kb.db"))
}

// ── cooldown precedence ──────────────────────────────────────────────────────

func TestViewCooldown_Precedence(t *testing.T) {
	db := newTestDB(t)

	v := View{Name: "miner", Projects: []string{"breadboard"}}
	assert.Equal(t, 60*time.Second, ViewCooldown(db, v))

	require.NoError(t, backlog.SetConfig(db, KeyCooldown, "2m"))
	assert.Equal(t, 2*time.Minute, ViewCooldown(db, v))

	v.Cooldown = "5m"
	assert.Equal(t, 5*time.Minute, ViewCooldown(db, v))
}

// ── view runtime ─────────────────────────────────────────────────────────────

// ── db-error propagation (closed *sql.DB forces a real query error) ────────

func TestListViews_DBError(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Close())

	_, err := ListViews(db)
	require.Error(t, err)
}

func TestGetView_DBError(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Close())

	_, _, err := GetView(db, "miner")
	require.Error(t, err)
}

func TestGetProjectConfig_DBError(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Close())

	_, err := GetProjectConfig(db, "proj")
	require.Error(t, err)
}

func TestEffectiveSegments_DBError(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Close())

	_, err := EffectiveSegments(db, "proj")
	require.Error(t, err)
}

func TestGetViewRuntime_DBError(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Close())

	_, err := GetViewRuntime(db, "miner")
	require.Error(t, err)
}

func TestMonitoredProjects_PropagatesListViewsError(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, backlog.SetConfig(db, ViewKeyPrefix+"bad", "not json"))

	projects, err := MonitoredProjects(db)
	require.Error(t, err)
	assert.Empty(t, projects)
}

func TestSetView_DBError(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Close())

	err := SetView(db, View{Name: "miner", Projects: []string{"breadboard"}})
	require.Error(t, err)
}

func TestSetProjectConfig_DBError(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Close())

	err := SetProjectConfig(db, "proj", ProjectConfig{})
	require.Error(t, err)
}

func TestResolveRepo_DBError(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Close())

	_, err := ResolveRepo(db, "proj")
	require.Error(t, err)
}

func TestSetViewLastRefresh_DBError(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Close())

	err := SetViewLastRefresh(db, "miner", "2026-07-08T00:00:00Z")
	require.Error(t, err)
}

func TestGetSetViewLastRefresh_RoundTrip(t *testing.T) {
	db := newTestDB(t)

	rt, err := GetViewRuntime(db, "miner")
	require.NoError(t, err)
	assert.Empty(t, rt.LastRefresh)

	require.NoError(t, SetViewLastRefresh(db, "miner", "2026-07-08T00:00:00Z"))
	rt, err = GetViewRuntime(db, "miner")
	require.NoError(t, err)
	assert.Equal(t, "2026-07-08T00:00:00Z", rt.LastRefresh)

	require.NoError(t, DeleteViewRuntime(db, "miner"))
	rt, err = GetViewRuntime(db, "miner")
	require.NoError(t, err)
	assert.Empty(t, rt.LastRefresh)
}
