package cli

import (
	"database/sql"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/query"
)

var (
	backlogGetVerboseFlag bool
	backlogGetJSONFlag    bool
)

var backlogGetCmd = &cobra.Command{
	Use:   "get <id> [<id>...]",
	Short: "Fetch backlog items by id",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runBacklogGet(cmd.OutOrStdout(), db, args, backlogGetVerboseFlag, backlogGetJSONFlag)
		})
	},
}

func init() {
	backlogGetCmd.Flags().BoolVar(&backlogGetVerboseFlag, "verbose", false, "Include notes + edges sidecar")
	backlogGetCmd.Flags().BoolVar(&backlogGetJSONFlag, "json", false, "Output as JSON")
}

// runBacklogGet dispatches an id-fetch via query.Get (domain=backlog, IDs
// mode), the same core the MCP query tool and `ouroboros query --domain
// backlog --ids ...` consume. Unlike domain=kb (store.GetDocuments silently
// omits a miss), domain=backlog's underlying backlog.GetItems is
// all-or-nothing: a single missing id fails the WHOLE batched query.Get
// call, so runBacklogGet pre-screens each requested id via backlog.GetItem
// (alias-aware resolution) to split found/missing BEFORE dispatching
// query.Get with only the ids known to resolve — that keeps query.Get's
// call always miss-free while still routing the actual fetch through the
// shared core.
//
// Found items are rendered normally to out (JSON array or table — data
// only, nothing appended); any missing id instead produces a returned "not
// found" error rather than writing to out — out must stay a clean,
// parseable stream for JSON consumers; the diagnostic surfaces via the
// returned error (cobra prints RunE errors to stderr; SilenceErrors is not
// set on the root command).
//
// query.Get's IDs-mode fetch always zeroes item.ProjectID (see
// internal/query/get.go's getBacklogItems), so table-mode rendering
// resolves each found item's real project name via a direct
// backlog.GetItem + backlog.GetProjectByID lookup.
func runBacklogGet(out io.Writer, db *sql.DB, idStrs []string, verbose, asJSON bool) error {
	var validIDs []string
	var missing []string
	for _, s := range idStrs {
		id := strings.TrimSpace(s)
		if _, err := backlog.GetItem(db, id); err != nil {
			missing = append(missing, id)
			continue
		}
		validIDs = append(validIDs, id)
	}

	result := query.Result{ItemsJSON: []query.ItemResult{}}
	if len(validIDs) > 0 {
		ids := make([]any, 0, len(validIDs))
		for _, id := range validIDs {
			ids = append(ids, id)
		}
		var err error
		result, err = query.Get(db, query.Request{Domain: "backlog", IDs: ids, Verbose: verbose})
		if err != nil {
			return fmt.Errorf("backlog get: %w", err)
		}
	}

	if asJSON {
		if err := printJSON(out, result.ItemsJSON); err != nil {
			return err
		}
	} else {
		for i, it := range result.ItemsJSON {
			if i > 0 {
				fmt.Fprintln(out)
			}
			formatItemDetail(out, it.Item, resolveProjectName(db, it.ID))
			writeEdges(out, it.Edges)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("backlog get: not found: %s", strings.Join(missing, ", "))
	}
	return nil
}

// resolveProjectName looks up id's real project name, following the same
// resolution path the retired `ls backlog` detail view used: backlog.GetItem (unlike
// query.Get's IDs-mode result, this still carries the real ProjectID) then
// backlog.GetProjectByID. Errors are swallowed (empty project name) rather
// than propagated — a lookup failure here would only degrade the detail
// view's project column, not the fetch itself, and the item is already
// known to exist (it came from result.ItemsJSON).
func resolveProjectName(db *sql.DB, id string) string {
	item, err := backlog.GetItem(db, id)
	if err != nil || item.ProjectID <= 0 {
		return ""
	}
	project, err := backlog.GetProjectByID(db, item.ProjectID)
	if err != nil || project == nil {
		return ""
	}
	return project.Name
}
