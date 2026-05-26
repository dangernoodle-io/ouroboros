package cli

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/store"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage projects",
}

func runProjectCreate(w io.Writer, db *sql.DB, name string) error {
	prefix, err := backlog.DerivePrefix(db, name)
	if err != nil {
		return fmt.Errorf("project create: %w", err)
	}
	proj, err := backlog.CreateProject(db, name, prefix)
	if err != nil {
		return fmt.Errorf("project create: %w", err)
	}
	data, err := json.Marshal(proj)
	if err != nil {
		return fmt.Errorf("project create: marshal: %w", err)
	}
	fmt.Fprintln(w, string(data))
	return nil
}

func runProjectGet(w io.Writer, db *sql.DB, name string) error {
	proj, err := backlog.GetProjectByName(db, name)
	if err != nil {
		return fmt.Errorf("project get: %w", err)
	}
	data, err := json.Marshal(proj)
	if err != nil {
		return fmt.Errorf("project get: marshal: %w", err)
	}
	fmt.Fprintln(w, string(data))
	return nil
}

func runProjectList(w io.Writer, db *sql.DB) error {
	projects, err := backlog.ListProjects(db)
	if err != nil {
		return fmt.Errorf("project list: %w", err)
	}
	if projects == nil {
		projects = []backlog.Project{}
	}
	data, err := json.Marshal(projects)
	if err != nil {
		return fmt.Errorf("project list: marshal: %w", err)
	}
	fmt.Fprintln(w, string(data))
	return nil
}

func runProjectRename(w io.Writer, db *sql.DB, name, newName, newPrefix string) error {
	proj, err := backlog.RenameProject(db, name, newName, newPrefix)
	if err != nil {
		return fmt.Errorf("project rename: %w", err)
	}
	data, err := json.Marshal(proj)
	if err != nil {
		return fmt.Errorf("project rename: marshal: %w", err)
	}
	fmt.Fprintln(w, string(data))
	return nil
}

func runProjectDelete(w io.Writer, db *sql.DB, name string, force bool, reassignTo string) error {
	if force && reassignTo != "" {
		return fmt.Errorf("project delete: --force and --reassign-to are mutually exclusive")
	}
	if err := backlog.DeleteProject(db, name, force, reassignTo); err != nil {
		return fmt.Errorf("project delete: %w", err)
	}
	fmt.Fprintf(w, "deleted project %q\n", name)
	return nil
}

var projectCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := store.InitDB()
		if err != nil {
			return fmt.Errorf("project create: open database: %w", err)
		}
		defer db.Close() //nolint:errcheck
		return runProjectCreate(cmd.OutOrStdout(), db, args[0])
	},
}

var projectGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get a project by name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := store.InitDB()
		if err != nil {
			return fmt.Errorf("project get: open database: %w", err)
		}
		defer db.Close() //nolint:errcheck
		return runProjectGet(cmd.OutOrStdout(), db, args[0])
	},
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all projects",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := store.InitDB()
		if err != nil {
			return fmt.Errorf("project list: open database: %w", err)
		}
		defer db.Close() //nolint:errcheck
		return runProjectList(cmd.OutOrStdout(), db)
	},
}

var (
	renameNewName   string
	renameNewPrefix string
)

var projectRenameCmd = &cobra.Command{
	Use:   "rename <name>",
	Short: "Rename a project (name and/or prefix)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if renameNewName == "" && renameNewPrefix == "" {
			return errors.New("at least one of --new-name or --new-prefix is required")
		}
		db, err := store.InitDB()
		if err != nil {
			return fmt.Errorf("project rename: open database: %w", err)
		}
		defer db.Close() //nolint:errcheck
		return runProjectRename(cmd.OutOrStdout(), db, args[0], renameNewName, renameNewPrefix)
	},
}

var (
	projectDeleteForce      bool
	projectDeleteReassignTo string
)

var projectDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := store.InitDB()
		if err != nil {
			return fmt.Errorf("project delete: open database: %w", err)
		}
		defer db.Close() //nolint:errcheck
		return runProjectDelete(cmd.OutOrStdout(), db, args[0], projectDeleteForce, projectDeleteReassignTo)
	},
}

func init() {
	projectRenameCmd.Flags().StringVar(&renameNewName, "new-name", "", "New project name")
	projectRenameCmd.Flags().StringVar(&renameNewPrefix, "new-prefix", "", "New project prefix (1-4 chars, letter-first)")

	projectDeleteCmd.Flags().BoolVar(&projectDeleteForce, "force", false, "Cascade-delete all children (items, plans, documents)")
	projectDeleteCmd.Flags().StringVar(&projectDeleteReassignTo, "reassign-to", "", "Move children to this project before deletion")

	projectCmd.AddCommand(projectCreateCmd)
	projectCmd.AddCommand(projectGetCmd)
	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectRenameCmd)
	projectCmd.AddCommand(projectDeleteCmd)
}
