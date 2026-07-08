package cli

import (
	"database/sql"
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/store"
)

// kbCmd is declared in kb.go, which also attaches kbDeleteCmd as a subcommand.
var kbDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a knowledge base document by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runKBDelete(cmd.OutOrStdout(), db, args[0])
		})
	},
}

func runKBDelete(out io.Writer, db *sql.DB, idStr string) error {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return fmt.Errorf("kb delete: invalid id %q: must be an integer", idStr)
	}

	doc, err := store.GetDocument(db, id)
	if err != nil {
		return fmt.Errorf("kb delete: %w", err)
	}
	if doc == nil {
		return fmt.Errorf("kb delete: document %d not found", id)
	}

	if err := store.DeleteDocument(db, id); err != nil {
		return fmt.Errorf("kb delete: %w", err)
	}

	fmt.Fprintf(out, "deleted document %d\n", id)
	return nil
}
