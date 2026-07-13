package cli

import (
	"fmt"

	mcpkitcli "github.com/dangernoodle-io/mcpkit/cli"
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

	// serverCmd is the mcpkit-composed MCP server ("ouroboros server"). A
	// bare invocation (no subcommand, no --version) also runs it — the
	// Claude Code plugin launches the bare binary — so
	// mcpkitcli.UseAsDefault wires serverCmd.RunE onto rootCmd, then the
	// wrapper below layers the pre-existing --version short-circuit back on
	// top (UseAsDefault would otherwise overwrite it wholesale).
	serverCmd := app.NewServerCommand(Version)
	rootCmd.AddCommand(serverCmd)
	mcpkitcli.UseAsDefault(rootCmd, serverCmd)

	serveRunE := rootCmd.RunE
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if versionFlag {
			if Version != "" {
				fmt.Println(Version)
			} else {
				fmt.Println("(development build)")
			}
			return nil
		}
		// No subcommand and no --version: run the MCP server (default action).
		return serveRunE(cmd, args)
	}

	rootCmd.AddCommand(queryCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(lsCmd)
	rootCmd.AddCommand(projectCmd)
	rootCmd.AddCommand(planCmd)
	rootCmd.AddCommand(kbCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(roadmapCmd)
	rootCmd.AddCommand(linkCmd)
	rootCmd.AddCommand(unlinkCmd)
	rootCmd.AddCommand(dashboardCmd)
	mustMountProviders(rootCmd, claudeProvider())
}

// mustMountProviders mounts providers onto root via mcpkit's MountProviders,
// panicking on a non-nil error (an unresolved Mount.Under path) — a mount
// failure is a startup-config bug, not a runtime condition to recover from.
// Extracted from init() so the panic path is independently testable.
func mustMountProviders(root *cobra.Command, providers ...mcpkitcli.CommandProvider) {
	if err := mcpkitcli.MountProviders(root, providers...); err != nil {
		panic(fmt.Sprintf("ouroboros: mount claude provider: %v", err))
	}
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
