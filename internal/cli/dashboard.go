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

var dashboardRefreshForce bool

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Dashboard data-capture: segment producers and NDJSON refresh",
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
	Short: "Run every enabled segment and write the collected NDJSON output",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runDashboardRefresh(cmd.OutOrStdout(), db, dashboardRefreshForce)
		})
	},
}

func init() {
	dashboardRefreshCmd.Flags().BoolVar(&dashboardRefreshForce, "force", false, "Bypass the refresh cooldown")

	dashboardCmd.AddCommand(dashboardSegmentCmd)
	dashboardCmd.AddCommand(dashboardRefreshCmd)
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

func runDashboardRefresh(out io.Writer, db *sql.DB, force bool) error {
	enabled, err := backlog.GetConfig(db, dashboard.KeyEnabled)
	if err != nil || enabled != "true" {
		fmt.Fprintln(out, "dashboard disabled")
		return nil
	}

	specs := dashboard.DefaultSegments()
	if raw, err := backlog.GetConfig(db, dashboard.KeySegments); err == nil {
		parsed, err := dashboard.ParseSegments(raw)
		if err != nil {
			return fmt.Errorf("dashboard refresh: parse segments: %w", err)
		}
		specs = parsed
	}

	now := time.Now().UTC()

	cooldownRaw, _ := backlog.GetConfig(db, dashboard.KeyCooldown)
	cooldown := dashboard.ParseCooldown(cooldownRaw)

	if !force {
		if lastRaw, err := backlog.GetConfig(db, dashboard.KeyLastRefresh); err == nil {
			// Corrupt/unparsable last_refresh (only ever self-written) fails
			// open: treat as no prior refresh rather than blocking; --force
			// remains available.
			if last, err := time.Parse(time.RFC3339, lastRaw); err == nil {
				if remaining := cooldown - now.Sub(last); remaining > 0 {
					fmt.Fprintf(out, "dashboard: within cooldown (%s remaining); use --force\n", remaining.Round(time.Second))
					return nil
				}
			}
		}
	}

	ctx := resolveDashboardContext()
	ctx.Now = now.Format(time.RFC3339)

	var attempted int
	var accumulated []dashboard.Fragment
	var drops []string

	for _, spec := range specs {
		if !spec.IsEnabled() {
			continue
		}
		attempted++

		if spec.Builtin == "" {
			drops = append(drops, fmt.Sprintf("%s: exec/shell producers not yet supported", spec.ID))
			continue
		}

		prov, ok := dashboard.Builtin(spec.Builtin)
		if !ok {
			drops = append(drops, fmt.Sprintf("%s: unknown builtin: %s", spec.ID, spec.Builtin))
			continue
		}

		frags, err := prov(ctx, db)
		if err != nil {
			drops = append(drops, fmt.Sprintf("%s: %s", spec.ID, err.Error()))
			continue
		}
		if len(frags) == 0 {
			continue
		}

		var buf bytes.Buffer
		if err := dashboard.Emit(&buf, frags); err != nil {
			drops = append(drops, fmt.Sprintf("%s: emit: %s", spec.ID, err.Error()))
			continue
		}

		res, err := dashboard.Parse(&buf)
		if err != nil {
			drops = append(drops, fmt.Sprintf("%s: parse: %s", spec.ID, err.Error()))
			continue
		}
		accumulated = append(accumulated, res.Fragments...)
		for range res.Dropped {
			drops = append(drops, fmt.Sprintf("%s: dropped fragment", spec.ID))
		}
	}

	outputPath, err := backlog.GetConfig(db, dashboard.KeyOutput)
	if err != nil {
		cfg, cfgErr := config.Load()
		if cfgErr != nil {
			return fmt.Errorf("dashboard refresh: load config: %w", cfgErr)
		}
		outputPath = filepath.Join(filepath.Dir(cfg.DBPath), "dashboard.ndjson")
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("dashboard refresh: create output dir: %w", err)
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("dashboard refresh: create output file: %w", err)
	}
	defer f.Close() //nolint:errcheck

	if err := dashboard.Emit(f, accumulated); err != nil {
		return fmt.Errorf("dashboard refresh: write output: %w", err)
	}

	if err := backlog.SetConfig(db, dashboard.KeyLastRefresh, now.Format(time.RFC3339)); err != nil {
		return fmt.Errorf("dashboard refresh: save last_refresh: %w", err)
	}

	fmt.Fprintf(out, "refreshed %d segments, %d fragments (%d dropped) -> %s\n",
		attempted, len(accumulated), len(drops), outputPath)
	return nil
}
