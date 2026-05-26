package cli

import (
	"database/sql"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/kb"
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export KB to markdown",
	Long:  "Export the knowledge base to markdown. Optionally filter by project and/or type.",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		docType, _ := cmd.Flags().GetString("type")
		return withDB(func(db *sql.DB) error {
			return runExport(cmd.OutOrStdout(), db, project, docType)
		})
	},
}

func init() {
	exportCmd.Flags().StringP("project", "p", "", "Filter by project name")
	exportCmd.Flags().StringP("type", "t", "", "Filter by document type (decision, fact, note, relation)")
}

func runExport(out io.Writer, db *sql.DB, project, docType string) error {
	var projects []string
	if project != "" {
		projects = []string{project}
	}

	markdown, err := kb.ExportMarkdown(db, projects, docType)
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}

	_, err = fmt.Fprint(out, markdown)
	return err
}
