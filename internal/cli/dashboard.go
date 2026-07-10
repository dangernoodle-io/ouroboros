package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/config"
	"dangernoodle.io/ouroboros/internal/dashboard"
)

var (
	dashboardRefreshForce    bool
	dashboardRefreshView     string
	dashboardRefreshAll      bool
	dashboardRefreshProjects string

	dashboardViewSetProjects string
	dashboardViewSetCooldown string
	dashboardViewSetOutput   string

	dashboardProjectSetSegments string
	dashboardProjectSetRepo     string
)

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Dashboard data-capture: segment producers, views, and NDJSON refresh",
}

var dashboardSegmentCmd = &cobra.Command{
	Use:   "segment <name>",
	Short: "Run one built-in segment producer, emitting NDJSON fragments to stdout",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runDashboardSegment(cmd.InOrStdin(), cmd.OutOrStdout(), db, args[0])
		})
	},
}

var dashboardRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Refresh one, several, or every dashboard view's NDJSON output",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runDashboardRefresh(cmd.OutOrStdout(), db, dashboardRefreshOpts{
				force:    dashboardRefreshForce,
				view:     dashboardRefreshView,
				all:      dashboardRefreshAll,
				projects: dashboardRefreshProjects,
			})
		})
	},
}

var dashboardViewCmd = &cobra.Command{
	Use:   "view",
	Short: "Manage dashboard views (named project sets)",
}

var dashboardViewSetCmd = &cobra.Command{
	Use:   "set <name>",
	Short: "Create or replace a view",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runDashboardViewSet(cmd.OutOrStdout(), db, args[0], dashboardViewSetProjects, dashboardViewSetCooldown, dashboardViewSetOutput)
		})
	},
}

var dashboardViewListCmd = &cobra.Command{
	Use:   "list",
	Short: "List every configured view",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runDashboardStatus(cmd.OutOrStdout(), db)
		})
	},
}

var dashboardViewRmCmd = &cobra.Command{
	Use:   "rm <name>",
	Short: "Remove a view",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runDashboardViewRm(cmd.OutOrStdout(), db, args[0])
		})
	},
}

var dashboardProjectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage per-project dashboard overrides",
}

var dashboardProjectSetCmd = &cobra.Command{
	Use:   "set <name>",
	Short: "Set a project's segment overrides and/or repo path",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runDashboardProjectSet(cmd.OutOrStdout(), db, args[0], dashboardProjectSetOpts{
				segmentsJSON:    dashboardProjectSetSegments,
				repo:            dashboardProjectSetRepo,
				segmentsChanged: cmd.Flags().Changed("segments"),
				repoChanged:     cmd.Flags().Changed("repo"),
			})
		})
	},
}

var dashboardStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show dashboard master toggle, output dir, views, and monitored projects",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runDashboardStatus(cmd.OutOrStdout(), db)
		})
	},
}

func init() {
	dashboardRefreshCmd.Flags().BoolVar(&dashboardRefreshForce, "force", false, "Bypass the refresh cooldown")
	dashboardRefreshCmd.Flags().StringVar(&dashboardRefreshView, "view", "", "Refresh only this saved view")
	dashboardRefreshCmd.Flags().BoolVar(&dashboardRefreshAll, "all", false, "Refresh every saved view (default when no other target flag is given)")
	dashboardRefreshCmd.Flags().StringVar(&dashboardRefreshProjects, "projects", "", "Refresh an ephemeral view over this comma-separated project list")

	dashboardViewSetCmd.Flags().StringVar(&dashboardViewSetProjects, "projects", "", "Comma-separated project list (required)")
	dashboardViewSetCmd.Flags().StringVar(&dashboardViewSetCooldown, "cooldown", "", "Per-view refresh cooldown override (e.g. 5m)")
	dashboardViewSetCmd.Flags().StringVar(&dashboardViewSetOutput, "output", "", "Explicit output file path override")

	dashboardProjectSetCmd.Flags().StringVar(&dashboardProjectSetSegments, "segments", "", "Segment specs as a JSON array")
	dashboardProjectSetCmd.Flags().StringVar(&dashboardProjectSetRepo, "repo", "", "On-disk repo path override for this project")

	dashboardViewCmd.AddCommand(dashboardViewSetCmd)
	dashboardViewCmd.AddCommand(dashboardViewListCmd)
	dashboardViewCmd.AddCommand(dashboardViewRmCmd)

	dashboardProjectCmd.AddCommand(dashboardProjectSetCmd)

	dashboardCmd.AddCommand(dashboardSegmentCmd)
	dashboardCmd.AddCommand(dashboardRefreshCmd)
	dashboardCmd.AddCommand(dashboardViewCmd)
	dashboardCmd.AddCommand(dashboardProjectCmd)
	dashboardCmd.AddCommand(dashboardStatusCmd)
}

