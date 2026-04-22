package cli

import (
	"database/sql"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/store"
)

var (
	lsKBProjectFlag  string
	lsKBTypeFlag     string
	lsKBCategoryFlag string
	lsKBTagsFlag     []string
	lsKBSearchFlag   string
	lsKBLimitFlag    int
	lsKBJSONFlag     bool
)

var lsKBCmd = &cobra.Command{
	Use:   "kb [ID]",
	Short: "List knowledge base entries",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := store.InitDB()
		if err != nil {
			return fmt.Errorf("ls kb: open database: %w", err)
		}
		defer db.Close()

		if len(args) == 1 {
			return runLSKBDetail(cmd.OutOrStdout(), db, args[0], lsKBJSONFlag)
		}
		return runLSKB(cmd.OutOrStdout(), db, lsKBProjectFlag, lsKBTypeFlag, lsKBCategoryFlag, lsKBTagsFlag, lsKBSearchFlag, lsKBLimitFlag, lsKBJSONFlag)
	},
}

func init() {
	lsKBCmd.Flags().StringVar(&lsKBProjectFlag, "project", "", "Project name filter")
	lsKBCmd.Flags().StringVar(&lsKBTypeFlag, "type", "", "Document type filter")
	lsKBCmd.Flags().StringVar(&lsKBCategoryFlag, "category", "", "Category filter")
	lsKBCmd.Flags().StringArrayVar(&lsKBTagsFlag, "tag", []string{}, "Tag filter (repeatable)")
	lsKBCmd.Flags().StringVar(&lsKBSearchFlag, "search", "", "Full-text search query")
	lsKBCmd.Flags().IntVar(&lsKBLimitFlag, "limit", 50, "Maximum number of results")
	lsKBCmd.Flags().BoolVar(&lsKBJSONFlag, "json", false, "Output as JSON")
}

func runLSKB(out io.Writer, db *sql.DB, projectName, docType, category string, tags []string, search string, limit int, asJSON bool) error {
	var projects []string
	if projectName != "" {
		projects = []string{projectName}
	}

	var summaries []store.DocumentSummary
	var err error

	if search != "" {
		summaries, err = store.SearchDocuments(db, search, docType, projects, limit)
	} else {
		summaries, err = store.QueryDocuments(db, docType, projects, category, "", tags, limit)
	}
	if err != nil {
		return fmt.Errorf("ls kb: query failed: %w", err)
	}

	if asJSON {
		return printJSON(out, summaries)
	}

	// Build table rows
	var rows [][]string
	for _, doc := range summaries {
		rows = append(rows, []string{
			strconv.FormatInt(doc.ID, 10),
			doc.Type,
			doc.Project,
			doc.Category,
			doc.Title,
			strings.Join(doc.Tags, ","),
		})
	}

	return printTable(out, []string{"ID", "TYPE", "PROJECT", "CATEGORY", "TITLE", "TAGS"}, rows)
}

func runLSKBDetail(out io.Writer, db *sql.DB, idStr string, asJSON bool) error {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return fmt.Errorf("ls kb: invalid id: %w", err)
	}

	doc, err := store.GetDocument(db, id)
	if err != nil {
		fmt.Fprintf(out, "not found: %d\n", id)
		return err
	}

	if asJSON {
		return printJSON(out, doc)
	}

	formatKBDetail(out, doc)
	return nil
}
