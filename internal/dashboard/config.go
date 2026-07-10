package dashboard

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dangernoodle.io/ouroboros/internal/backlog"
)

// Config keys under which the dashboard layer stores its settings in the
// backlog config table.
const (
	KeyEnabled       = "dashboard.enabled"
	KeySegments      = "dashboard.segments"
	KeyCooldown      = "dashboard.cooldown"
	KeyOutputDir     = "dashboard.output_dir"
	KeyWorkspaceRoot = "dashboard.workspace_root"

	// ViewKeyPrefix/ProjectKeyPrefix/RuntimeKeyPrefix key one JSON blob per
	// entity: dashboard.view.<name>, dashboard.project.<name>,
	// dashboard.rt.<view-name>. A single blob per entity avoids parsing
	// dotted config-key tails, so project/view names containing dots (e.g.
	// notarizedbyape.com) round-trip safely as the whole key remainder.
	ViewKeyPrefix    = "dashboard.view."
	ProjectKeyPrefix = "dashboard.project."
	RuntimeKeyPrefix = "dashboard.rt."
)

// defaultOutputDirName names the subdirectory (next to the KB db file) used
// when dashboard.output_dir is unset.
const defaultOutputDirName = "dashboards"

// View is a named set of projects that share one composed dashboard output.
// A project may belong to several views; the union of every view's
// Projects is what ouroboros considers monitored.
type View struct {
	Name     string   `json:"-"`
	Projects []string `json:"projects"`
	Cooldown string   `json:"cooldown,omitempty"`
	Output   string   `json:"output,omitempty"`
}

// ProjectConfig holds per-project dashboard overrides.
type ProjectConfig struct {
	Segments []SegmentSpec `json:"segments,omitempty"`
	// Repo overrides the on-disk repo path a project resolves to (see
	// ResolveRepo). Wins over dashboard.workspace_root when set.
	Repo string `json:"repo,omitempty"`
}

// viewRuntime holds per-view runtime state (last_refresh), stored under a
// separate key from the user-authored View blob so refreshing never
// rewrites config the user wrote.
type viewRuntime struct {
	LastRefresh string `json:"last_refresh,omitempty"`
}

// ListViews returns every configured view, sorted by name. A malformed view
// blob does not hide the other views: it is skipped and its parse error is
// joined into the returned error, so the caller still gets every view that
// parsed.
func ListViews(db *sql.DB) ([]View, error) {
	all, err := backlog.GetAllConfig(db)
	if err != nil {
		return nil, err
	}

	var views []View
	var errs []error
	for key, raw := range all {
		if !strings.HasPrefix(key, ViewKeyPrefix) {
			continue
		}
		name := key[len(ViewKeyPrefix):]
		var v View
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			errs = append(errs, fmt.Errorf("view %s: %w", name, err))
			continue
		}
		v.Name = name
		views = append(views, v)
	}

	sort.Slice(views, func(i, j int) bool { return views[i].Name < views[j].Name })
	return views, errors.Join(errs...)
}

// GetView looks up one view by name.
func GetView(db *sql.DB, name string) (View, bool, error) {
	all, err := backlog.GetAllConfig(db)
	if err != nil {
		return View{}, false, err
	}
	raw, ok := all[ViewKeyPrefix+name]
	if !ok {
		return View{}, false, nil
	}
	var v View
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return View{}, false, err
	}
	v.Name = name
	return v, true, nil
}

// validViewName rejects view names that would let ViewOutputPath's default
// "<output dir>/<name>.ndjson" escape output_dir (e.g. via a path-traversal
// or path-separator name such as "../../tmp/evil").
func validViewName(name string) error {
	if name == "" {
		return fmt.Errorf("dashboard: view name must not be empty")
	}
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return fmt.Errorf("dashboard: invalid view name %q: must not contain '/', '\\', or '..'", name)
	}
	return nil
}

// SetView creates or replaces a view. Enforced here (not just at the CLI
// layer) so every caller gets the view-name safety check.
func SetView(db *sql.DB, v View) error {
	if err := validViewName(v.Name); err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return backlog.SetConfig(db, ViewKeyPrefix+v.Name, string(b))
}

// RemoveView deletes a view's config. It does not touch the view's runtime
// state; callers that want a clean removal should also clear that (see
// DeleteViewRuntime).
func RemoveView(db *sql.DB, name string) error {
	return backlog.DeleteConfig(db, ViewKeyPrefix+name)
}