// resolveDashboardContext builds a base Context from cwd/.git auto-detection,
// mirroring statusline.go's project-detection approach.
func resolveDashboardContext() dashboard.Context {
	ctx := dashboard.Context{
		Schema: 1,
		Now:    time.Now().UTC().Format(time.RFC3339),
	}

	cwd, err := os.Getwd()
	if err != nil {
		return ctx
	}
	ctx.Cwd = cwd

	gitPath := filepath.Join(cwd, ".git")
	if _, err := os.Stat(gitPath); err == nil {
		ctx.Project = filepath.Base(cwd)
		ctx.Repo = cwd
	}
	return ctx
}

func runDashboardSegment(in io.Reader, out io.Writer, db *sql.DB, name string) error {
	prov, ok := dashboard.Builtin(name)
	if !ok {
		return fmt.Errorf("dashboard segment: unknown builtin %q (available: %s)", name, strings.Join(dashboard.BuiltinNames(), ", "))
	}

	raw, err := io.ReadAll(in)
	if err != nil {
		return fmt.Errorf("dashboard segment: read stdin: %w", err)
	}

	var ctx dashboard.Context
	if len(bytes.TrimSpace(raw)) == 0 {
		ctx = resolveDashboardContext()
	} else {
		if err := json.Unmarshal(raw, &ctx); err != nil {
			return fmt.Errorf("dashboard segment: parse context: %w", err)
		}
	}

	frags, err := prov(ctx, db)
	if err != nil {
		return fmt.Errorf("dashboard segment %s: %w", name, err)
	}
	if len(frags) == 0 {
		return nil
	}
	return dashboard.Emit(out, frags)
}

// runDashboardViewSet creates or replaces a view.
func runDashboardViewSet(out io.Writer, db *sql.DB, name, projectsCSV, cooldown, output string) error {
	projects := splitCSV(projectsCSV)
	if len(projects) == 0 {
		return fmt.Errorf("dashboard view set: --projects is required and must be non-empty")
	}

	v := dashboard.View{Name: name, Projects: projects, Cooldown: cooldown, Output: output}
	if err := dashboard.SetView(db, v); err != nil {
		return fmt.Errorf("dashboard view set: %w", err)
	}

	fmt.Fprintf(out, "set view %s (%d projects)\n", name, len(projects))
	return nil
}

// runDashboardViewRm removes a view and its runtime state.
func runDashboardViewRm(out io.Writer, db *sql.DB, name string) error {
	if err := dashboard.RemoveView(db, name); err != nil {
		return fmt.Errorf("dashboard view rm: %w", err)
	}
	if err := dashboard.DeleteViewRuntime(db, name); err != nil {
		return fmt.Errorf("dashboard view rm: %w", err)
	}
	fmt.Fprintf(out, "removed view %s\n", name)
	return nil
}

// dashboardProjectSetOpts bundles the `project set` command's flags plus
// whether each was explicitly passed, so the handler can read-modify-write
// without an unset flag wiping the other field.
type dashboardProjectSetOpts struct {
	segmentsJSON    string
	repo            string
	segmentsChanged bool
	repoChanged     bool
}

