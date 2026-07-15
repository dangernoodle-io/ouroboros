package app

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/dangernoodle-io/mcpkit/mcpx"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/edges"
)

// errBacklogEntriesOrDeleteIDsRequired is the verbatim message the old
// backlog write handler returned when neither delete_ids nor entries was
// present -- reproduced here since backlogInput schema-declares both
// omitempty (see backlogInput's comment).
const errBacklogEntriesOrDeleteIDsRequired = "delete_ids or entries is required"

// handleBacklogV2 is the backlog write tool handler. delete_ids takes
// PRIORITY over entries; the entries[] batch reuses
// handleBacklogEntriesV2's single-transaction write. st.db is read at call
// time (not at construction time) — see serverState's doc comment.
func handleBacklogV2(st *serverState) mcpx.Handler[backlogInput, any] {
	return func(_ context.Context, _ *mcpx.CallToolRequest, in backlogInput) (*mcpx.CallToolResult, any, error) {
		if len(in.DeleteIDs) > 0 {
			affected, err := backlog.DeleteItems(st.db, in.DeleteIDs)
			if err != nil {
				return mcpx.ErrorResult(err.Error()), nil, nil
			}
			return jsonResultV2(map[string]interface{}{
				"deleted": affected,
			})
		}

		if len(in.Entries) > 0 {
			return handleBacklogEntriesV2(st.db, in.Entries)
		}

		return mcpx.ErrorResult(errBacklogEntriesOrDeleteIDsRequired), nil, nil
	}
}

// buildEdgeSpecsV2 is parseEdgeSpecs' typed-input counterpart: builds
// []edgeSpec from the typed Edges []edgeInput field instead of reading a
// raw map, preserving the same hard-error validation for an empty
// label/target or an unrecognized label. A present-but-non-object edges[]
// element can't reach here at all (go-sdk decode error -- see edgeInput's
// comment); an empty/absent Edges slice returns (nil, nil), same as
// parseEdgeSpecs' not-present case.
func buildEdgeSpecsV2(in []edgeInput) ([]edgeSpec, error) {
	if len(in) == 0 {
		return nil, nil
	}

	specs := make([]edgeSpec, 0, len(in))
	for _, e := range in {
		if e.Label == "" || e.Target == "" {
			return nil, fmt.Errorf("edges[] entry requires label and target")
		}
		if !edges.ValidLabel(e.Label) {
			return nil, fmt.Errorf("invalid edge label %q: must be one of blocks, relates, explains", e.Label)
		}
		specs = append(specs, edgeSpec(e))
	}
	return specs, nil
}

// appendBacklogNotes fetches the item's current notes within tx (the
// caller's shared transaction -- no second tx, avoiding a TOCTOU race with
// the field update that follows) and returns them with addition appended,
// \n\n-separated (no leading separator when existing notes are empty).
func appendBacklogNotes(tx *sql.Tx, id, addition string) (string, error) {
	current, err := backlog.GetItem(tx, id)
	if err != nil {
		return "", err
	}
	if current.Notes == "" {
		return addition, nil
	}
	return current.Notes + "\n\n" + addition, nil
}

