package cli

import (
	"database/sql"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/backlog"
)

// maxBacklogCreateDescChars mirrors handlers_backlog_v2.go's 500-char create
// description cap (a create-only guard; UpdateItem's own MaxItemDescBytes is
// a much larger byte cap enforced independently by the core).
const maxBacklogCreateDescChars = 500

var (
	backlogCreateProjectFlag     string
	backlogCreatePriorityFlag    string
	backlogCreateTitleFlag       string
	backlogCreateComponentFlag   string
	backlogCreateEpicFlag        string
	backlogCreateDescriptionFlag string
	backlogCreateNotesFlag       string
)

var backlogCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new backlog item",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if backlogCreateProjectFlag == "" {
			return fmt.Errorf("--project is required")
		}
		if backlogCreatePriorityFlag == "" {
			return fmt.Errorf("--priority is required")
		}
		if backlogCreateTitleFlag == "" {
			return fmt.Errorf("--title is required")
		}
		return withDB(func(db *sql.DB) error {
			return runBacklogCreate(cmd.OutOrStdout(), db, backlogCreateRequest{
				Project:     backlogCreateProjectFlag,
				Priority:    backlogCreatePriorityFlag,
				Title:       backlogCreateTitleFlag,
				Component:   backlogCreateComponentFlag,
				Epic:        backlogCreateEpicFlag,
				Description: backlogCreateDescriptionFlag,
				Notes:       backlogCreateNotesFlag,
			})
		})
	},
}

// backlogCreateRequest is `backlog create`'s raw flag capture, translated
// into a backlog.AddItem call by runBacklogCreate.
type backlogCreateRequest struct {
	Project     string
	Priority    string
	Title       string
	Component   string
	Epic        string
	Description string
	Notes       string
}

// runBacklogCreate builds a new backlog item. Status is always "open" (the
// core hardcodes it — see backlog.addItemExec); there is no --status flag.
// Priority validation deliberately reuses the existing, already-shared
// backlog.ParsePriority (case-insensitive) rather than backlog.
// ParsePriorityStrict (OU-329's unified strict P0-P6 validator, shared by
// the MCP write/read paths) — CLI create intentionally accepts the more
// lenient form. The parsed value is then normalized to
// uppercase before being persisted, since AddItem stores the raw string
// verbatim and downstream sorting/filtering (e.g. buildItemFilterWhere's
// CAST(SUBSTR(priority,2)...)) expects the "P<n>" shape produced here.
func runBacklogCreate(w io.Writer, db *sql.DB, req backlogCreateRequest) error {
	if _, ok := backlog.ParsePriority(req.Priority); !ok {
		return fmt.Errorf("backlog create: invalid priority %q: expected P0-P6", req.Priority)
	}
	priority := strings.ToUpper(strings.TrimSpace(req.Priority))

	proj, err := backlog.GetProjectByName(db, req.Project)
	if err != nil {
		return fmt.Errorf("backlog create: %w", err)
	}

	if req.Epic != "" {
		if _, err := backlog.GetItem(db, req.Epic); err != nil {
			return fmt.Errorf("backlog create: epic item %q not found", req.Epic)
		}
	}

	if len(req.Description) > maxBacklogCreateDescChars {
		return fmt.Errorf("backlog create: description exceeds %d char hard cap (got %d); move narrative into the notes field", maxBacklogCreateDescChars, len(req.Description))
	}

	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, priority, req.Title, req.Description, req.Notes, req.Component, req.Epic)
	if err != nil {
		return fmt.Errorf("backlog create: %w", err)
	}
	return printJSON(w, item)
}

var (
	backlogUpdatePriorityFlag    string
	backlogUpdateTitleFlag       string
	backlogUpdateStatusFlag      string
	backlogUpdateComponentFlag   string
	backlogUpdateEpicFlag        string
	backlogUpdateDescriptionFlag string
	backlogUpdateNotesFlag       string
	backlogUpdateAppendNotesFlag bool
)

var backlogUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a backlog item",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fields := backlogUpdateFields{AppendNotes: backlogUpdateAppendNotesFlag}
		if cmd.Flags().Changed("priority") {
			fields.Priority = &backlogUpdatePriorityFlag
		}
		if cmd.Flags().Changed("title") {
			fields.Title = &backlogUpdateTitleFlag
		}
		if cmd.Flags().Changed("status") {
			fields.Status = &backlogUpdateStatusFlag
		}
		if cmd.Flags().Changed("component") {
			fields.Component = &backlogUpdateComponentFlag
		}
		if cmd.Flags().Changed("epic") {
			fields.Epic = &backlogUpdateEpicFlag
		}
		if cmd.Flags().Changed("description") {
			fields.Description = &backlogUpdateDescriptionFlag
		}
		if cmd.Flags().Changed("notes") {
			fields.Notes = &backlogUpdateNotesFlag
		}
		return withDB(func(db *sql.DB) error {
			return runBacklogUpdate(cmd.OutOrStdout(), db, args[0], fields)
		})
	},
}

// backlogUpdateFields is `backlog update`'s raw flag capture: a nil pointer
// means the flag was not passed (leave that field untouched); a non-nil
// pointer to an empty string means the flag WAS passed with an empty value
// (clear that field) — this distinction requires cmd.Flags().Changed, not a
// "non-empty string" check, so an explicit `--epic ""` (clear) is
// distinguishable from omitting --epic entirely (no change).
type backlogUpdateFields struct {
	Priority    *string
	Title       *string
	Status      *string
	Component   *string
	Epic        *string
	Description *string
	Notes       *string
	AppendNotes bool
}

// runBacklogUpdate builds a fields map from only the flags the caller
// actually changed (backlog.UpdateItem treats an empty map as a silent
// no-op, so "nothing changed" must be caught here as an error instead).
func runBacklogUpdate(w io.Writer, db *sql.DB, id string, f backlogUpdateFields) error {
	fields := make(map[string]string)

	if f.Priority != nil {
		if _, ok := backlog.ParsePriority(*f.Priority); !ok {
			return fmt.Errorf("backlog update: invalid priority %q: expected P0-P6", *f.Priority)
		}
		fields["priority"] = strings.ToUpper(strings.TrimSpace(*f.Priority))
	}
	if f.Title != nil {
		// Unlike epic/description/notes, title is NOT a clearable field —
		// matches the MCP handler, which treats an explicit empty title as
		// a no-op rather than blanking it.
		if *f.Title == "" {
			return fmt.Errorf("backlog update: title cannot be empty")
		}
		fields["title"] = *f.Title
	}
	if f.Status != nil {
		if *f.Status != "open" && *f.Status != "done" {
			return fmt.Errorf("backlog update: invalid status %q: must be one of open, done", *f.Status)
		}
		fields["status"] = *f.Status
	}
	if f.Component != nil {
		// Unlike title, component (and epic below) DOES clear on an
		// explicit empty value — a deliberate divergence from the MCP
		// handler's empty-means-no-op convention, since the core already
		// supports clearing these fields and description/notes clear the
		// same way. See OU-338 to reconcile the MCP side (unify the
		// CLI/MCP empty-value semantics across all four fields).
		fields["component"] = *f.Component
	}
	if f.Epic != nil {
		epic := *f.Epic
		if epic != "" {
			if _, err := backlog.GetItem(db, epic); err != nil {
				return fmt.Errorf("backlog update: epic item %q not found", epic)
			}
		}
		fields["epic"] = epic
	}
	if f.Description != nil {
		desc := *f.Description
		if desc != "" && len(desc) > maxBacklogCreateDescChars {
			return fmt.Errorf("backlog update: description exceeds %d char hard cap (got %d); move narrative into the notes field", maxBacklogCreateDescChars, len(desc))
		}
		fields["description"] = desc
	}
	if f.Notes != nil {
		switch {
		case f.AppendNotes:
			if *f.Notes != "" {
				appended, err := appendBacklogUpdateNotes(db, id, *f.Notes)
				if err != nil {
					return fmt.Errorf("backlog update: %w", err)
				}
				fields["notes"] = appended
			}
		default:
			fields["notes"] = *f.Notes
		}
	}

	if len(fields) == 0 {
		return fmt.Errorf("backlog update: no fields to update")
	}

	item, err := backlog.UpdateItem(db, id, fields)
	if err != nil {
		return fmt.Errorf("backlog update: %w", err)
	}
	return printJSON(w, item)
}