// runDashboardProjectSet sets a project's segment overrides and/or repo path
// override, read-modify-write: only the fields whose flags were explicitly
// passed are updated, so `--repo` alone never clears existing segments and
// vice versa.
func runDashboardProjectSet(out io.Writer, db *sql.DB, project string, opts dashboardProjectSetOpts) error {
	if !opts.segmentsChanged && !opts.repoChanged {
		return fmt.Errorf("dashboard project set: nothing to set: pass --repo and/or --segments")
	}

	pc, err := dashboard.GetProjectConfig(db, project)
	if err != nil {
		return fmt.Errorf("dashboard project set: %w", err)
	}

	if opts.segmentsChanged {
		specs, err := dashboard.ParseSegments(opts.segmentsJSON)
		if err != nil {
			return fmt.Errorf("dashboard project set: parse segments: %w", err)
		}
		pc.Segments = specs
	}
	if opts.repoChanged {
		pc.Repo = opts.repo
	}

	if err := dashboard.SetProjectConfig(db, project, pc); err != nil {
		return fmt.Errorf("dashboard project set: %w", err)
	}

	repoDisplay := pc.Repo
	if repoDisplay == "" {
		repoDisplay = "(unset)"
	}
	fmt.Fprintf(out, "set project %s (repo=%s, %d segments)\n", project, repoDisplay, len(pc.Segments))
	return nil
}

// runDashboardStatus prints the master toggle, output dir, every view
// (members + last_refresh), and the union of monitored projects. It backs
// both `dashboard status` and `dashboard view list`.
func runDashboardStatus(out io.Writer, db *sql.DB) error {
	enabled, _ := backlog.GetConfig(db, dashboard.KeyEnabled)
	if enabled == "" {
		enabled = "false"
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("dashboard status: load config: %w", err)
	}
	outputDir := dashboard.OutputDir(db, cfg.DBPath)

	workspaceRoot, _ := backlog.GetConfig(db, dashboard.KeyWorkspaceRoot)
	if workspaceRoot == "" {
		workspaceRoot = "(unset)"
	}

	fmt.Fprintf(out, "dashboard.enabled: %s\n", enabled)
	fmt.Fprintf(out, "output_dir: %s\n", outputDir)
	fmt.Fprintf(out, "workspace_root: %s\n", workspaceRoot)

	views, err := dashboard.ListViews(db)
	if err != nil {
		fmt.Fprintf(out, "warning: %s\n", err.Error())
	}

	if len(views) == 0 {
		fmt.Fprintln(out, "no views configured")
	}
	for _, v := range views {
		rt, rtErr := dashboard.GetViewRuntime(db, v.Name)
		lastRefresh := "never"
		if rtErr == nil && rt.LastRefresh != "" {
			lastRefresh = rt.LastRefresh
		}
		fmt.Fprintf(out, "view %s: [%s] last_refresh=%s%s\n", v.Name, strings.Join(v.Projects, ", "), lastRefresh, segmentOverrideNote(db, v.Projects))
	}

	monitored, monErr := dashboard.MonitoredProjects(db)
	if monErr != nil {
		fmt.Fprintf(out, "warning: %s\n", monErr.Error())
	}
	fmt.Fprintf(out, "monitored projects (%d): %s\n", len(monitored), strings.Join(monitored, ", "))
	for _, p := range monitored {
		repo, err := dashboard.ResolveRepo(db, p)
		if err != nil {
			fmt.Fprintf(out, "  %s: repo=(error)\n", p)
			continue
		}
		if repo == "" {
			repo = "(unresolved)"
		}
		fmt.Fprintf(out, "  %s: repo=%s\n", p, repo)
	}
	return nil
}

// segmentOverrideNote returns a trailing note listing which of a view's
// projects carry a per-project segment override, or "" if none do.
func segmentOverrideNote(db *sql.DB, projects []string) string {
	var overridden []string
	for _, p := range projects {
		pc, err := dashboard.GetProjectConfig(db, p)
		if err == nil && len(pc.Segments) > 0 {
			overridden = append(overridden, p)
		}
	}
	if len(overridden) == 0 {
		return ""
	}
	return fmt.Sprintf(" (segment overrides: %s)", strings.Join(overridden, ", "))
}

