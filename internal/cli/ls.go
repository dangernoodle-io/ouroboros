package cli

import (
	"github.com/spf13/cobra"
)

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "Browse knowledge base, items, plans, and projects",
}

func init() {
	lsCmd.AddCommand(lsItemsCmd)
	lsCmd.AddCommand(lsKBCmd)
	lsCmd.AddCommand(lsPlansCmd)
	lsCmd.AddCommand(lsProjectsCmd)
}
