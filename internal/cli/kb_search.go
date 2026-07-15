package cli

import (
	"database/sql"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/query"
)

var (
	kbSearchProjectFlag  string
	kbSearchTypeFlag     string
	kbSearchCategoryFlag string
	kbSearchLimitFlag    int
	kbSearchJSONFlag     bool
)

var kbSearchCmd = &cobra.Command{
	Use:   "search <text>",
	Short: "Full-text search knowledge base documents",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runKBSearch(cmd.OutOrStdout(), db, args[0], kbSearchProjectFlag, kbSearchTypeFlag, kbSearchCategoryFlag, kbSearchLimitFlag, kbSearchJSONFlag)
		})
	},
}

func init() {
	kbSearchCmd.Flags().StringVar(&kbSearchProjectFlag, "project", "", "Project name filter")
	kbSearchCmd.Flags().StringVar(&kbSearchTypeFlag, "type", "", "Document type filter")
	kbSearchCmd.Flags().StringVar(&kbSearchCategoryFlag, "category", "", "Category filter")
	kbSearchCmd.Flags().IntVar(&kbSearchLimitFlag, "limit", 50, "Maximum number of results")
	kbSearchCmd.Flags().BoolVar(&kbSearchJSONFlag, "json", false, "Output as JSON")
}

// runKBSearch dispatches a full-text search via query.Search (domain=kb,
// single-query mode), the same core the MCP query tool and `ouroboros
// query --domain kb --search ...` consume.
func runKBSearch(out io.Writer, db *sql.DB, text, project, docType, category string, limit int, asJSON bool) error {
	if text == "" {
		return fmt.Errorf("kb search: text is required")
	}

	var projects []string
	if project != "" {
		projects = []string{project}
	}
	var types []string
	if docType != "" {
		types = []string{docType}
	}
	var categories []string
	if category != "" {
		categories = []string{category}
	}

	result, err := query.Search(db, query.Request{
		Domain:     "kb",
		Query:      text,
		Projects:   projects,
		Types:      types,
		Categories: categories,
		Limit:      limit,
	})
	if err != nil {
		return fmt.Errorf("kb search: %w", err)
	}

	if asJSON {
		return printJSON(out, result.DocSummaries)
	}
	return printKBSummaryTable(out, result.DocSummaries)
}