// appendBacklogUpdateNotes is the CLI-local twin of
// internal/app/shared.go's appendBacklogNotes: read the item's current
// notes and return them with addition appended, \n\n-separated (no leading
// separator when existing notes are empty). Deliberately re-implemented
// here rather than imported from internal/app — that package is the MCP
// server's handler layer, not a dependency internal/cli should take on for
// one helper.
func appendBacklogUpdateNotes(db *sql.DB, id, addition string) (string, error) {
	current, err := backlog.GetItem(db, id)
	if err != nil {
		return "", err
	}
	if current.Notes == "" {
		return addition, nil
	}
	return current.Notes + "\n\n" + addition, nil
}

var backlogDeleteCmd = &cobra.Command{
	Use:   "delete <id> [<id>...]",
	Short: "Delete one or more backlog items by id",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runBacklogDelete(cmd.OutOrStdout(), db, args)
		})
	},
}

// runBacklogDelete dedups the requested ids, then performs an all-or-nothing
// existence pre-check (mirroring kb_delete.go's runKBDelete) via one batched
// backlog.GetItemsByIDs call: any missing id aborts with nothing deleted,
// rather than a partial delete. backlog.DeleteItems itself is already
// transactional and cascades edge cleanup.
func runBacklogDelete(w io.Writer, db *sql.DB, idArgs []string) error {
	seen := make(map[string]bool, len(idArgs))
	ids := make([]string, 0, len(idArgs))
	for _, s := range idArgs {
		id := strings.TrimSpace(s)
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}

	fetched, err := backlog.GetItemsByIDs(db, ids)
	if err != nil {
		return fmt.Errorf("backlog delete: %w", err)
	}
	found := make(map[string]bool, len(fetched))
	for _, item := range fetched {
		found[item.ID] = true
	}
	var missing []string
	for _, id := range ids {
		if !found[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("backlog delete: item(s) not found: %s — nothing deleted", strings.Join(missing, ", "))
	}

	if _, err := backlog.DeleteItems(db, ids); err != nil {
		return fmt.Errorf("backlog delete: %w", err)
	}

	fmt.Fprintf(w, "deleted %d item(s): %s\n", len(ids), strings.Join(ids, ", "))
	return nil
}

func init() {
	backlogCreateCmd.Flags().StringVar(&backlogCreateProjectFlag, "project", "", "Project name (required)")
	backlogCreateCmd.Flags().StringVar(&backlogCreatePriorityFlag, "priority", "", "Priority P0-P6 (required)")
	backlogCreateCmd.Flags().StringVar(&backlogCreateTitleFlag, "title", "", "Item title (required)")
	backlogCreateCmd.Flags().StringVar(&backlogCreateComponentFlag, "component", "", "Component")
	backlogCreateCmd.Flags().StringVar(&backlogCreateEpicFlag, "epic", "", "Epic item id this item belongs to")
	backlogCreateCmd.Flags().StringVar(&backlogCreateDescriptionFlag, "description", "", "Description (500 char hard cap)")
	backlogCreateCmd.Flags().StringVar(&backlogCreateNotesFlag, "notes", "", "Notes")

	backlogUpdateCmd.Flags().StringVar(&backlogUpdatePriorityFlag, "priority", "", "New priority P0-P6")
	backlogUpdateCmd.Flags().StringVar(&backlogUpdateTitleFlag, "title", "", "New title")
	backlogUpdateCmd.Flags().StringVar(&backlogUpdateStatusFlag, "status", "", "New status (open or done; required-valued, not clearable)")
	backlogUpdateCmd.Flags().StringVar(&backlogUpdateComponentFlag, "component", "", `New component ("" clears)`)
	backlogUpdateCmd.Flags().StringVar(&backlogUpdateEpicFlag, "epic", "", `New epic item id ("" clears)`)
	backlogUpdateCmd.Flags().StringVar(&backlogUpdateDescriptionFlag, "description", "", `New description ("" clears)`)
	backlogUpdateCmd.Flags().StringVar(&backlogUpdateNotesFlag, "notes", "", `New notes ("" clears, unless --append-notes)`)
	backlogUpdateCmd.Flags().BoolVar(&backlogUpdateAppendNotesFlag, "append-notes", false, "Append --notes to the existing notes instead of replacing them")

	backlogCmd.AddCommand(backlogCreateCmd)
	backlogCmd.AddCommand(backlogUpdateCmd)
	backlogCmd.AddCommand(backlogDeleteCmd)
}
