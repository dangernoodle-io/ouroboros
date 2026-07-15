package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"dangernoodle.io/ouroboros/internal/edges"
	"dangernoodle.io/ouroboros/internal/store"
)

// printKBSummaryTable renders a []store.DocumentSummary as a table (lifted
// from the unlanded OU-324 branch's query_render.go, verbatim) — shared by
// `kb list` and `kb search`.
func printKBSummaryTable(out io.Writer, summaries []store.DocumentSummary) error {
	rows := make([][]string, 0, len(summaries))
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

// writeEdges prints a verbose --ids/`kb get` fetch's edges sidecar, or
// nothing if there are none (lifted from the unlanded OU-324 branch's
// query_render.go, verbatim).
func writeEdges(out io.Writer, edgeList []edges.Edge) {
	if len(edgeList) == 0 {
		return
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Edges:")
	for _, e := range edgeList {
		fmt.Fprintf(out, "  %s %s:%s -> %s:%s\n", e.Label, e.SourceType, e.SourceID, e.TargetType, e.TargetID)
	}
}
