package cli

import (
	"database/sql"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/edges"
)

var linkCmd = &cobra.Command{
	Use:   "link <src> <label> <dst>",
	Short: "Create an edge (blocks|relates|explains) between two entries",
	Long:  "src/dst are type:id refs, e.g. item:BB-9 or kb:123. label is blocks, relates, or explains.",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runLink(cmd.OutOrStdout(), db, args[0], args[1], args[2])
		})
	},
}

var unlinkCmd = &cobra.Command{
	Use:   "unlink <src> <label> <dst>",
	Short: "Remove an edge between two entries",
	Long:  "src/dst are type:id refs, e.g. item:BB-9 or kb:123. label is blocks, relates, or explains.",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runUnlink(cmd.OutOrStdout(), db, args[0], args[1], args[2])
		})
	},
}

// parseEdgeRef splits a "type:id" ref (e.g. "item:BB-9" or "kb:123") into
// its endpoint type and id.
func parseEdgeRef(ref string) (typ, id string, err error) {
	parts := strings.SplitN(ref, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid ref %q: expected type:id (e.g. item:BB-9 or kb:123)", ref)
	}
	typ, id = parts[0], parts[1]
	if !edges.ValidType(typ) {
		return "", "", fmt.Errorf("invalid ref type %q: must be item or kb", typ)
	}
	return typ, id, nil
}

func runLink(out io.Writer, db *sql.DB, srcRef, label, dstRef string) error {
	srcType, srcID, err := parseEdgeRef(srcRef)
	if err != nil {
		return fmt.Errorf("link: %w", err)
	}
	dstType, dstID, err := parseEdgeRef(dstRef)
	if err != nil {
		return fmt.Errorf("link: %w", err)
	}

	edge, err := edges.Link(db, srcType, srcID, label, dstType, dstID, 0)
	if err != nil {
		return fmt.Errorf("link: %w", err)
	}

	fmt.Fprintf(out, "linked %s:%s %s %s:%s\n", edge.SourceType, edge.SourceID, edge.Label, edge.TargetType, edge.TargetID)
	return nil
}

func runUnlink(out io.Writer, db *sql.DB, srcRef, label, dstRef string) error {
	srcType, srcID, err := parseEdgeRef(srcRef)
	if err != nil {
		return fmt.Errorf("unlink: %w", err)
	}
	dstType, dstID, err := parseEdgeRef(dstRef)
	if err != nil {
		return fmt.Errorf("unlink: %w", err)
	}

	affected, err := edges.Unlink(db, srcType, srcID, label, dstType, dstID)
	if err != nil {
		return fmt.Errorf("unlink: %w", err)
	}
	if affected == 0 {
		fmt.Fprintf(out, "no edge %s:%s %s %s:%s\n", srcType, srcID, label, dstType, dstID)
		return nil
	}

	fmt.Fprintf(out, "unlinked %s:%s %s %s:%s\n", srcType, srcID, label, dstType, dstID)
	return nil
}
