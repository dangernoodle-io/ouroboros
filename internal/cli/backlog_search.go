package cli

import (
	"database/sql"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/query"
)

var (
	backlogSearchProjectFlag   []string
	backlogSearchStatusFlag    string
	backlogSearchPriorityFlag  string
	backlogSearchComponentFlag string
	backlogSearchEpicFlag      string
	backlogSearchEpicsFlag     bool
	backlogSearchSinceFlag     string
	backlogSearchSortFlag      string
	backlogSearchLimitFlag     int
	backlogSearchJSONFlag      bool
)

var backlogSearchCmd = &cobra.Command{
	Use:   "search <text>",
	Short: "Full-text search backlog items",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runBacklogSearch(cmd.OutOrStdout(), db, args[0], backlogSearchRequestFlags{
				Projects:  backlogSearchProjectFlag,
				Status:    backlogSearchStatusFlag,
				Priority:  backlogSearchPriorityFlag,
				Component: backlogSearchComponentFlag,
				Epic:      backlogSearchEpicFlag,
				EpicsOnly: backlogSearchEpicsFlag,
				Since:     backlogSearchSinceFlag,
				Sort:      backlogSearchSortFlag,
				Limit:     backlogSearchLimitFlag,
				JSON:      backlogSearchJSONFlag,
			})
		})
	},
}

func init() {
	backlogSearchCmd.Flags().StringArrayVar(&backlogSearchProjectFlag, "project", nil, "Project name filter (repeatable)")
	backlogSearchCmd.Flags().StringVar(&backlogSearchStatusFlag, "status", "", "Status filter (open or done)")
	backlogSearchCmd.Flags().StringVar(&backlogSearchPriorityFlag, "priority", "", "Priority filter (P0-P6)")
	backlogSearchCmd.Flags().StringVar(&backlogSearchComponentFlag, "component", "", "Component filter")
	backlogSearchCmd.Flags().StringVar(&backlogSearchEpicFlag, "epic", "", "Epic item id filter (that epic's children)")
	backlogSearchCmd.Flags().BoolVar(&backlogSearchEpicsFlag, "epics", false, "Only epic items (EPIC:-titled); takes precedence over --epic")
	backlogSearchCmd.Flags().StringVar(&backlogSearchSinceFlag, "since", "", "Only items created at/after this cutoff: a duration (24h, 7d) or a date (2006-01-02) or RFC3339 timestamp")
	backlogSearchCmd.Flags().StringVar(&backlogSearchSortFlag, "sort", "", `Sort order: "created" (newest first); default relevance`)
	backlogSearchCmd.Flags().IntVar(&backlogSearchLimitFlag, "limit", 20, "Maximum number of results")
	backlogSearchCmd.Flags().BoolVar(&backlogSearchJSONFlag, "json", false, "Output as JSON")
}

// backlogSearchRequestFlags is `backlog search`'s raw flag capture,
// translated into an internal/query.Request by runBacklogSearch. Unlike
// `backlog list`, --status is single-valued here (query.Request.Status is
// single-valued and OU-102's multi-status merge is list-only per the
// backlog-noun-group build spec).
type backlogSearchRequestFlags struct {
	Projects  []string
	Status    string
	Priority  string
	Component string
	Epic      string
	EpicsOnly bool
	Since     string
	Sort      string
	Limit     int
	JSON      bool
}

// runBacklogSearch dispatches a full-text search via query.Search
// (domain=backlog), the same core the MCP query tool and `ouroboros query
// --domain backlog --search ...` consume; filters route through the same
// internal/query buildItemFilter as `backlog list`.
func runBacklogSearch(out io.Writer, db *sql.DB, text string, f backlogSearchRequestFlags) error {
	if text == "" {
		return fmt.Errorf("backlog search: text is required")
	}

	priorityMin, priorityMax := priorityMinMax(f.Priority)

	result, err := query.Search(db, query.Request{
		Domain:      "backlog",
		Query:       text,
		Projects:    f.Projects,
		PriorityMin: priorityMin,
		PriorityMax: priorityMax,
		Status:      f.Status,
		Component:   f.Component,
		Epic:        f.Epic,
		EpicsOnly:   f.EpicsOnly,
		Since:       f.Since,
		Sort:        f.Sort,
		Limit:       f.Limit,
	})
	if err != nil {
		return fmt.Errorf("backlog search: %w", err)
	}

	if f.JSON {
		return printJSON(out, result.Items)
	}
	return printBacklogItemTable(out, db, result.Items)
}