// GetProjectConfig looks up per-project dashboard overrides. An absent
// project config is not an error: it returns the zero value.
func GetProjectConfig(db *sql.DB, project string) (ProjectConfig, error) {
	all, err := backlog.GetAllConfig(db)
	if err != nil {
		return ProjectConfig{}, err
	}
	raw, ok := all[ProjectKeyPrefix+project]
	if !ok {
		return ProjectConfig{}, nil
	}
	var pc ProjectConfig
	if err := json.Unmarshal([]byte(raw), &pc); err != nil {
		return ProjectConfig{}, err
	}
	return pc, nil
}

// SetProjectConfig creates or replaces a project's dashboard overrides.
func SetProjectConfig(db *sql.DB, project string, pc ProjectConfig) error {
	b, err := json.Marshal(pc)
	if err != nil {
		return err
	}
	return backlog.SetConfig(db, ProjectKeyPrefix+project, string(b))
}

// ResolveRepo resolves the on-disk repo path for a project: a per-project
// Repo override wins, else dashboard.workspace_root joined with the project
// name, else "" (gitSegment then falls back to ctx.Cwd/cwd auto-detection,
// leaving unconfigured behavior unchanged). It does not stat or validate the
// resolved path — gitSegment already degrades gracefully on a bad dir.
func ResolveRepo(db *sql.DB, project string) (string, error) {
	pc, err := GetProjectConfig(db, project)
	if err != nil {
		return "", err
	}
	if pc.Repo != "" {
		return pc.Repo, nil
	}

	root, err := backlog.GetConfig(db, KeyWorkspaceRoot)
	if err != nil {
		// backlog.GetConfig returns a generic error both for "key not
		// found" (the normal, expected case when no workspace root is
		// configured) and for a genuine DB failure. A real DB failure
		// would already have surfaced above, from GetProjectConfig's own
		// query — so failing open here (treat as "unset") never masks
		// it, and keeps the common unconfigured case error-free.
		return "", nil //nolint:nilerr // fail open: GetConfig doesn't distinguish not-found from a real error; a real DB failure already surfaced via GetProjectConfig above
	}
	if root == "" {
		return "", nil
	}
	// Never join a project name that could escape workspace_root (path
	// separator or ".." segment) — fail safe to unresolved rather than a
	// traversal path. Mirrors validViewName's safety check. The pc.Repo
	// override above is exempt: it's an operator-set full path, not
	// joined against anything.
	if strings.ContainsAny(project, "/\\") || strings.Contains(project, "..") {
		return "", nil
	}
	return filepath.Join(root, project), nil
}

// EffectiveSegments resolves the segment list for one project: an explicit
// per-project override wins, else the global dashboard.segments, else
// DefaultSegments. It reads the config table once (rather than delegating
// to GetProjectConfig, which would issue a second query) so there is a
// single, testable GetAllConfig error path.
func EffectiveSegments(db *sql.DB, project string) ([]SegmentSpec, error) {
	all, err := backlog.GetAllConfig(db)
	if err != nil {
		return nil, err
	}

	if raw, ok := all[ProjectKeyPrefix+project]; ok {
		var pc ProjectConfig
		if err := json.Unmarshal([]byte(raw), &pc); err != nil {
			return nil, err
		}
		if len(pc.Segments) > 0 {
			return pc.Segments, nil
		}
	}

	if raw, ok := all[KeySegments]; ok {
		return ParseSegments(raw)
	}
	return DefaultSegments(), nil
}

// MonitoredProjects returns the distinct, sorted union of every view's
// Projects — the answer to "what is ouroboros monitoring." A malformed
// view is excluded from the union but does not block the rest; its parse
// error is still returned (joined) for visibility.
func MonitoredProjects(db *sql.DB) ([]string, error) {
	views, err := ListViews(db)

	set := make(map[string]struct{})
	for _, v := range views {
		for _, p := range v.Projects {
			set[p] = struct{}{}
		}
	}
	projects := make([]string, 0, len(set))
	for p := range set {
		projects = append(projects, p)
	}
	sort.Strings(projects)
	return projects, err
}

// ViewCooldown resolves the refresh cooldown for a view: the view's own
// Cooldown if set, else the global dashboard.cooldown, else the default
// (ParseCooldown handles empty/invalid input).
func ViewCooldown(db *sql.DB, v View) time.Duration {
	if v.Cooldown != "" {
		return ParseCooldown(v.Cooldown)
	}
	raw, _ := backlog.GetConfig(db, KeyCooldown)
	return ParseCooldown(raw)
}

