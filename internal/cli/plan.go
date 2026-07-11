package cli

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/backlog"
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Manage implementation plans",
}

func runPlanCreate(w io.Writer, db *sql.DB, project, title, content, status, itemID string) error {
	var projectID *int64
	if project != "" {
		proj, err := backlog.GetProjectByName(db, project)
		if err != nil {
			return fmt.Errorf("plan create: %w", err)
		}
		projectID = &proj.ID
	}

	var itemIDPtr *string
	if itemID != "" {
		itemIDPtr = &itemID
	}

	plan, err := backlog.CreatePlan(db, title, content, projectID, itemIDPtr)
	if err != nil {
		return fmt.Errorf("plan create: %w", err)
	}

	if status != "" && status != "draft" {
		updated, err := backlog.UpdatePlan(db, plan.ID, map[string]string{"status": status})
		if err != nil {
			return fmt.Errorf("plan create: set status: %w", err)
		}
		plan = updated
	}

	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("plan create: marshal: %w", err)
	}
	fmt.Fprintln(w, string(data))
	return nil
}

func runPlanGet(w io.Writer, db *sql.DB, idStr string) error {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return fmt.Errorf("plan get: invalid id %q: %w", idStr, err)
	}
	plan, err := backlog.GetPlan(db, id)
	if err != nil {
		return fmt.Errorf("plan get: %w", err)
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("plan get: marshal: %w", err)
	}
	fmt.Fprintln(w, string(data))
	return nil
}

func runPlanUpdate(w io.Writer, db *sql.DB, idStr string, fields map[string]string) error {
	if len(fields) == 0 {
		return errors.New("plan update: at least one of --title, --content, --status is required")
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return fmt.Errorf("plan update: invalid id %q: %w", idStr, err)
	}
	plan, err := backlog.UpdatePlan(db, id, fields)
	if err != nil {
		return fmt.Errorf("plan update: %w", err)
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("plan update: marshal: %w", err)
	}
	fmt.Fprintln(w, string(data))
	return nil
}

func runPlanList(w io.Writer, db *sql.DB, project, status string) error {
	var f backlog.PlanFilter

	if project != "" {
		proj, err := backlog.GetProjectByName(db, project)
		if err != nil {
			return fmt.Errorf("plan list: %w", err)
		}
		f.ProjectIDs = []int64{proj.ID}
	}
	if status != "" {
		f.Status = &status
	}

	plans, err := backlog.ListPlans(db, f)
	if err != nil {
		return fmt.Errorf("plan list: %w", err)
	}
	if plans == nil {
		plans = []backlog.PlanSummary{}
	}
	data, err := json.MarshalIndent(plans, "", "  ")
	if err != nil {
		return fmt.Errorf("plan list: marshal: %w", err)
	}
	fmt.Fprintln(w, string(data))
	return nil
}

var (
	planCreateProject string
	planCreateTitle   string
	planCreateContent string
	planCreateStatus  string
	planCreateItemID  string
)

var planCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new plan",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if planCreateProject == "" {
			return errors.New("--project is required")
		}
		if planCreateTitle == "" {
			return errors.New("--title is required")
		}
		return withDB(func(db *sql.DB) error {
			return runPlanCreate(cmd.OutOrStdout(), db, planCreateProject, planCreateTitle, planCreateContent, planCreateStatus, planCreateItemID)
		})
	},
}

var planGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a plan by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runPlanGet(cmd.OutOrStdout(), db, args[0])
		})
	},
}

var (
	planUpdateTitle   string
	planUpdateContent string
	planUpdateStatus  string
)

var planUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a plan",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fields := make(map[string]string)
		if planUpdateTitle != "" {
			fields["title"] = planUpdateTitle
		}
		if planUpdateContent != "" {
			fields["content"] = planUpdateContent
		}
		if planUpdateStatus != "" {
			fields["status"] = planUpdateStatus
		}
		return withDB(func(db *sql.DB) error {
			return runPlanUpdate(cmd.OutOrStdout(), db, args[0], fields)
		})
	},
}

var (
	planListProject string
	planListStatus  string
	planListItemID  string
)

var planListCmd = &cobra.Command{
	Use:   "list",
	Short: "List plans",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runPlanList(cmd.OutOrStdout(), db, planListProject, planListStatus)
		})
	},
}

func init() {
	planCreateCmd.Flags().StringVar(&planCreateProject, "project", "", "Project name (required)")
	planCreateCmd.Flags().StringVar(&planCreateTitle, "title", "", "Plan title (required)")
	planCreateCmd.Flags().StringVar(&planCreateContent, "content", "", "Plan content")
	planCreateCmd.Flags().StringVar(&planCreateStatus, "status", "", "Initial status (default: draft)")
	planCreateCmd.Flags().StringVar(&planCreateItemID, "item-id", "", "Associated backlog item ID")

	planUpdateCmd.Flags().StringVar(&planUpdateTitle, "title", "", "New title")
	planUpdateCmd.Flags().StringVar(&planUpdateContent, "content", "", "New content")
	planUpdateCmd.Flags().StringVar(&planUpdateStatus, "status", "", "New status")

	planListCmd.Flags().StringVar(&planListProject, "project", "", "Filter by project name")
	planListCmd.Flags().StringVar(&planListStatus, "status", "", "Filter by status")
	planListCmd.Flags().StringVar(&planListItemID, "item-id", "", "Filter by item ID (reserved; not yet supported by list filter)")

	planCmd.AddCommand(planCreateCmd)
	planCmd.AddCommand(planGetCmd)
	planCmd.AddCommand(planUpdateCmd)
	planCmd.AddCommand(planListCmd)
}