// dashboardRefreshOpts bundles the refresh command's target-selection flags.
type dashboardRefreshOpts struct {
	force    bool
	view     string
	all      bool
	projects string
}

// runDashboardRefresh resolves the target view(s) from opts and refreshes
// each: master-toggle gate, cooldown check, per-project segment run,
// project-stamped fragment accumulation, and one NDJSON file per view.
func runDashboardRefresh(out io.Writer, db *sql.DB, opts dashboardRefreshOpts) error {
	enabled, getErr := backlog.GetConfig(db, dashboard.KeyEnabled)
	if getErr != nil || enabled != "true" {
		fmt.Fprintln(out, "dashboard disabled")
		return nil //nolint:nilerr // "not found"/any lookup failure degrades to the same disabled message, not a hard error
	}

	targets, adhoc, err := resolveRefreshTargets(out, db, opts)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		fmt.Fprintln(out, "no views configured (dashboard view set <name> --projects ...)")
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("dashboard refresh: load config: %w", err)
	}

	now := time.Now().UTC()
	for _, v := range targets {
		if err := refreshOneView(out, db, cfg.DBPath, v, now, opts.force, adhoc); err != nil {
			return err
		}
	}
	return nil
}

// resolveRefreshTargets picks the view(s) to refresh from the mutually
// exclusive --view/--all/--projects flags, defaulting to every saved view.
//
// The --all/default path tolerates a malformed view blob: it proceeds with
// every view that DID parse and prints a warning to out, rather than
// blocking refresh of every good view over one corrupt one (matching
// `status`/`view list`). A specific --view <name> still errors outright —
// the caller asked for that one view by name.
func resolveRefreshTargets(out io.Writer, db *sql.DB, opts dashboardRefreshOpts) (targets []dashboard.View, adhoc bool, err error) {
	selected := 0
	if opts.view != "" {
		selected++
	}
	if opts.all {
		selected++
	}
	if opts.projects != "" {
		selected++
	}
	if selected > 1 {
		return nil, false, fmt.Errorf("dashboard refresh: --view, --all, and --projects are mutually exclusive")
	}

	switch {
	case opts.projects != "":
		projects := splitCSV(opts.projects)
		return []dashboard.View{{Name: "_adhoc", Projects: projects}}, true, nil
	case opts.view != "":
		v, ok, getErr := dashboard.GetView(db, opts.view)
		if getErr != nil {
			return nil, false, fmt.Errorf("dashboard refresh: %w", getErr)
		}
		if !ok {
			return nil, false, fmt.Errorf("dashboard refresh: unknown view %q", opts.view)
		}
		return []dashboard.View{v}, false, nil
	default:
		views, listErr := dashboard.ListViews(db)
		if listErr != nil {
			if len(views) == 0 {
				return nil, false, fmt.Errorf("dashboard refresh: %w", listErr)
			}
			fmt.Fprintf(out, "warning: some views failed to load: %s\n", listErr.Error())
		}
		return views, false, nil
	}
}

