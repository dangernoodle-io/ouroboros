package cli

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/roadmap"
)

var roadmapCmd = &cobra.Command{
	Use:   "roadmap",
	Short: "Manage per-project roadmaps",
}

// parseIntIDs converts each raw value (e.g. KB doc IDs) to an int, naming
// the offending value in the error rather than silently skipping it.
func parseIntIDs(vals []string) ([]int, error) {
	if len(vals) == 0 {
		return nil, nil
	}
	out := make([]int, 0, len(vals))
	for _, v := range vals {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return nil, fmt.Errorf("invalid id %q: %w", v, err)
		}
		out = append(out, n)
	}
	return out, nil
}

// parseBlockers parses raw --blocked-by values, each "project:ref[:note]",
// into []roadmap.Blocker. SplitN with 3 parts means note may itself contain
// colons; only the first two colons are treated as delimiters. project and
// ref are both required — a blocker missing either renders a malformed
// "blocked by :" line downstream, so it errors here instead.
func parseBlockers(vals []string) ([]roadmap.Blocker, error) {
	if len(vals) == 0 {
		return nil, nil
	}
	out := make([]roadmap.Blocker, 0, len(vals))
	for _, raw := range vals {
		parts := strings.SplitN(raw, ":", 3)
		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid --blocked-by %q: want project:ref[:note]", raw)
		}
		project := strings.TrimSpace(parts[0])
		ref := strings.TrimSpace(parts[1])
		if project == "" {
			return nil, fmt.Errorf("invalid --blocked-by %q: missing project", raw)
		}
		if ref == "" {
			return nil, fmt.Errorf("invalid --blocked-by %q: missing ref", raw)
		}
		bl := roadmap.Blocker{Project: project, Ref: ref}
		if len(parts) == 3 {
			bl.Note = parts[2]
		}
		out = append(out, bl)
	}
	return out, nil
}

func parseSection(s string) (roadmap.Section, error) {
	section := roadmap.Section(s)
	if !roadmap.ValidSection(section) {
		return "", fmt.Errorf("invalid section %q", s)
	}
	return section, nil
}

// show

var (
	roadmapShowBy        string
	roadmapShowComponent string
	roadmapShowEpic      string
)

var roadmapShowCmd = &cobra.Command{
	Use:   "show <project>",
	Short: "Print the roadmap as Markdown",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runRoadmapShow(cmd.OutOrStdout(), db, args[0], roadmapShowBy, roadmapShowComponent, roadmapShowEpic)
		})
	},
}

func runRoadmapShow(w io.Writer, db *sql.DB, project, by, component, epic string) error {
	rm, err := roadmap.Load(db, project)
	if err != nil {
		return fmt.Errorf("roadmap: %w", err)
	}
	rm = roadmap.Filter(rm, component, epic)
	epicLabels := backlog.EpicLabels(db, roadmap.EpicIDs(rm))
	fmt.Fprint(w, roadmap.RenderMarkdown(rm, by, epicLabels, nil))
	return nil
}

// add

var (
	roadmapAddSection   string
	roadmapAddTitle     string
	roadmapAddBody      string
	roadmapAddComponent string
	roadmapAddWhy       string
	roadmapAddResume    string
	roadmapAddKB        []string
	roadmapAddTicket    []string
	roadmapAddBlockedBy []string
	roadmapAddEpic      string
	roadmapAddPosition  int
)

