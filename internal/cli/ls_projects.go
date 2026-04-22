package cli

import (
	"database/sql"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/store"
)

var lsProjectJSONFlag bool

var lsProjectsCmd = &cobra.Command{
	Use:   "projects",
	Short: "List projects",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := store.InitDB()
		if err != nil {
			return fmt.Errorf("ls projects: open database: %w", err)
		}
		defer db.Close()

		return runLSProjects(cmd.OutOrStdout(), db, lsProjectJSONFlag)
	},
}

func init() {
	lsProjectsCmd.Flags().BoolVar(&lsProjectJSONFlag, "json", false, "Output as JSON")
}

func runLSProjects(out io.Writer, db *sql.DB, asJSON bool) error {
	projects, err := backlog.ListProjects(db)
	if err != nil {
		return fmt.Errorf("ls projects: list failed: %w", err)
	}

	if asJSON {
		return printJSON(out, projects)
	}

	// Build table rows
	var rows [][]string
	for _, p := range projects {
		rows = append(rows, []string{p.Name, p.Prefix, p.Created})
	}

	return printTable(out, []string{"NAME", "PREFIX", "CREATED"}, rows)
}
