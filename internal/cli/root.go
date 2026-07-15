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

func init() {
	// rootCmd carries no Run/RunE: bare `ouroboros` (no subcommand) is a
	// pure help dispatcher (cobra's default for a childless invocation),
	// not a default action. The MCP server only runs via the explicit
	// "server" subcommand (app.NewServerCommand, mcpkit's cli.ServerCmd) —
	// the Claude Code plugin invokes `ouroboros server` explicitly
	// (plugin/.claude-plugin/plugin.json).
	rootCmd.AddCommand(app.NewServerCommand(Version))

	// rootCmd.Version wires cobra's own --version/-v flag (idiomatic:
	// InitDefaultVersionFlag adds it automatically whenever Version is
	// non-empty). A dev build (no ldflags -X) still gets a working
	// --version rather than silently losing the flag.
	if Version != "" {
		rootCmd.Version = Version
	} else {
		rootCmd.Version = "(development build)"
	}

	rootCmd.AddCommand(queryCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(lsCmd)
	rootCmd.AddCommand(projectCmd)
	rootCmd.AddCommand(planCmd)
	rootCmd.AddCommand(kbCmd)
	rootCmd.AddCommand(backlogCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(roadmapCmd)
	rootCmd.AddCommand(linkCmd)
	rootCmd.AddCommand(unlinkCmd)
	rootCmd.AddCommand(edgesCmd)
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