// GetViewRuntime looks up a view's runtime state. Absent runtime state is
// not an error: it returns the zero value (no prior refresh).
func GetViewRuntime(db *sql.DB, name string) (viewRuntime, error) {
	all, err := backlog.GetAllConfig(db)
	if err != nil {
		return viewRuntime{}, err
	}
	raw, ok := all[RuntimeKeyPrefix+name]
	if !ok {
		return viewRuntime{}, nil
	}
	var rt viewRuntime
	if err := json.Unmarshal([]byte(raw), &rt); err != nil {
		return viewRuntime{}, err
	}
	return rt, nil
}

// SetViewLastRefresh records a view's last-refresh timestamp (RFC3339).
func SetViewLastRefresh(db *sql.DB, name, ts string) error {
	b, err := json.Marshal(viewRuntime{LastRefresh: ts})
	if err != nil {
		return err
	}
	return backlog.SetConfig(db, RuntimeKeyPrefix+name, string(b))
}

// DeleteViewRuntime clears a view's runtime state (used alongside
// RemoveView when a view is deleted).
func DeleteViewRuntime(db *sql.DB, name string) error {
	return backlog.DeleteConfig(db, RuntimeKeyPrefix+name)
}

// OutputDir resolves the directory dashboard views write their NDJSON
// output into: dashboard.output_dir if set, else a "dashboards" directory
// next to the KB database file.
func OutputDir(db *sql.DB, dbPath string) string {
	if raw, err := backlog.GetConfig(db, KeyOutputDir); err == nil && raw != "" {
		return raw
	}
	return filepath.Join(filepath.Dir(dbPath), defaultOutputDirName)
}

// ViewOutputPath resolves the file a view refreshes to: the view's own
// Output override if set, else <output dir>/<view name>.ndjson.
func ViewOutputPath(db *sql.DB, v View, dbPath string) string {
	if v.Output != "" {
		return v.Output
	}
	return filepath.Join(OutputDir(db, dbPath), v.Name+".ndjson")
}

// MinCooldown floors any configured refresh cooldown.
const MinCooldown = 30 * time.Second

// defaultCooldown is used when dashboard.cooldown is unset or invalid.
const defaultCooldown = 60 * time.Second

// SegmentSpec declares one segment producer: exactly one of a built-in
// (Builtin), an argv command (Exec — execve, no shell), or a shell pipeline
// (Shell — opt-in "sh -c").
type SegmentSpec struct {
	ID string `json:"id"`
	// Every is reserved for the lifecycle slice (per-segment cadence); not
	// enforced yet — only the global dashboard.cooldown gate applies in
	// this slice.
	Every   string `json:"every,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
	// Timeout bounds an exec/shell producer's run time (dashboard.ParseTimeout):
	// defaults to 5s when empty/invalid, clamped to [1s, 30s]. Ignored for
	// Builtin (in-process, no subprocess timeout).
	Timeout string   `json:"timeout,omitempty"`
	Builtin string   `json:"builtin,omitempty"`
	Exec    []string `json:"exec,omitempty"`
	Shell   string   `json:"shell,omitempty"`
}

// IsEnabled reports whether the segment is enabled; a nil Enabled defaults
// to true.
func (s SegmentSpec) IsEnabled() bool {
	if s.Enabled == nil {
		return true
	}
	return *s.Enabled
}

// DefaultSegments is the segment list used when dashboard.segments is unset.
func DefaultSegments() []SegmentSpec {
	return []SegmentSpec{
		{ID: "git", Builtin: "git"},
		{ID: "roadmap", Builtin: "roadmap"},
		{ID: "tickets", Builtin: "tickets"},
		{ID: "kb", Builtin: "kb"},
	}
}

// ParseSegments unmarshals a dashboard.segments config value.
func ParseSegments(jsonStr string) ([]SegmentSpec, error) {
	var specs []SegmentSpec
	if err := json.Unmarshal([]byte(jsonStr), &specs); err != nil {
		return nil, err
	}
	return specs, nil
}

// ParseCooldown parses a dashboard.cooldown duration string, defaulting to
// 60s when empty or invalid, and flooring the result at MinCooldown.
func ParseCooldown(s string) time.Duration {
	d := defaultCooldown
	if s != "" {
		if parsed, err := time.ParseDuration(s); err == nil {
			d = parsed
		}
	}
	if d < MinCooldown {
		d = MinCooldown
	}
	return d
}
