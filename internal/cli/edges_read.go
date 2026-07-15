package cli

import (
	"database/sql"
	"fmt"
	"io"

	"dangernoodle.io/ouroboros/internal/edges"
)

// runLSEdges is the edges-listing core: label/type/id validation +
// filtering, shared by `edges list` (edges_group.go).
func runLSEdges(out io.Writer, db *sql.DB, label, typ, id string, asJSON bool) error {
	if (typ == "") != (id == "") {
		return fmt.Errorf("edges list: --type and --id must be given together")
	}

	if label != "" && !edges.ValidLabel(label) {
		return fmt.Errorf("edges list: invalid edge label %q: must be one of blocks, relates, explains", label)
	}

	var list []edges.Edge
	var err error
	if typ != "" {
		list, err = edges.EdgesFor(db, typ, id)
		if err != nil {
			return fmt.Errorf("edges list: %w", err)
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
			return fmt.Errorf("edges list: %w", err)
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
