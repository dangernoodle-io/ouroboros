package cli

import (
	"database/sql"

	"github.com/spf13/cobra"
)

// edgesCmd is the cobra-idiomatic noun group for edge operations
// (list/link/unlink). It coexists with the top-level link/unlink
// commands (kept as back-compat aliases) and with `ls edges` (kept
// until dissolution) — all three share the same run* helpers so
// behavior never diverges between surfaces.
var edgesCmd = &cobra.Command{
	Use:   "edges",
	Short: "Manage edges (blocks|relates|explains) between items/kb docs",
}

var (
	edgesListLabelFlag string
	edgesListTypeFlag  string
	edgesListIDFlag    string
	edgesListJSONFlag  bool
)

var edgesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List edges (blocks|relates|explains) between items/kb docs",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runLSEdges(cmd.OutOrStdout(), db, edgesListLabelFlag, edgesListTypeFlag, edgesListIDFlag, edgesListJSONFlag)
		})
	},
}

var edgesLinkCmd = &cobra.Command{
	Use:   "link <src> <label> <dst>",
	Short: linkCmd.Short,
	Long:  linkCmd.Long,
	Args:  cobra.ExactArgs(3),
	RunE:  linkCmd.RunE,
}

var edgesUnlinkCmd = &cobra.Command{
	Use:   "unlink <src> <label> <dst>",
	Short: unlinkCmd.Short,
	Long:  unlinkCmd.Long,
	Args:  cobra.ExactArgs(3),
	RunE:  unlinkCmd.RunE,
}

func init() {
	edgesListCmd.Flags().StringVar(&edgesListLabelFlag, "label", "", "Filter by label (blocks, relates, explains)")
	edgesListCmd.Flags().StringVar(&edgesListTypeFlag, "type", "", "Endpoint type: item or kb (requires --id)")
	edgesListCmd.Flags().StringVar(&edgesListIDFlag, "id", "", "Endpoint id (requires --type)")
	edgesListCmd.Flags().BoolVar(&edgesListJSONFlag, "json", false, "Output as JSON")

	edgesCmd.AddCommand(edgesListCmd)
	edgesCmd.AddCommand(edgesLinkCmd)
	edgesCmd.AddCommand(edgesUnlinkCmd)
}
