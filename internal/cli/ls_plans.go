package cli

import (
	"database/sql"
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/store"
)

var (
	lsPlanProjectFlag string
	lsPlanStatusFlag  string
	lsPlanJSONFlag    bool
)

var lsPlansCmd = &cobra.Command{
	Use:   "plans [ID]",
	Short: "List implementation plans",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := store.InitDB()
		if err != nil {
			return fmt.Errorf("ls plans: open database: %w", err)
		}
		defer db.Close()

		if len(args) == 1 {
			return runLSPlanDetail(cmd.OutOrStdout(), db, args[0], lsPlanJSONFlag)
		}
		return runLSPlans(cmd.OutOrStdout(), db, lsPlanProjectFlag, lsPlanStatusFlag, lsPlanJSONFlag)
	},
}

func init() {
	lsPlansCmd.Flags().StringVar(&lsPlanProjectFlag, "project", "", "Project name filter")
	lsPlansCmd.Flags().StringVar(&lsPlanStatusFlag, "status", "", "Status filter (draft, active, complete)")
	lsPlansCmd.Flags().BoolVar(&lsPlanJSONFlag, "json", false, "Output as JSON")
}

func runLSPlans(out io.Writer, db *sql.DB, projectName, status string, asJSON bool) error {
	// Build project filter
	var projectIDs []int64
	if projectName != "" {
		project, _ := backlog.GetProjectByName(db, projectName)
		if project == nil {
			// No match; print empty results
			if asJSON {
				return printJSON(out, []backlog.Plan{})
			}
			return printTable(out, []string{"ID", "STATUS", "PROJECT", "TITLE"}, [][]string{})
		}
		projectIDs = []int64{project.ID}
	} else {
		// All projects
		projects, err := backlog.ListProjects(db)
		if err != nil {
			return fmt.Errorf("ls plans: list projects: %w", err)
		}
		for _, p := range projects {
			projectIDs = append(projectIDs, p.ID)
		}
	}

	// Build filter
	filter := backlog.PlanFilter{ProjectIDs: projectIDs}
	if status != "" {
		filter.Status = &status
	}

	plans, err := backlog.ListPlans(db, filter)
	if err != nil {
		return fmt.Errorf("ls plans: list failed: %w", err)
	}

	if asJSON {
		return printJSON(out, plans)
	}

	// Build project name map
	projectMap := make(map[int64]string)
	projects, err := backlog.ListProjects(db)
	if err != nil {
		return fmt.Errorf("ls plans: list projects: %w", err)
	}
	for _, p := range projects {
		projectMap[p.ID] = p.Name
	}

	// Build table rows
	var rows [][]string
	for _, plan := range plans {
		projectName := ""
		if plan.ProjectID != nil {
			projectName = projectMap[*plan.ProjectID]
		}
		rows = append(rows, []string{
			strconv.FormatInt(plan.ID, 10),
			plan.Status,
			projectName,
			plan.Title,
		})
	}

	return printTable(out, []string{"ID", "STATUS", "PROJECT", "TITLE"}, rows)
}

func runLSPlanDetail(out io.Writer, db *sql.DB, idStr string, asJSON bool) error {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return fmt.Errorf("ls plans: invalid id: %w", err)
	}

	plan, err := backlog.GetPlan(db, id)
	if err != nil {
		return fmt.Errorf("ls plans: get plan: %w", err)
	}

	if asJSON {
		return printJSON(out, plan)
	}

	projectName := ""
	if plan.ProjectID != nil {
		project, _ := backlog.GetProjectByID(db, *plan.ProjectID)
		if project != nil {
			projectName = project.Name
		}
	}

	formatPlanDetail(out, plan, projectName)
	return nil
}
