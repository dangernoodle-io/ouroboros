package cli

import (
	"database/sql"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/store"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage project configuration (get/set/list)",
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a config value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := store.InitDB()
		if err != nil {
			return fmt.Errorf("config get: open database: %w", err)
		}
		defer db.Close()
		return runConfigGet(cmd.OutOrStdout(), db, args[0])
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := store.InitDB()
		if err != nil {
			return fmt.Errorf("config set: open database: %w", err)
		}
		defer db.Close()
		return runConfigSet(cmd.OutOrStdout(), db, args[0], args[1])
	},
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all config values",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := store.InitDB()
		if err != nil {
			return fmt.Errorf("config list: open database: %w", err)
		}
		defer db.Close()
		return runConfigList(cmd.OutOrStdout(), db)
	},
}

func init() {
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configListCmd)
}

func runConfigGet(out io.Writer, db *sql.DB, key string) error {
	value, err := backlog.GetConfig(db, key)
	if err != nil {
		return fmt.Errorf("config get: %w", err)
	}
	fmt.Fprintln(out, value)
	return nil
}

func runConfigSet(out io.Writer, db *sql.DB, key, value string) error {
	if err := backlog.SetConfig(db, key, value); err != nil {
		return fmt.Errorf("config set: %w", err)
	}
	fmt.Fprintf(out, "set %s=%s\n", key, value)
	return nil
}

func runConfigList(out io.Writer, db *sql.DB) error {
	cfg, err := backlog.GetAllConfig(db)
	if err != nil {
		return fmt.Errorf("config list: %w", err)
	}
	for k, v := range cfg {
		fmt.Fprintf(out, "%s=%s\n", k, v)
	}
	return nil
}