var roadmapAddCmd = &cobra.Command{
	Use:   "add <project>",
	Short: "Add a roadmap item",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if roadmapAddSection == "" {
			return errors.New("roadmap: --section is required")
		}
		if roadmapAddTitle == "" {
			return errors.New("roadmap: --title is required")
		}
		section, err := parseSection(roadmapAddSection)
		if err != nil {
			return fmt.Errorf("roadmap: %w", err)
		}
		kbIDs, err := parseIntIDs(roadmapAddKB)
		if err != nil {
			return fmt.Errorf("roadmap: %w", err)
		}
		blockers, err := parseBlockers(roadmapAddBlockedBy)
		if err != nil {
			return fmt.Errorf("roadmap: %w", err)
		}

		item := roadmap.Item{
			Title:         roadmapAddTitle,
			Body:          roadmapAddBody,
			Component:     roadmapAddComponent,
			Why:           roadmapAddWhy,
			ResumeTrigger: roadmapAddResume,
			KB:            kbIDs,
			Ticket:        roadmapAddTicket,
			BlockedBy:     blockers,
			Epic:          roadmapAddEpic,
		}

		// AddItem's position is presence-signaled (mirrors move/reorder) —
		// item.Position is never an ordering input, so only pass position
		// through when --position was actually set.
		var position []int
		if cmd.Flags().Changed("position") {
			position = []int{roadmapAddPosition}
		}

		return withDB(func(db *sql.DB) error {
			return runRoadmapAdd(cmd.OutOrStdout(), db, args[0], section, item, position...)
		})
	},
}

func runRoadmapAdd(w io.Writer, db *sql.DB, project string, section roadmap.Section, item roadmap.Item, position ...int) error {
	var id int
	err := roadmap.Mutate(db, project, func(rm *roadmap.Roadmap) error {
		var err error
		id, err = roadmap.AddItem(rm, section, item, position...)
		return err
	})
	if err != nil {
		return fmt.Errorf("roadmap: %w", err)
	}
	fmt.Fprintf(w, "added item %d to %s/%s\n", id, project, section)
	return nil
}

// update

var (
	roadmapUpdateTitle     string
	roadmapUpdateBody      string
	roadmapUpdateComponent string
	roadmapUpdateWhy       string
	roadmapUpdateResume    string
	roadmapUpdateSection   string
	roadmapUpdateKB        []string
	roadmapUpdateTicket    []string
	roadmapUpdateBlockedBy []string
	roadmapUpdateEpic      string
)

var roadmapUpdateCmd = &cobra.Command{
	Use:   "update <project> <id>",
	Short: "Update roadmap item fields",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("roadmap: invalid id %q: %w", args[1], err)
		}

		var patch roadmap.Patch
		if cmd.Flags().Changed("title") {
			patch.Title = &roadmapUpdateTitle
		}
		if cmd.Flags().Changed("body") {
			patch.Body = &roadmapUpdateBody
		}
		if cmd.Flags().Changed("component") {
			patch.Component = &roadmapUpdateComponent
		}
		if cmd.Flags().Changed("why") {
			patch.Why = &roadmapUpdateWhy
		}
		if cmd.Flags().Changed("resume") {
			patch.ResumeTrigger = &roadmapUpdateResume
		}
		if cmd.Flags().Changed("kb") {
			kbIDs, err := parseIntIDs(roadmapUpdateKB)
			if err != nil {
				return fmt.Errorf("roadmap: %w", err)
			}
			patch.KB = &kbIDs
		}
		if cmd.Flags().Changed("ticket") {
			patch.Ticket = &roadmapUpdateTicket
		}
		if cmd.Flags().Changed("blocked-by") {
			blockers, err := parseBlockers(roadmapUpdateBlockedBy)
			if err != nil {
				return fmt.Errorf("roadmap: %w", err)
			}
			patch.BlockedBy = &blockers
		}
		if cmd.Flags().Changed("epic") {
			patch.Epic = &roadmapUpdateEpic
		}

		var toSection roadmap.Section
		move := cmd.Flags().Changed("section")
		if move {
			toSection, err = parseSection(roadmapUpdateSection)
			if err != nil {
				return fmt.Errorf("roadmap: %w", err)
			}
		}

		return withDB(func(db *sql.DB) error {
			return runRoadmapUpdate(cmd.OutOrStdout(), db, args[0], id, patch, move, toSection)
		})
	},
}

