package app

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/edges"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func parsePriority(s string) (int, error) {
	if len(s) != 2 || s[0] != 'P' {
		return 0, fmt.Errorf("invalid priority: %s (expected P0-P6)", s)
	}
	n, err := strconv.Atoi(string(s[1]))
	if err != nil || n < 0 || n > 6 {
		return 0, fmt.Errorf("invalid priority: %s (expected P0-P6)", s)
	}
	return n, nil
}

func resolveProject(d *sql.DB, name string) (*backlog.Project, error) {
	return backlog.GetProjectByName(d, name)
}

// edgeSpec is one {label,target} element of a backlog entry's edges[]:
// an item->item edge, source = the item being written.
type edgeSpec struct {
	Label  string
	Target string
}

// parseEdgeSpecs extracts and validates the optional edges[] array on a
// backlog write entry. A present-but-malformed element (missing
// label/target, or an unrecognized label) is a hard error, not a silent
// skip — a wrong/dropped edge is worse than a rejected write.
func parseEdgeSpecs(e map[string]interface{}) ([]edgeSpec, error) {
	raw, ok := e["edges"].([]interface{})
	if !ok {
		return nil, nil
	}

	specs := make([]edgeSpec, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid edges[] entry: expected object with label and target")
		}
		label, _ := m["label"].(string)
		target, _ := m["target"].(string)
		if label == "" || target == "" {
			return nil, fmt.Errorf("edges[] entry requires label and target")
		}
		if !edges.ValidLabel(label) {
			return nil, fmt.Errorf("invalid edge label %q: must be one of blocks, relates, explains", label)
		}
		specs = append(specs, edgeSpec{Label: label, Target: target})
	}
	return specs, nil
}

// validateEpicTx confirms a non-empty epic value resolves to an existing
// backlog item (alias-aware, via backlog.GetItem — the same resolution path
// EpicLabels uses, so a renamed epic still validates), mirroring
// linkEdgesTx's target-exists check for edges: a typo'd/dangling epic id
// must not be silently accepted. An empty epic (clearing the field) is
// always allowed and skips validation.
func validateEpicTx(tx *sql.Tx, epic string) error {
	if epic == "" {
		return nil
	}
	if _, err := backlog.GetItem(tx, epic); err != nil {
		return fmt.Errorf("epic item %q not found: %w", epic, err)
	}
	return nil
}

// linkEdgesTx creates each spec as an item->item edge sourced from itemID,
// on the given transaction so the edge links commit atomically with the
// item write that produced them (see handleBacklog). Validates each
// target item exists first — a typo'd target must not silently produce a
// permanently dangling edge.
func linkEdgesTx(tx *sql.Tx, itemID string, projectID int64, specs []edgeSpec) error {
	for _, spec := range specs {
		if _, err := backlog.GetItem(tx, spec.Target); err != nil {
			return fmt.Errorf("edge target item %q not found: %w", spec.Target, err)
		}
		if _, err := edges.Link(tx, edges.TypeItem, itemID, spec.Label, edges.TypeItem, spec.Target, projectID); err != nil {
			return err
		}
	}
	return nil
}

func resolveProjects(d *sql.DB, names []string) ([]int64, error) {
	ids := make([]int64, 0, len(names))
	for _, name := range names {
		proj, err := resolveProject(d, name)
		if err != nil {
			return nil, err
		}
		ids = append(ids, proj.ID)
	}
	return ids, nil
}

// parseItemFilter builds a backlog.ItemFilter from projects/priority_min/priority_max/status/component
// arguments, shared by domain=backlog reads (get list mode, search).
func parseItemFilter(d *sql.DB, req mcp.CallToolRequest) (backlog.ItemFilter, error) {
	var f backlog.ItemFilter

	projectNames := parseStringSlice(req.GetArguments(), "projects")
	if len(projectNames) > 0 {
		ids, err := resolveProjects(d, projectNames)
		if err != nil {
			return f, err
		}
		f.ProjectIDs = ids
	}
	if v, ok := req.GetArguments()["priority_min"].(string); ok && v != "" {
		n, err := parsePriority(v)
		if err != nil {
			return f, err
		}
		f.PriorityMin = &n
	}
	if v, ok := req.GetArguments()["priority_max"].(string); ok && v != "" {
		n, err := parsePriority(v)
		if err != nil {
			return f, err
		}
		f.PriorityMax = &n
	}
	if v, ok := req.GetArguments()["status"].(string); ok && v != "" {
		f.Status = &v
	}
	if v, ok := req.GetArguments()["component"].(string); ok {
		f.Component = &v
	}
	// epics_only takes precedence over epic when both are set — they
	// can't sensibly combine (see ItemFilter.EpicsOnly/Epic).
	if v, ok := req.GetArguments()["epics_only"].(bool); ok && v {
		f.EpicsOnly = true
	} else if v, ok := req.GetArguments()["epic"].(string); ok && v != "" {
		f.Epic = &v
	}
	if v, ok := req.GetArguments()["since"].(string); ok && v != "" {
		cutoff, err := backlog.ParseSinceCutoff(v)
		if err != nil {
			return f, err
		}
		f.CreatedSince = &cutoff
	}
	if v, ok := req.GetArguments()["sort"].(string); ok && v != "" {
		switch v {
		case "created":
			f.SortByCreated = true
		default:
			return f, fmt.Errorf("invalid sort value %q: expected \"created\"", v)
		}
	}
	if v, ok := req.GetArguments()["limit"].(float64); ok {
		f.Limit = int(v)
	}
	return f, nil
}

