package cli

import (
	"github.com/spf13/cobra"
)

// kbCmd is a pure noun group (OU-334): bare `kb` shows help. Verb
// subcommands carry all behavior — write (kb_write.go, the former bare-`kb`
// write action, moved verbatim), list/get/search (kb_list.go/kb_get.go/
// kb_search.go, the internal/query-backed reads), and delete (kb_delete.go,
// unchanged).
var kbCmd = &cobra.Command{
	Use:   "kb",
	Short: "Manage knowledge base documents",
}

func init() {
	kbCmd.AddCommand(kbWriteCmd)
	kbCmd.AddCommand(kbListCmd)
	kbCmd.AddCommand(kbGetCmd)
	kbCmd.AddCommand(kbSearchCmd)
	kbCmd.AddCommand(kbDeleteCmd)
}