// refreshOneView refreshes a single view: cooldown gate, per-project
// segment run, project-stamped accumulation, and one NDJSON output file.
func refreshOneView(out io.Writer, db *sql.DB, dbPath string, v dashboard.View, now time.Time, force, adhoc bool) error {
	if !adhoc && !force {
		rt, err := dashboard.GetViewRuntime(db, v.Name)
		if err == nil && rt.LastRefresh != "" {
			if last, parseErr := time.Parse(time.RFC3339, rt.LastRefresh); parseErr == nil {
				cooldown := dashboard.ViewCooldown(db, v)
				if remaining := cooldown - now.Sub(last); remaining > 0 {
					fmt.Fprintf(out, "view %s: within cooldown (%s remaining); use --force\n", v.Name, remaining.Round(time.Second))
					return nil
				}
			}
		}
	}

	var accumulated []dashboard.Fragment
	var drops []string

	for _, project := range v.Projects {
		specs, err := dashboard.EffectiveSegments(db, project)
		if err != nil {
			drops = append(drops, fmt.Sprintf("%s: effective segments: %s", project, err.Error()))
			continue
		}

		ctx := resolveDashboardContext()
		ctx.Project = project
		ctx.Now = now.Format(time.RFC3339)

		repo, err := dashboard.ResolveRepo(db, project)
		if err != nil {
			drops = append(drops, fmt.Sprintf("%s: resolve repo: %s", project, err.Error()))
			continue
		}
		if repo != "" {
			ctx.Repo = repo
			ctx.Cwd = repo
		}

		for _, spec := range specs {
			if !spec.IsEnabled() {
				continue
			}

			stdout, stderr, err := dashboard.RunSegment(db, spec, ctx)
			if err != nil {
				drops = append(drops, fmt.Sprintf("%s/%s: %s", project, spec.ID, err.Error()))
				continue
			}
			if strings.TrimSpace(stderr) != "" {
				drops = append(drops, fmt.Sprintf("%s/%s: stderr: %s", project, spec.ID, truncateStr(stderr, 200)))
			}
			if len(stdout) == 0 {
				continue
			}

			res, err := dashboard.Parse(bytes.NewReader(stdout))
			if err != nil {
				drops = append(drops, fmt.Sprintf("%s/%s: parse: %s", project, spec.ID, err.Error()))
				continue
			}
			accumulated = append(accumulated, dashboard.StampProject(res.Fragments, project)...)
			for range res.Dropped {
				drops = append(drops, fmt.Sprintf("%s/%s: dropped fragment", project, spec.ID))
			}
		}
	}

	outputPath := dashboard.ViewOutputPath(db, v, dbPath)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("dashboard refresh: create output dir: %w", err)
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("dashboard refresh: create output file: %w", err)
	}
	writeErr := dashboard.Emit(f, accumulated)
	closeErr := f.Close()
	if writeErr != nil {
		return fmt.Errorf("dashboard refresh: write output: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("dashboard refresh: close output file: %w", closeErr)
	}

	htmlPath := dashboardHTMLPath(outputPath)
	// ViewCooldown/ParseCooldown already floor the cooldown at MinCooldown
	// (30s), so refreshSecs is always >=30 here — no separate floor needed.
	refreshSecs := int(dashboard.ViewCooldown(db, v).Seconds())
	page := dashboard.RenderHTML(v.Name, accumulated, refreshSecs)
	if err := os.WriteFile(htmlPath, []byte(page), 0o644); err != nil { //nolint:gosec // dashboard page, world-readable is fine
		return fmt.Errorf("dashboard refresh: write html: %w", err)
	}

	if !adhoc {
		if err := dashboard.SetViewLastRefresh(db, v.Name, now.Format(time.RFC3339)); err != nil {
			return fmt.Errorf("dashboard refresh: save last_refresh: %w", err)
		}
	}

	fmt.Fprintf(out, "refreshed view %s (%d projects): %d fragments (%d dropped) -> %s + %s\n",
		v.Name, len(v.Projects), len(accumulated), len(drops), outputPath, htmlPath)
	return nil
}

// dashboardHTMLPath derives a view's HTML output path from its NDJSON output
// path: the ".ndjson" extension (if present) is swapped for ".html";
// otherwise ".html" is appended, so a custom --output without that suffix
// still gets a sensible sibling file rather than colliding with it.
func dashboardHTMLPath(outputPath string) string {
	if ext := filepath.Ext(outputPath); ext == ".ndjson" {
		return strings.TrimSuffix(outputPath, ext) + ".html"
	}
	return outputPath + ".html"
}

// truncateStr shortens s to at most max bytes, appending an ellipsis marker
// when truncated.
func truncateStr(s string, limit int) string {
	s = strings.TrimSpace(s)
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "…"
}

// splitCSV splits a comma-separated list, trimming whitespace and dropping
// empty entries.
func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
