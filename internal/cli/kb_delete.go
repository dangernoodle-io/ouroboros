package cli

import (
	"database/sql"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/edges"
	"dangernoodle.io/ouroboros/internal/store"
)

// kbCmd is declared in kb.go, which also attaches kbDeleteCmd as a subcommand.
var kbDeleteCmd = &cobra.Command{
	Use:   "delete <id> [<id>...]",
	Short: "Delete one or more knowledge base documents by ID",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runKBDelete(cmd.OutOrStdout(), db, args)
		})
	},
}

func runKBDelete(out io.Writer, db *sql.DB, idStrs []string) error {
	seen := make(map[int64]bool, len(idStrs))
	ids := make([]int64, 0, len(idStrs))
	for _, s := range idStrs {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil || id <= 0 {
			return fmt.Errorf("kb delete: invalid id %q: must be a positive integer", s)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}

	// All-or-nothing: verify every id exists before deleting any of them, so
	// a typo among a batch aborts with nothing deleted rather than a partial
	// delete. One batched WHERE id IN (...) query instead of one GetDocument
	// call per id.
	fetched, err := store.GetDocuments(db, ids)
	if err != nil {
		return fmt.Errorf("kb delete: %w", err)
	}
	found := make(map[int64]bool, len(fetched))
	for _, doc := range fetched {
		found[doc.ID] = true
	}
	var missing []string
	for _, id := range ids {
		if !found[id] {
			missing = append(missing, strconv.FormatInt(id, 10))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("kb delete: document(s) not found: %s — nothing deleted", strings.Join(missing, ", "))
	}

	// Atomic batch: the doc delete (single DELETE ... WHERE id IN) and the
	// per-id edge cascade cleanup happen in one tx (mirrors
	// backlog.DeleteItems), followed by a single FTS rebuild after commit —
	// not one rebuild per id.
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("kb delete: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := store.DeleteDocumentsTx(tx, ids); err != nil {
		return fmt.Errorf("kb delete: %w", err)
	}
	for _, id := range ids {
		if _, err := edges.CascadeDelete(tx, edges.TypeKB, strconv.FormatInt(id, 10)); err != nil {
			return fmt.Errorf("kb delete: cascade cleanup: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("kb delete: %w", err)
	}

	if err := store.RebuildFTS(db); err != nil {
		return fmt.Errorf("kb delete: %w", err)
	}

	idOut := make([]string, len(ids))
	for i, id := range ids {
		idOut[i] = strconv.FormatInt(id, 10)
	}
	fmt.Fprintf(out, "deleted %d document(s): %s\n", len(ids), strings.Join(idOut, ", "))
	return nil
}
