package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/backlog"
)

var (
	itemsProjectFlag string
	itemsStatusFlag  string
	itemsLimitFlag   int
)

var itemsCmd = &cobra.Command{
	Use:   "items",
	Short: "List backlog items (CLI for hook integration)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runItems(cmd.OutOrStdout(), db, itemsProjectFlag, itemsStatusFlag)
		})
	},
}

func init() {
	itemsCmd.Flags().StringVar(&itemsProjectFlag, "project", "", "Project name filter")
	itemsCmd.Flags().StringVar(&itemsStatusFlag, "status", "open", "Status filter (open or done)")
	itemsCmd.Flags().IntVar(&itemsLimitFlag, "limit", 20, "Maximum number of results")
}

func runItems(out io.Writer, db *sql.DB, projectName, status string) error {
	project, _ := backlog.GetProjectByName(db, projectName)
	if project == nil {
		// Preserve the existing behavior: project not found → print [] and succeed.
		// The current implementation swallows this silently; keep that contract.
		fmt.Fprintln(out, "[]")
		return nil
	}

	filter := backlog.ItemFilter{ProjectIDs: []int64{project.ID}, Limit: itemsLimitFlag}
	if status != "" {
		filter.Status = &status
	}

	items, err := backlog.ListItems(db, filter)
	if err != nil {
		return fmt.Errorf("items: list failed: %w", err)
	}

	data, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("items: marshal failed: %w", err)
	}

	fmt.Fprintln(out, string(data))
	return nil
}