// itemLinesResult renders a compact one-line-per-item text result (or "no items").
func itemLinesResult(items []backlog.Item) (*mcp.CallToolResult, error) {
	if len(items) == 0 {
		return mcp.NewToolResultText("no items"), nil
	}

	var lines []string
	for _, item := range items {
		componentStr := ""
		if item.Component != "" {
			componentStr = fmt.Sprintf("(%s) ", item.Component)
		}
		lines = append(lines, fmt.Sprintf("%s %s [%s] %s%s", item.ID, item.Priority, item.Status, componentStr, item.Title))
	}
	return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
}

// itemWithEdges wraps a backlog item with its edges sidecar, only populated
// on verbose=true reads (see getBacklogItems).
type itemWithEdges struct {
	*backlog.Item
	Edges []edges.Edge `json:"edges,omitempty"`
}

// getBacklogItems handles domain=backlog reads for the get tool: ids[] fetch, or filters list.
func getBacklogItems(d *sql.DB, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ids := parseStringSlice(req.GetArguments(), "ids")

	// backlog ids are prefixed strings (e.g. "B1-728"). If the caller
	// supplied an ids[] array where ANY element failed to parse as a string
	// (e.g. a JSON number like 728, or a null), that's a type mismatch, not
	// "no ids" — silently dropping the bad element and returning the rest
	// would return silently-wrong data. Correctly-typed-but-nonexistent ids
	// still parse fine here (parsed length == raw length) and fall through
	// to the ids-fetch branch below (empty result), unaffected. An
	// explicitly empty ids[] array (len 0 == len 0) falls through to
	// filter/list mode.
	if rawIDs, ok := req.GetArguments()["ids"].([]interface{}); ok && len(ids) != len(rawIDs) {
		return mcp.NewToolResultError(`ids for domain=backlog must be prefixed strings like "B1-728"`), nil //nolint:nilerr
	}

	if len(ids) > 0 {
		verbose, _ := req.GetArguments()["verbose"].(bool)

		// backlog.GetItems does one batched IN-query (falling back to a
		// per-id alias-resolving lookup only on a miss) instead of one
		// GetItem call per id; misses cause an error (vs kb which omits
		// them), and results are already re-sorted to requested order.
		fetched, err := backlog.GetItems(d, ids)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		items := make([]interface{}, 0, len(fetched))
		for _, item := range fetched {
			if !verbose {
				item.Notes = ""
				item.ProjectID = 0
				items = append(items, item)
				continue
			}
			item.ProjectID = 0

			edgeList, err := edges.EdgesFor(d, edges.TypeItem, item.ID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			items = append(items, itemWithEdges{Item: item, Edges: edgeList})
		}

		return jsonResult(items)
	}

	f, err := parseItemFilter(d, req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	items, err := backlog.ListItems(d, f)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return itemLinesResult(items)
}

// searchBacklogItems handles domain=backlog search: FTS query over title/description/notes,
// honoring the same filters as get domain=backlog list mode.
func searchBacklogItems(d *sql.DB, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, _ := req.GetArguments()["query"].(string)
	if query == "" {
		return mcp.NewToolResultError("query is required"), nil //nolint:nilerr
	}

	f, err := parseItemFilter(d, req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	items, err := backlog.SearchItems(d, query, f)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return itemLinesResult(items)
}

// handleBacklog is the backlog write tool (destructive): delete_ids[] batch delete,
// or entries[] batch create/update. Reads live under get/search domain=backlog.
func handleBacklog(d *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Check for delete_ids[] batch delete
		deleteIDs := parseStringSlice(req.GetArguments(), "delete_ids")
		if len(deleteIDs) > 0 {
			affected, err := backlog.DeleteItems(d, deleteIDs)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(map[string]interface{}{
				"deleted": affected,
			})
		}

		// Check for entries[] batch write (mixed create/update)
		entries := parseEntriesArray(req.GetArguments(), "entries")
		if len(entries) > 0 {
			results := make([]interface{}, 0, len(entries))

			for _, e := range entries {
				edgeSpecs, err := parseEdgeSpecs(e)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}

				// Check if this is an update (has id) or create (no id)
				if entryID, ok := e["id"].(string); ok && entryID != "" {
					// Update mode
					fields := make(map[string]string)
					for _, key := range []string{"priority", "title", "description", "notes", "status"} {
						if v, ok := e[key].(string); ok && v != "" {
							fields[key] = v
						}
					}
					// component/epic are single-valued — a caller passing a
					// list/object for either is a misuse, not a silent no-op.
					component, err := scalarStringArg(e, "component")
					if err != nil {
						return mcp.NewToolResultError(err.Error()), nil
					}
					if component != "" {
						fields["component"] = component
					}
					epic, err := scalarStringArg(e, "epic")
					if err != nil {
						return mcp.NewToolResultError(err.Error()), nil
					}
					if epic != "" {
						fields["epic"] = epic
					}

					if len(fields) > 0 || len(edgeSpecs) > 0 {
						// Validate priority if present
						if p, ok := fields["priority"]; ok {
							if _, err := parsePriority(p); err != nil {
								return mcp.NewToolResultError(err.Error()), nil
							}
						}

						// The field update and its edges[] links commit
						// atomically: a failing (e.g. nonexistent-target)
						// edge must not leave a partially-applied write.
						tx, err := d.Begin()
						if err != nil {
							return mcp.NewToolResultError(err.Error()), nil
						}
						defer tx.Rollback() //nolint:errcheck

						if err := validateEpicTx(tx, epic); err != nil {
							return mcp.NewToolResultError(err.Error()), nil
						}

						var item *backlog.Item
						if len(fields) > 0 {
							item, err = backlog.UpdateItemTx(tx, entryID, fields)
						} else {
							item, err = backlog.GetItem(tx, entryID)
						}
						if err != nil {
							return mcp.NewToolResultError(err.Error()), nil
						}

						if err := linkEdgesTx(tx, item.ID, item.ProjectID, edgeSpecs); err != nil {
							return mcp.NewToolResultError(err.Error()), nil
						}

						if err := tx.Commit(); err != nil {
							return mcp.NewToolResultError(err.Error()), nil
						}

						results = append(results, map[string]interface{}{
							"id":     item.ID,
							"action": "update",
						})
					}
				} else {
					// Create mode
					projectName, _ := e["project"].(string)
					priority, _ := e["priority"].(string)
					title, _ := e["title"].(string)

					if projectName != "" && priority != "" && title != "" {
						if _, err := parsePriority(priority); err != nil {
							return mcp.NewToolResultError(err.Error()), nil
						}

						proj, err := resolveProject(d, projectName)
						if err != nil {
							return mcp.NewToolResultError(err.Error()), nil
						}

						desc := ""
						if v, ok := e["description"].(string); ok {
							desc = v
						}

						if len(desc) > 500 {
							return mcp.NewToolResultError(fmt.Sprintf("description exceeds 500 char hard cap (got %d). Move narrative into the notes field.", len(desc))), nil //nolint:nilerr
						}

						notes := ""
						if v, ok := e["notes"].(string); ok {
							notes = v
						}

						// component/epic are single-valued — a caller passing a
						// list/object for either is a misuse, not a silent no-op.
						component, err := scalarStringArg(e, "component")
						if err != nil {
							return mcp.NewToolResultError(err.Error()), nil
						}
						epic, err := scalarStringArg(e, "epic")
						if err != nil {
							return mcp.NewToolResultError(err.Error()), nil
						}

						// The item create and its edges[] links commit
						// atomically: a failing (e.g. nonexistent-target)
						// edge must not leave a half-created item behind.
						tx, err := d.Begin()
						if err != nil {
							return mcp.NewToolResultError(err.Error()), nil
						}
						defer tx.Rollback() //nolint:errcheck

						if err := validateEpicTx(tx, epic); err != nil {
							return mcp.NewToolResultError(err.Error()), nil
						}

						item, err := backlog.AddItemTx(tx, proj.ID, proj.Prefix, priority, title, desc, notes, component, epic)
						if err != nil {
							return mcp.NewToolResultError(err.Error()), nil
						}

						if err := linkEdgesTx(tx, item.ID, proj.ID, edgeSpecs); err != nil {
							return mcp.NewToolResultError(err.Error()), nil
						}

						if err := tx.Commit(); err != nil {
							return mcp.NewToolResultError(err.Error()), nil
						}

						results = append(results, map[string]interface{}{
							"id":     item.ID,
							"action": "create",
						})
					}
				}
			}

			return jsonResult(results)
		}

		return mcp.NewToolResultError("delete_ids or entries is required"), nil //nolint:nilerr
	}
}
