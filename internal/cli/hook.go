package cli

import "github.com/spf13/cobra"

// hookCmd is the parent for Claude Code plugin hook integrations; subcommands
// are added via their own init() (see hook_stop.go).
var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Claude Code plugin hook integrations",
}
