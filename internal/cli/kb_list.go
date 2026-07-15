package cli

import (
	"database/sql"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/query"
)

var (
	kbListProjectFlag  []string
	kbListTypeFlag     string
	kbListCategoryFlag string
	kbListTagFlag      []string
	kbListLimitFlag    int
	kbListJSONFlag     bool
)

var kbListCmd = &cobra.Command{
	Use:   "list",
	Short: "List/filter knowledge base documents",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runKBList(cmd.OutOrStdout(), db, kbListRequestFlags{
				Projects: kbListProjectFlag,
				Type:     kbListTypeFlag,
				Category: kbListCategoryFlag,
				Tags:     kbListTagFlag,
				Limit:    kbListLimitFlag,
				JSON:     kbListJSONFlag,
			})
		})
	},
}

func init() {
	kbListCmd.Flags().StringArrayVar(&kbListProjectFlag, "project", nil, "Project name filter (repeatable)")
	kbListCmd.Flags().StringVar(&kbListTypeFlag, "type", "", "Document type filter")
	kbListCmd.Flags().StringVar(&kbListCategoryFlag, "category", "", "Category filter")
	kbListCmd.Flags().StringArrayVar(&kbListTagFlag, "tag", nil, "Tag filter (repeatable)")
	kbListCmd.Flags().IntVar(&kbListLimitFlag, "limit", 50, "Maximum number of results")
	kbListCmd.Flags().BoolVar(&kbListJSONFlag, "json", false, "Output as JSON")
}

// kbListRequestFlags is `kb list`'s raw flag capture, translated into an
// internal/query.Request by runKBList.
type kbListRequestFlags struct {
	Projects []string
	Type     string
	Category string
	Tags     []string
	Limit    int
	JSON     bool
}

// runKBList builds a query.Request (domain=kb, filter/list mode — no IDs,
// no Query) and dispatches to query.Get, the same core the MCP query tool
// and `ouroboros query --domain kb` consume.
func runKBList(out io.Writer, db *sql.DB, f kbListRequestFlags) error {
	var types []string
	if f.Type != "" {
		types = []string{f.Type}
	}
	var categories []string
	if f.Category != "" {
		categories = []string{f.Category}
	}

	result, err := query.Get(db, query.Request{
		Domain:     "kb",
		Projects:   f.Projects,
		Types:      types,
		Categories: categories,
		Tags:       f.Tags,
		Limit:      f.Limit,
	})
	if err != nil {
		return fmt.Errorf("kb list: %w", err)
	}

	if f.JSON {
		return printJSON(out, result.DocSummaries)
	}
	return printKBSummaryTable(out, result.DocSummaries)
}
