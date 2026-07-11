package cli

import (
	"database/sql"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/backlog"
)

var (
	lsItemsProjectFlag   string
	lsItemsStatusFlag    string
	lsItemsPriorityFlag  string
	lsItemsComponentFlag string
	lsItemsEpicFlag      string
	lsItemsEpicsFlag     bool
	lsItemsSinceFlag     string
	lsItemsSortFlag      string
	lsItemsLimitFlag     int
	lsItemsJSONFlag      bool
)

var lsItemsCmd = &cobra.Command{
	Use:   "backlog [ID]",
	Short: "List backlog items",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			if len(args) == 1 {
				return runLSItemDetail(cmd.OutOrStdout(), db, args[0], lsItemsJSONFlag)
			}
			return runLSItems(cmd.OutOrStdout(), db, lsItemsProjectFlag, lsItemsStatusFlag, lsItemsPriorityFlag, lsItemsComponentFlag, lsItemsEpicFlag, lsItemsEpicsFlag, lsItemsSinceFlag, lsItemsSortFlag, lsItemsLimitFlag, lsItemsJSONFlag)
		})
	},
}

func init() {
	lsItemsCmd.Flags().StringVar(&lsItemsProjectFlag, "project", "", "Project name filter")
	lsItemsCmd.Flags().StringVar(&lsItemsStatusFlag, "status", "", "Status filter (open or done)")
	lsItemsCmd.Flags().StringVar(&lsItemsPriorityFlag, "priority", "", "Priority filter (P0-P6)")
	lsItemsCmd.Flags().StringVar(&lsItemsComponentFlag, "component", "", "Component filter")
	lsItemsCmd.Flags().StringVar(&lsItemsEpicFlag, "epic", "", "Epic item id filter (that epic's children)")
	lsItemsCmd.Flags().BoolVar(&lsItemsEpicsFlag, "epics", false, "List only epic items (EPIC:-titled); takes precedence over --epic")
	lsItemsCmd.Flags().StringVar(&lsItemsSinceFlag, "since", "", "Only items created at/after this cutoff: a duration (24h, 7d) or a date (2006-01-02) or RFC3339 timestamp")
	lsItemsCmd.Flags().StringVar(&lsItemsSortFlag, "sort", "", `Sort order: "created" (newest first); default priority`)
	lsItemsCmd.Flags().IntVar(&lsItemsLimitFlag, "limit", 20, "Maximum number of results")
	lsItemsCmd.Flags().BoolVar(&lsItemsJSONFlag, "json", false, "Output as JSON")
}

func runLSItems(out io.Writer, db *sql.DB, projectName, status, priority, component, epic string, epicsOnly bool, since, sort string, limit int, asJSON bool) error {
	// Build project filter
	var projectIDs []int64
	if projectName != "" {
		project, _ := backlog.GetProjectByName(db, projectName)
		if project == nil {
			// No match; print empty results
			if asJSON {
				return printJSON(out, []backlog.Item{})
			}
			return printTable(out, []string{"ID", "PRIORITY", "STATUS", "PROJECT", "COMPONENT", "EPIC", "CREATED", "TITLE"}, [][]string{})
		}
		projectIDs = []int64{project.ID}
	} else {
		// All projects
		projects, err := backlog.ListProjects(db)
		if err != nil {
			return fmt.Errorf("ls backlog: list projects: %w", err)
		}
		for _, p := range projects {
			projectIDs = append(projectIDs, p.ID)
		}
	}

	// Build filter
	filter := backlog.ItemFilter{ProjectIDs: projectIDs, Limit: limit}
	if status != "" {
		filter.Status = &status
	}
	if component != "" {
		filter.Component = &component
	}
	// --epics takes precedence over --epic when both are given — they
	// can't sensibly combine (epics-only vs. one epic's children).
	if epicsOnly {
		filter.EpicsOnly = true
	} else if epic != "" {
		filter.Epic = &epic
	}
	if since != "" {
		cutoff, err := backlog.ParseSinceCutoff(since)
		if err != nil {
			return fmt.Errorf("ls backlog: %w", err)
		}
		filter.CreatedSince = &cutoff
	}
	switch sort {
	case "":
		// default ordering (priority)
	case "created":
		filter.SortByCreated = true
	default:
		return fmt.Errorf("ls backlog: invalid --sort value %q: expected \"created\"", sort)
	}
	if priority != "" {
		// Parse priority: P0 -> 0, P6 -> 6
		if len(priority) == 2 && priority[0] == 'P' {
			if p, ok := backlog.ParsePriority(priority); ok && p >= 0 && p <= 6 {
				filter.PriorityMin = &p
				filter.PriorityMax = &p
			}
		}
	}

	items, err := backlog.ListItems(db, filter)
	if err != nil {
		return fmt.Errorf("ls backlog: list failed: %w", err)
	}

	if asJSON {
		return printJSON(out, items)
	}

	// Build project name map
	projectMap := make(map[int64]string)
	projects, err := backlog.ListProjects(db)
	if err != nil {
		return fmt.Errorf("ls backlog: list projects: %w", err)
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
			item.Epic,
			item.Created,
			item.Title,
		})
	}

	return printTable(out, []string{"ID", "PRIORITY", "STATUS", "PROJECT", "COMPONENT", "EPIC", "CREATED", "TITLE"}, rows)
}

func runLSItemDetail(out io.Writer, db *sql.DB, id string, asJSON bool) error {
	item, err := backlog.GetItem(db, id)
	if err != nil {
		return fmt.Errorf("ls backlog: get item: %w", err)
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