// handleBacklogEntriesV2 is handleBacklogEntries' typed-input counterpart:
// processes entries[] (mixed create/update) inside ONE shared transaction,
// reproducing handleBacklogEntries' behavior exactly (see its comment for
// the single-tx rationale). All reads during the batch (resolveProject,
// GetItem, validateEpicTx) run on tx, never db, per the store's
// SetMaxOpenConns(1) deadlock-avoidance constraint.
func handleBacklogEntriesV2(db *sql.DB, entries []backlogEntryInput) (*mcpx.CallToolResult, any, error) {
	results := make([]interface{}, 0, len(entries))

	// posMap tracks, per batch position, the item id that position produced
	// (create) or referred to (update) -- populated as the loop runs so a
	// later entry's "$N" epic value can back-reference an earlier one's
	// server-assigned id (see resolveEpicRef).
	posMap := make(map[int]string)

	tx, err := db.Begin()
	if err != nil {
		return mcpx.ErrorResult(err.Error()), nil, nil
	}
	defer tx.Rollback() //nolint:errcheck

	for idx, e := range entries {
		edgeSpecs, err := buildEdgeSpecsV2(e.Edges)
		if err != nil {
			return mcpx.ErrorResult(err.Error()), nil, nil
		}

		if e.ID != nil && *e.ID != "" {
			// Update mode. entryID is already known, so this position's item
			// id resolves immediately (unlike create, where the id isn't
			// assigned until AddItemTx runs below).
			entryID := *e.ID
			posMap[idx] = entryID

			fields := make(map[string]string)
			for key, v := range map[string]*string{
				"priority": e.Priority,
				"title":    e.Title,
				"status":   e.Status,
			} {
				if v != nil && *v != "" {
					fields[key] = *v
				}
			}
			// description is clear-aware (nil=preserve, ""=clear, text=replace)
			// -- unlike priority/title/status above, an explicit empty string
			// is a real value here, not "not provided" (fixes the
			// can't-clear-description quirk).
			if e.Description != nil {
				fields["description"] = *e.Description
			}
			// notes is clear-aware like description, EXCEPT when append_notes
			// is set: then notes text (if any) is appended to the existing
			// value inside this same tx (read-modify-write, no second tx) via
			// appendBacklogNotes, rather than replacing it. append_notes with
			// omitted/empty notes is a no-op (nothing to append) -- it must
			// NOT clear, unlike the default (non-append) empty-string case.
			switch {
			case e.Notes == nil:
				// omitted -> preserve, nothing to do.
			case e.AppendNotes:
				if *e.Notes != "" {
					appended, err := appendBacklogNotes(tx, entryID, *e.Notes)
					if err != nil {
						return mcpx.ErrorResult(err.Error()), nil, nil
					}
					fields["notes"] = appended
				}
			default:
				fields["notes"] = *e.Notes
			}
			// component/epic are single-valued -- an absent/empty *string
			// means "no change" (matches the old handler's scalarStringArg
			// present-but-empty == not added).
			if e.Component != nil && *e.Component != "" {
				fields["component"] = *e.Component
			}
			var epic string
			if e.Epic != nil {
				epic = *e.Epic
			}
			if epic != "" {
				epic, err = resolveEpicRef(epic, idx, len(entries), posMap)
				if err != nil {
					return mcpx.ErrorResult(err.Error()), nil, nil
				}
				fields["epic"] = epic
			}

			if len(fields) > 0 || len(edgeSpecs) > 0 {
				if p, ok := fields["priority"]; ok {
					if _, err := backlog.ParsePriorityStrict(p); err != nil {
						return mcpx.ErrorResult(err.Error()), nil, nil
					}
				}

				// The field update and its edges[] links participate in the
				// batch's single shared tx -- an error here aborts the whole
				// batch via the outer deferred Rollback, not just this entry.
				if err := validateEpicTx(tx, epic); err != nil {
					return mcpx.ErrorResult(err.Error()), nil, nil
				}

				var item *backlog.Item
				if len(fields) > 0 {
					item, err = backlog.UpdateItemTx(tx, entryID, fields)
				} else {
					item, err = backlog.GetItem(tx, entryID)
				}
				if err != nil {
					return mcpx.ErrorResult(err.Error()), nil, nil
				}

				if err := linkEdgesTx(tx, item.ID, item.ProjectID, edgeSpecs); err != nil {
					return mcpx.ErrorResult(err.Error()), nil, nil
				}

				results = append(results, map[string]interface{}{
					"id":     item.ID,
					"action": "update",
				})
			}
			continue
		}

		// Create mode.
		var projectName, priority, title string
		if e.Project != nil {
			projectName = *e.Project
		}
		if e.Priority != nil {
			priority = *e.Priority
		}
		if e.Title != nil {
			title = *e.Title
		}

		if projectName == "" || priority == "" || title == "" {
			// Silently skip -- matches handlers_backlog.go's create gate
			// (no error, no result entry).
			continue
		}

		if _, err := backlog.ParsePriorityStrict(priority); err != nil {
			return mcpx.ErrorResult(err.Error()), nil, nil
		}

		// Resolve on the shared tx, not db: the store enforces
		// SetMaxOpenConns(1), so a *sql.DB-level query while this tx holds
		// the only connection would deadlock.
		proj, err := resolveProject(tx, projectName)
		if err != nil {
			return mcpx.ErrorResult(err.Error()), nil, nil
		}

		var desc string
		if e.Description != nil {
			desc = *e.Description
		}
		if len(desc) > 500 {
			return mcpx.ErrorResult(fmt.Sprintf("description exceeds 500 char hard cap (got %d). Move narrative into the notes field.", len(desc))), nil, nil
		}

		var notes string
		if e.Notes != nil {
			notes = *e.Notes
		}

		var component string
		if e.Component != nil {
			component = *e.Component
		}
		var epic string
		if e.Epic != nil {
			epic = *e.Epic
		}
		if epic != "" {
			epic, err = resolveEpicRef(epic, idx, len(entries), posMap)
			if err != nil {
				return mcpx.ErrorResult(err.Error()), nil, nil
			}
		}

		// The item create and its edges[] links participate in the batch's
		// single shared tx -- an error here aborts the whole batch via the
		// outer deferred Rollback, not just this entry.
		if err := validateEpicTx(tx, epic); err != nil {
			return mcpx.ErrorResult(err.Error()), nil, nil
		}

		item, err := backlog.AddItemTx(tx, proj.ID, proj.Prefix, priority, title, desc, notes, component, epic)
		if err != nil {
			return mcpx.ErrorResult(err.Error()), nil, nil
		}
		posMap[idx] = item.ID

		if err := linkEdgesTx(tx, item.ID, proj.ID, edgeSpecs); err != nil {
			return mcpx.ErrorResult(err.Error()), nil, nil
		}

		results = append(results, map[string]interface{}{
			"id":     item.ID,
			"action": "create",
		})
	}

	if err := tx.Commit(); err != nil {
		return mcpx.ErrorResult(err.Error()), nil, nil
	}

	return jsonResultV2(results)
}
