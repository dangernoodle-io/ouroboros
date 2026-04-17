package cli

import (
	"database/sql"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/kb"
	"dangernoodle.io/ouroboros/internal/store"
)

var importCmd = &cobra.Command{
	Use:   "import [file]",
	Short: "Import documents from JSON file or stdin",
	Long:  "Import documents from a JSON file or stdin. If no file is specified or file is '-', reads from stdin.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := store.InitDB()
		if err != nil {
			return fmt.Errorf("import: open database: %w", err)
		}
		defer db.Close()

		project, _ := cmd.Flags().GetString("project")
		content, err := readImportSource(cmd.InOrStdin(), args)
		if err != nil {
			return err
		}
		return runImport(cmd.OutOrStdout(), db, project, content)
	},
}

func init() {
	importCmd.Flags().StringP("project", "p", "", "Default project if not specified in documents")
}

func readImportSource(stdin io.Reader, args []string) (string, error) {
	if len(args) == 0 || args[0] == "-" {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("import: read stdin: %w", err)
		}
		return string(data), nil
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		return "", fmt.Errorf("import: read file: %w", err)
	}
	return string(data), nil
}

func runImport(out io.Writer, db *sql.DB, project, content string) error {
	if err := kb.Import(db, project, content); err != nil {
		return fmt.Errorf("import: %w", err)
	}
	fmt.Fprintln(out, "ok")
	return nil
}
