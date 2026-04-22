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
	lsItemsProjectFlag   string
	lsItemsStatusFlag    string
	lsItemsPriorityFlag  string
	lsItemsComponentFlag string
	lsItemsJSONFlag      bool
)

var lsItemsCmd = &cobra.Command{
	Use:   "items [ID]",
	Short: "List backlog items",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := store.InitDB()
		if err != nil {
			return fmt.Errorf("ls items: open database: %w", err)
		}
		defer db.Close()

		if len(args) == 1 {
			return runLSItemDetail(cmd.OutOrStdout(), db, args[0], lsItemsJSONFlag)
		}
		return runLSItems(cmd.OutOrStdout(), db, lsItemsProjectFlag, lsItemsStatusFlag, lsItemsPriorityFlag, lsItemsComponentFlag, lsItemsJSONFlag)
	},
}

func init() {
	lsItemsCmd.Flags().StringVar(&lsItemsProjectFlag, "project", "", "Project name filter")
	lsItemsCmd.Flags().StringVar(&lsItemsStatusFlag, "status", "", "Status filter (open or done)")
	lsItemsCmd.Flags().StringVar(&lsItemsPriorityFlag, "priority", "", "Priority filter (P0-P6)")
	lsItemsCmd.Flags().StringVar(&lsItemsComponentFlag, "component", "", "Component filter")
	lsItemsCmd.Flags().BoolVar(&lsItemsJSONFlag, "json", false, "Output as JSON")
}

func runLSItems(out io.Writer, db *sql.DB, projectName, status, priority, component string, asJSON bool) error {
	// Build project filter
	var projectIDs []int64
	if projectName != "" {
		project, _ := backlog.GetProjectByName(db, projectName)
		if project == nil {
			// No match; print empty results
			if asJSON {
				return printJSON(out, []backlog.Item{})
			}
			return printTable(out, []string{"ID", "PRIORITY", "STATUS", "PROJECT", "COMPONENT", "TITLE"}, [][]string{})
		}
		projectIDs = []int64{project.ID}
	} else {
		// All projects
		projects, err := backlog.ListProjects(db)
		if err != nil {
			return fmt.Errorf("ls items: list projects: %w", err)
		}
		for _, p := range projects {
			projectIDs = append(projectIDs, p.ID)
		}
	}

	// Build filter
	filter := backlog.ItemFilter{ProjectIDs: projectIDs}
	if status != "" {
		filter.Status = &status
	}
	if component != "" {
		filter.Component = &component
	}
	if priority != "" {
		// Parse priority: P0 -> 0, P6 -> 6
		if len(priority) == 2 && priority[0] == 'P' {
			if p, err := strconv.Atoi(string(priority[1])); err == nil && p >= 0 && p <= 6 {
				filter.PriorityMin = &p
				filter.PriorityMax = &p
			}
		}
	}

	items, err := backlog.ListItems(db, filter)
	if err != nil {
		return fmt.Errorf("ls items: list failed: %w", err)
	}

	if asJSON {
		return printJSON(out, items)
	}

	// Build project name map
	projectMap := make(map[int64]string)
	projects, err := backlog.ListProjects(db)
	if err != nil {
		return fmt.Errorf("ls items: list projects: %w", err)
	}
	for _, p := range projects {
		projectMap[p.ID] = p.Name
	}

	// Build table rows
	var rows [][]string
	for _, item := range items {
		rows = append(rows, []string{
			item.ID,
			item.Priority,
			item.Status,
			projectMap[item.ProjectID],
			item.Component,
			item.Title,
		})
	}

	return printTable(out, []string{"ID", "PRIORITY", "STATUS", "PROJECT", "COMPONENT", "TITLE"}, rows)
}

func runLSItemDetail(out io.Writer, db *sql.DB, id string, asJSON bool) error {
	item, err := backlog.GetItem(db, id)
	if err != nil {
		return fmt.Errorf("ls items: get item: %w", err)
	}

	if asJSON {
		return printJSON(out, item)
	}

	projectName := ""
	if item.ProjectID > 0 {
		project, _ := backlog.GetProjectByID(db, item.ProjectID)
		if project != nil {
			projectName = project.Name
		}
	}

	formatItemDetail(out, item, projectName)
	return nil
}
