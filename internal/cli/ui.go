package cli

import (
	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/store"
	"dangernoodle.io/ouroboros/internal/tui"
)

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Browse KB, backlog, and plans in a terminal UI",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := store.InitDB()
		if err != nil {
			return err
		}
		defer db.Close()
		return tui.Run(db)
	},
}
