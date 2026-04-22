package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/app"
)

// Version is set via ldflags at build time.
var Version string

var rootCmd = &cobra.Command{
	Use:          "ouroboros",
	Short:        "MCP server for project knowledge base and backlog management",
	SilenceUsage: true,
}

var versionFlag bool

func init() {
	rootCmd.Flags().BoolVarP(&versionFlag, "version", "v", false, "Print version and exit")

	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if versionFlag {
			if Version != "" {
				fmt.Println(Version)
			} else {
				fmt.Println("(development build)")
			}
			return nil
		}
		// No subcommand and no --version: run the MCP server (default action)
		return app.Serve(Version)
	}

	rootCmd.AddCommand(queryCmd)
	rootCmd.AddCommand(itemsCmd)
	rootCmd.AddCommand(putCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(statuslineCmd)
	rootCmd.AddCommand(lsCmd)
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