func runRoadmapUpdate(w io.Writer, db *sql.DB, project string, id int, patch roadmap.Patch, move bool, toSection roadmap.Section) error {
	err := roadmap.Mutate(db, project, func(rm *roadmap.Roadmap) error {
		if err := roadmap.UpdateItem(rm, id, patch); err != nil {
			return err
		}
		if move {
			return roadmap.MoveItem(rm, id, toSection)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("roadmap: %w", err)
	}
	fmt.Fprintf(w, "updated item %d in %s\n", id, project)
	return nil
}

// move

var (
	roadmapMoveTo       string
	roadmapMovePosition int
)

var roadmapMoveCmd = &cobra.Command{
	Use:   "move <project> <id>",
	Short: "Move a roadmap item between sections",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("roadmap: invalid id %q: %w", args[1], err)
		}
		if roadmapMoveTo == "" {
			return errors.New("roadmap: --to is required")
		}
		section, err := parseSection(roadmapMoveTo)
		if err != nil {
			return fmt.Errorf("roadmap: %w", err)
		}

		var position []int
		if cmd.Flags().Changed("position") {
			position = []int{roadmapMovePosition}
		}

		return withDB(func(db *sql.DB) error {
			return runRoadmapMove(cmd.OutOrStdout(), db, args[0], id, section, position...)
		})
	},
}

func runRoadmapMove(w io.Writer, db *sql.DB, project string, id int, section roadmap.Section, position ...int) error {
	err := roadmap.Mutate(db, project, func(rm *roadmap.Roadmap) error {
		return roadmap.MoveItem(rm, id, section, position...)
	})
	if err != nil {
		return fmt.Errorf("roadmap: %w", err)
	}
	fmt.Fprintf(w, "moved item %d to %s\n", id, section)
	return nil
}

// reorder

var roadmapReorderPosition int

var roadmapReorderCmd = &cobra.Command{
	Use:   "reorder <project> <id>",
	Short: "Set a roadmap item's sort position within its section/lane",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("roadmap: invalid id %q: %w", args[1], err)
		}
		if !cmd.Flags().Changed("position") {
			return errors.New("roadmap: --position is required")
		}
		return withDB(func(db *sql.DB) error {
			return runRoadmapReorder(cmd.OutOrStdout(), db, args[0], id, roadmapReorderPosition)
		})
	},
}

func runRoadmapReorder(w io.Writer, db *sql.DB, project string, id int, position int) error {
	err := roadmap.Mutate(db, project, func(rm *roadmap.Roadmap) error {
		return roadmap.ReorderItem(rm, id, position)
	})
	if err != nil {
		return fmt.Errorf("roadmap: %w", err)
	}
	fmt.Fprintf(w, "reordered item %d to position %d\n", id, position)
	return nil
}

// done

var roadmapDoneCmd = &cobra.Command{
	Use:   "done <project> <id>",
	Short: "Mark a roadmap item done",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("roadmap: invalid id %q: %w", args[1], err)
		}
		return withDB(func(db *sql.DB) error {
			return runRoadmapDone(cmd.OutOrStdout(), db, args[0], id)
		})
	},
}

func runRoadmapDone(w io.Writer, db *sql.DB, project string, id int) error {
	err := roadmap.Mutate(db, project, func(rm *roadmap.Roadmap) error {
		return roadmap.MarkDone(rm, id)
	})
	if err != nil {
		return fmt.Errorf("roadmap: %w", err)
	}
	fmt.Fprintf(w, "marked item %d done\n", id)
	return nil
}

// remove

var roadmapRemoveCmd = &cobra.Command{
	Use:   "remove <project> <id>",
	Short: "Remove a roadmap item",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("roadmap: invalid id %q: %w", args[1], err)
		}
		return withDB(func(db *sql.DB) error {
			return runRoadmapRemove(cmd.OutOrStdout(), db, args[0], id)
		})
	},
}

func runRoadmapRemove(w io.Writer, db *sql.DB, project string, id int) error {
	err := roadmap.Mutate(db, project, func(rm *roadmap.Roadmap) error {
		return roadmap.RemoveItem(rm, id)
	})
	if err != nil {
		return fmt.Errorf("roadmap: %w", err)
	}
	fmt.Fprintf(w, "removed item %d from %s\n", id, project)
	return nil
}

const roadmapSectionsHelp = "now, next, deferred, parked, dropped, or done"

