package cli

import (
	"database/sql"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/query"
	"dangernoodle.io/ouroboros/internal/store"
)

var (
	kbGetVerboseFlag bool
	kbGetJSONFlag    bool
)

var kbGetCmd = &cobra.Command{
	Use:   "get <id> [<id>...]",
	Short: "Fetch knowledge base documents by id",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runKBGet(cmd.OutOrStdout(), db, args, kbGetVerboseFlag, kbGetJSONFlag)
		})
	},
}

func init() {
	kbGetCmd.Flags().BoolVar(&kbGetVerboseFlag, "verbose", false, "Include notes + edges sidecar")
	kbGetCmd.Flags().BoolVar(&kbGetJSONFlag, "json", false, "Output as JSON")
}

// runKBGet dispatches an id-fetch via query.Get (domain=kb, IDs mode), the
// same core the MCP query tool and `ouroboros query --domain kb --ids ...`
// consume. query.Get's contract silently omits an id with no matching row
// (mirrors store.GetDocuments); runKBGet restores fail-on-miss (matching the
// old `ls kb <id>` behavior) by diffing the requested ids against the
// returned docs after fetching: docs that WERE found are still rendered
// normally to out (JSON array or table — data only, nothing appended), but
// any missing id produces a returned "not found" error rather than writing
// to out — out must stay a clean, parseable stream for JSON consumers; the
// diagnostic surfaces via the returned error (cobra prints RunE errors to
// stderr; SilenceErrors is not set on the root command). Only a non-numeric
// id is rejected before dispatch.
func runKBGet(out io.Writer, db *sql.DB, idStrs []string, verbose, asJSON bool) error {
	ids := make([]any, 0, len(idStrs))
	normalized := make([]string, 0, len(idStrs))
	for _, s := range idStrs {
		n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return fmt.Errorf("kb get: invalid id %q: must be an integer", s)
		}
		ids = append(ids, n)
		normalized = append(normalized, strconv.FormatInt(int64(n), 10))
	}

	result, err := query.Get(db, query.Request{Domain: "kb", IDs: ids, Verbose: verbose})
	if err != nil {
		return fmt.Errorf("kb get: %w", err)
	}

	found := make(map[string]bool, len(result.Docs))
	for _, d := range result.Docs {
		switch v := d.(type) {
		case *store.Document:
			found[strconv.FormatInt(v.ID, 10)] = true
		case query.DocWithEdges:
			found[strconv.FormatInt(v.ID, 10)] = true
		default:
			return fmt.Errorf("kb get: unexpected doc type %T", d)
		}
	}

	if asJSON {
		if err := printJSON(out, result.Docs); err != nil {
			return err
		}
	} else {
		for i, d := range result.Docs {
			if i > 0 {
				fmt.Fprintln(out)
			}
			switch v := d.(type) {
			case *store.Document:
				formatKBDetail(out, v)
			case query.DocWithEdges:
				formatKBDetail(out, v.Document)
				writeEdges(out, v.Edges)
			}
		}
	}

	var missing []string
	for i, id := range normalized {
		if !found[id] {
			missing = append(missing, strings.TrimSpace(idStrs[i]))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("kb get: not found: %s", strings.Join(missing, ", "))
	}
	return nil
}
