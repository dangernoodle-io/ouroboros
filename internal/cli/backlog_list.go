package cli

import (
	"database/sql"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/query"
)

var (
	backlogListProjectFlag   []string
	backlogListStatusFlag    []string
	backlogListPriorityFlag  string
	backlogListComponentFlag string
	backlogListEpicFlag      string
	backlogListEpicsFlag     bool
	backlogListSinceFlag     string
	backlogListSortFlag      string
	backlogListLimitFlag     int
	backlogListJSONFlag      bool
)

var backlogListCmd = &cobra.Command{
	Use:   "list",
	Short: "List/filter backlog items",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runBacklogList(cmd.OutOrStdout(), db, backlogListRequestFlags{
				Projects:  backlogListProjectFlag,
				Statuses:  backlogListStatusFlag,
				Priority:  backlogListPriorityFlag,
				Component: backlogListComponentFlag,
				Epic:      backlogListEpicFlag,
				EpicsOnly: backlogListEpicsFlag,
				Since:     backlogListSinceFlag,
				Sort:      backlogListSortFlag,
				Limit:     backlogListLimitFlag,
				JSON:      backlogListJSONFlag,
			})
		})
	},
}

func init() {
	backlogListCmd.Flags().StringArrayVar(&backlogListProjectFlag, "project", nil, "Project name filter (repeatable)")
	backlogListCmd.Flags().StringArrayVar(&backlogListStatusFlag, "status", nil, "Status filter (open or done; repeatable)")
	backlogListCmd.Flags().StringVar(&backlogListPriorityFlag, "priority", "", "Priority filter (P0-P6)")
	backlogListCmd.Flags().StringVar(&backlogListComponentFlag, "component", "", "Component filter")
	backlogListCmd.Flags().StringVar(&backlogListEpicFlag, "epic", "", "Epic item id filter (that epic's children)")
	backlogListCmd.Flags().BoolVar(&backlogListEpicsFlag, "epics", false, "List only epic items (EPIC:-titled); takes precedence over --epic")
	backlogListCmd.Flags().StringVar(&backlogListSinceFlag, "since", "", "Only items created at/after this cutoff: a duration (24h, 7d) or a date (2006-01-02) or RFC3339 timestamp")
	backlogListCmd.Flags().StringVar(&backlogListSortFlag, "sort", "", `Sort order: "created" (newest first); default priority`)
	backlogListCmd.Flags().IntVar(&backlogListLimitFlag, "limit", 20, "Maximum number of results")
	backlogListCmd.Flags().BoolVar(&backlogListJSONFlag, "json", false, "Output as JSON")
}

// backlogListRequestFlags is `backlog list`'s raw flag capture, translated
// into an internal/query.Request by runBacklogList.
type backlogListRequestFlags struct {
	Projects  []string
	Statuses  []string
	Priority  string
	Component string
	Epic      string
	EpicsOnly bool
	Since     string
	Sort      string
	Limit     int
	JSON      bool
}

// runBacklogList builds a query.Request (domain=backlog, filter/list mode —
// no IDs, no Query) and dispatches to query.Get, the same core the MCP query
// tool and `ouroboros query --domain backlog` consume. Multiple --status
// values (OU-102's multi-status, folded in here since query.Request.Status
// is single-valued) are resolved via mergeItemsByStatus rather than
// query.Get directly.
func runBacklogList(out io.Writer, db *sql.DB, f backlogListRequestFlags) error {
	priorityMin, priorityMax := priorityMinMax(f.Priority)

	base := query.Request{
		Domain:      "backlog",
		Projects:    f.Projects,
		PriorityMin: priorityMin,
		PriorityMax: priorityMax,
		Component:   f.Component,
		Epic:        f.Epic,
		EpicsOnly:   f.EpicsOnly,
		Since:       f.Since,
		Sort:        f.Sort,
		Limit:       f.Limit,
	}

	statuses := dedupStrings(f.Statuses)

	var items []backlog.Item
	switch {
	case len(statuses) > 1:
		merged, err := mergeItemsByStatus(db, base, statuses)
		if err != nil {
			return fmt.Errorf("backlog list: %w", err)
		}
		items = merged
	default:
		if len(statuses) == 1 {
			base.Status = statuses[0]
		}
		result, err := query.Get(db, base)
		if err != nil {
			return fmt.Errorf("backlog list: %w", err)
		}
		items = result.Items
	}

	if f.JSON {
		return printJSON(out, items)
	}
	return printBacklogItemTable(out, db, items)
}
