package cli

import (
	"github.com/spf13/cobra"
)

// backlogCmd is a pure noun group (OU-335): bare `backlog` shows help.
// Verb subcommands carry all behavior — list/get/search (backlog_list.go/
// backlog_get.go/backlog_search.go, the internal/query-backed reads).
// `ls backlog` (ls_backlog.go) stays as-is for now (removed in a later PR);
// this noun group coexists with it.
var backlogCmd = &cobra.Command{
	Use:   "backlog",
	Short: "Read backlog items",
}

func init() {
	backlogCmd.AddCommand(backlogListCmd)
	backlogCmd.AddCommand(backlogGetCmd)
	backlogCmd.AddCommand(backlogSearchCmd)
}
