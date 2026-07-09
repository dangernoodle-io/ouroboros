package cli

import (
	"database/sql"
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/edges"
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

	// Atomic: the doc delete and its edge cascade cleanup happen in one tx
	// (mirrors backlog.DeleteItems) — a crash between them must not orphan
	// edges referencing the now-deleted document.
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("kb delete: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := store.DeleteDocumentTx(tx, id); err != nil {
		return fmt.Errorf("kb delete: %w", err)
	}
	if _, err := edges.CascadeDelete(tx, edges.TypeKB, idStr); err != nil {
		return fmt.Errorf("kb delete: cascade cleanup: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("kb delete: %w", err)
	}

	if err := store.RebuildFTS(db); err != nil {
		return fmt.Errorf("kb delete: %w", err)
	}

	fmt.Fprintf(out, "deleted document %d\n", id)
	return nil
}
