package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/store"
)

var (
	queryProjectFlag string
	queryTypeFlag    string
	querySearchFlag  string
	queryLimitFlag   int
)

var queryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query knowledge base documents (CLI for hook integration)",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := store.InitDB()
		if err != nil {
			return fmt.Errorf("query: open database: %w", err)
		}
		defer db.Close()
		return runQuery(cmd.OutOrStdout(), db, queryProjectFlag, queryTypeFlag, querySearchFlag, queryLimitFlag)
	},
}

func init() {
	queryCmd.Flags().StringVar(&queryProjectFlag, "project", "", "Project name filter")
	queryCmd.Flags().StringVar(&queryTypeFlag, "type", "", "Document type filter")
	queryCmd.Flags().StringVar(&querySearchFlag, "search", "", "Full-text search query")
	queryCmd.Flags().IntVar(&queryLimitFlag, "limit", 10, "Maximum number of results")
}

func runQuery(out io.Writer, db *sql.DB, project, docType, search string, limit int) error {
	var summaries []store.DocumentSummary
	var err error

	if search != "" {
		summaries, err = store.KeywordSearch(db, search, project, limit)
		if err != nil {
			return fmt.Errorf("query: search failed: %w", err)
		}
	} else {
		summaries, err = store.QueryDocuments(db, docType, project, "", "", nil, limit)
		if err != nil {
			return fmt.Errorf("query: list failed: %w", err)
		}
	}

	data, err := json.Marshal(summaries)
	if err != nil {
		return fmt.Errorf("query: marshal failed: %w", err)
	}

	fmt.Fprintln(out, string(data))
	return nil
}
