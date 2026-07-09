package cli

import (
	"database/sql"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/edges"
)

var (
	lsEdgesLabelFlag string
	lsEdgesTypeFlag  string
	lsEdgesIDFlag    string
	lsEdgesJSONFlag  bool
)

var lsEdgesCmd = &cobra.Command{
	Use:   "edges",
	Short: "List edges (blocks|relates|explains) between items/kb docs",
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runLSEdges(cmd.OutOrStdout(), db, lsEdgesLabelFlag, lsEdgesTypeFlag, lsEdgesIDFlag, lsEdgesJSONFlag)
		})
	},
}

func init() {
	lsEdgesCmd.Flags().StringVar(&lsEdgesLabelFlag, "label", "", "Filter by label (blocks, relates, explains)")
	lsEdgesCmd.Flags().StringVar(&lsEdgesTypeFlag, "type", "", "Endpoint type: item or kb (requires --id)")
	lsEdgesCmd.Flags().StringVar(&lsEdgesIDFlag, "id", "", "Endpoint id (requires --type)")
	lsEdgesCmd.Flags().BoolVar(&lsEdgesJSONFlag, "json", false, "Output as JSON")
}

func runLSEdges(out io.Writer, db *sql.DB, label, typ, id string, asJSON bool) error {
	if (typ == "") != (id == "") {
		return fmt.Errorf("ls edges: --type and --id must be given together")
	}

	if label != "" && !edges.ValidLabel(label) {
		return fmt.Errorf("ls edges: invalid edge label %q: must be one of blocks, relates, explains", label)
	}

	var list []edges.Edge
	var err error
	if typ != "" {
		list, err = edges.EdgesFor(db, typ, id)
		if err != nil {
			return fmt.Errorf("ls edges: %w", err)
		}
		if label != "" {
			filtered := make([]edges.Edge, 0, len(list))
			for _, e := range list {
				if e.Label == label {
					filtered = append(filtered, e)
				}
			}
			list = filtered
		}
	} else {
		list, err = edges.ListEdges(db, label)
		if err != nil {
			return fmt.Errorf("ls edges: %w", err)
		}
	}

	if asJSON {
		return printJSON(out, list)
	}

	rows := make([][]string, 0, len(list))
	for _, e := range list {
		rows = append(rows, []string{
			fmt.Sprintf("%d", e.ID),
			fmt.Sprintf("%s:%s", e.SourceType, e.SourceID),
			e.Label,
			fmt.Sprintf("%s:%s", e.TargetType, e.TargetID),
		})
	}
	return printTable(out, []string{"ID", "SOURCE", "LABEL", "TARGET"}, rows)
}