func init() {
	roadmapAddCmd.Flags().StringVar(&roadmapAddSection, "section", "", "Section: "+roadmapSectionsHelp+" (required)")
	roadmapAddCmd.Flags().StringVar(&roadmapAddTitle, "title", "", "Item title (required)")
	roadmapAddCmd.Flags().StringVar(&roadmapAddBody, "body", "", "Item body")
	roadmapAddCmd.Flags().StringVar(&roadmapAddComponent, "component", "", "Component")
	roadmapAddCmd.Flags().StringVar(&roadmapAddWhy, "why", "", "Why parked (parked items)")
	roadmapAddCmd.Flags().StringVar(&roadmapAddResume, "resume", "", "Resume trigger (parked items)")
	roadmapAddCmd.Flags().StringSliceVar(&roadmapAddKB, "kb", nil, "KB doc IDs (repeatable, comma-split)")
	roadmapAddCmd.Flags().StringSliceVar(&roadmapAddTicket, "ticket", nil, "Ticket IDs (repeatable, comma-split)")
	roadmapAddCmd.Flags().StringArrayVar(&roadmapAddBlockedBy, "blocked-by", nil, "project:ref[:note] blocker (repeatable; note may contain colons/commas)")
	roadmapAddCmd.Flags().StringVar(&roadmapAddEpic, "epic", "", "Epic backlog item id (single-valued)")
	roadmapAddCmd.Flags().IntVar(&roadmapAddPosition, "position", 0, "Sort position within the section")

	roadmapUpdateCmd.Flags().StringVar(&roadmapUpdateTitle, "title", "", "New title")
	roadmapUpdateCmd.Flags().StringVar(&roadmapUpdateBody, "body", "", "New body")
	roadmapUpdateCmd.Flags().StringVar(&roadmapUpdateComponent, "component", "", "New component (single-valued)")
	roadmapUpdateCmd.Flags().StringVar(&roadmapUpdateWhy, "why", "", "New why-parked")
	roadmapUpdateCmd.Flags().StringVar(&roadmapUpdateResume, "resume", "", "New resume trigger")
	roadmapUpdateCmd.Flags().StringVar(&roadmapUpdateSection, "section", "", "Move to section: "+roadmapSectionsHelp)
	roadmapUpdateCmd.Flags().StringSliceVar(&roadmapUpdateKB, "kb", nil, "KB doc IDs (repeatable, comma-split; replaces existing)")
	roadmapUpdateCmd.Flags().StringSliceVar(&roadmapUpdateTicket, "ticket", nil, "Ticket IDs (repeatable, comma-split; replaces existing)")
	roadmapUpdateCmd.Flags().StringArrayVar(&roadmapUpdateBlockedBy, "blocked-by", nil, "project:ref[:note] blocker (repeatable; note may contain colons/commas; replaces existing)")
	roadmapUpdateCmd.Flags().StringVar(&roadmapUpdateEpic, "epic", "", "New epic backlog item id (single-valued)")

	roadmapMoveCmd.Flags().StringVar(&roadmapMoveTo, "to", "", "Target section: "+roadmapSectionsHelp+" (required)")
	roadmapMoveCmd.Flags().IntVar(&roadmapMovePosition, "position", 0, "Sort position within the target section")

	roadmapReorderCmd.Flags().IntVar(&roadmapReorderPosition, "position", 0, "Sort position within the item's section (required)")

	roadmapShowCmd.Flags().StringVar(&roadmapShowBy, "by", "component", `Markdown grouping axis: "component" or "epic"`)
	roadmapShowCmd.Flags().StringVar(&roadmapShowComponent, "component", "", "Filter by component")
	roadmapShowCmd.Flags().StringVar(&roadmapShowEpic, "epic", "", "Filter by epic backlog item id")

	roadmapCmd.AddCommand(roadmapShowCmd)
	roadmapCmd.AddCommand(roadmapAddCmd)
	roadmapCmd.AddCommand(roadmapUpdateCmd)
	roadmapCmd.AddCommand(roadmapMoveCmd)
	roadmapCmd.AddCommand(roadmapReorderCmd)
	roadmapCmd.AddCommand(roadmapDoneCmd)
	roadmapCmd.AddCommand(roadmapRemoveCmd)
}
