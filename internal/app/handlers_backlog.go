package app

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/backup"
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

func derivePrefix(d *sql.DB, name string) (string, error) {
	base := strings.ToUpper(name)
	if len(base) < 2 {
		base = base + "X"
	}
	prefix := base[:2]

	projects, err := backlog.ListProjects(d)
	if err != nil {
		return "", err
	}

	existing := make(map[string]bool)
	for _, p := range projects {
		existing[p.Prefix] = true
	}

	if !existing[prefix] {
		return prefix, nil
	}

	for i := 1; i <= 9; i++ {
		candidate := fmt.Sprintf("%c%d", prefix[0], i)
		if !existing[candidate] {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("cannot derive unique prefix for: %s", name)
}

func backupCommit(bk *backup.Backup, msg string) {
	if bk == nil {
		return
	}
	if err := bk.Commit(msg); err != nil {
		log.Printf("backup: %v", err)
	}
}

func handleProject(d *sql.DB, bk *backup.Backup) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, ok := req.GetArguments()["name"].(string)
		if !ok || name == "" {
			projects, err := backlog.ListProjects(d)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if projects == nil {
				projects = []backlog.Project{}
			}
			return jsonResult(projects)
		}

		prefix, err := derivePrefix(d, name)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		proj, err := backlog.CreateProject(d, name, prefix)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		backupCommit(bk, fmt.Sprintf("project: %s (%s)", name, prefix))
		return jsonResult(proj)
	}
}

func handleItem(d *sql.DB, bk *backup.Backup) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Check for delete_ids[] batch delete
		deleteIDs := parseStringSlice(req.GetArguments(), "delete_ids")
		if len(deleteIDs) > 0 {
			affected, err := backlog.DeleteItems(d, deleteIDs)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			backupCommit(bk, fmt.Sprintf("deleted %d items", affected))
			return jsonResult(map[string]interface{}{
				"deleted": affected,
			})
		}

		// Check for ids[] batch fetch
		ids := parseStringSlice(req.GetArguments(), "ids")
		if len(ids) > 0 {
			verbose, _ := req.GetArguments()["verbose"].(bool)
			items := make([]interface{}, 0, len(ids))

			for _, id := range ids {
				item, err := backlog.GetItem(d, id)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				if item == nil {
					// Omit misses
					continue
				}

				if !verbose {
					item.Notes = ""
				}

				items = append(items, item)
			}

			return jsonResult(items)
		}

		// Check for entries[] batch write (mixed create/update)
		entries := parseEntriesArray(req.GetArguments(), "entries")
		if len(entries) > 0 {
			verbose, _ := req.GetArguments()["verbose"].(bool)
			results := make([]interface{}, 0, len(entries))
			writeCount := 0

			for _, e := range entries {
				// Check if this is an update (has id) or create (no id)
				if entryID, ok := e["id"].(string); ok && entryID != "" {
					// Update mode
					fields := make(map[string]string)
					for _, key := range []string{"priority", "title", "description", "notes", "status", "component"} {
						if v, ok := e[key].(string); ok && v != "" {
							fields[key] = v
						}
					}

					if len(fields) > 0 {
						// Validate priority if present
						if p, ok := fields["priority"]; ok {
							if _, err := parsePriority(p); err != nil {
								return mcp.NewToolResultError(err.Error()), nil
							}
						}

						item, err := backlog.UpdateItem(d, entryID, fields)
						if err != nil {
							return mcp.NewToolResultError(err.Error()), nil
						}

						writeCount++

						if !verbose {
							item.Notes = ""
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

						component := ""
						if v, ok := e["component"].(string); ok {
							component = v
						}

						item, err := backlog.AddItem(d, proj.ID, proj.Prefix, priority, title, desc, notes, component)
						if err != nil {
							return mcp.NewToolResultError(err.Error()), nil
						}

						writeCount++

						results = append(results, map[string]interface{}{
							"id":     item.ID,
							"action": "create",
						})
					}
				}
			}

			// Single backup commit at end with batch count
			if writeCount > 0 {
				backupCommit(bk, fmt.Sprintf("batch: %d items written", writeCount))
			}

			return jsonResult(results)
		}

		// List mode — apply filters
		var f backlog.ItemFilter

		projectNames := parseStringSlice(req.GetArguments(), "projects")
		if len(projectNames) > 0 {
			ids, err := resolveProjects(d, projectNames)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			f.ProjectIDs = ids
		}
		if v, ok := req.GetArguments()["priority_min"].(string); ok && v != "" {
			n, err := parsePriority(v)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			f.PriorityMin = &n
		}
		if v, ok := req.GetArguments()["priority_max"].(string); ok && v != "" {
			n, err := parsePriority(v)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			f.PriorityMax = &n
		}
		if v, ok := req.GetArguments()["status"].(string); ok && v != "" {
			f.Status = &v
		}
		if v, ok := req.GetArguments()["component"].(string); ok {
			f.Component = &v
		}

		items, err := backlog.ListItems(d, f)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

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
}

func handlePlan(d *sql.DB, bk *backup.Backup) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Check for ids[] batch fetch (int64)
		ids := parseInt64Slice(req.GetArguments(), "ids")
		if len(ids) > 0 {
			plans := make([]interface{}, 0, len(ids))

			for _, id := range ids {
				plan, err := backlog.GetPlan(d, id)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				if plan == nil {
					// Omit misses
					continue
				}

				plans = append(plans, plan)
			}

			return jsonResult(plans)
		}

		// Check for entries[] batch write (mixed create/update)
		entries := parseEntriesArray(req.GetArguments(), "entries")
		if len(entries) > 0 {
			results := make([]interface{}, 0, len(entries))
			writeCount := 0

			for _, e := range entries {
				// Check if this is an update (has id) or create (no id)
				if idFloat, ok := e["id"].(float64); ok && idFloat != 0 {
					// Update mode
					id := int64(idFloat)
					fields := make(map[string]string)
					for _, key := range []string{"title", "content", "status"} {
						if v, ok := e[key].(string); ok && v != "" {
							fields[key] = v
						}
					}

					if len(fields) > 0 {
						plan, err := backlog.UpdatePlan(d, id, fields)
						if err != nil {
							return mcp.NewToolResultError(err.Error()), nil
						}

						writeCount++

						results = append(results, map[string]interface{}{
							"id":     plan.ID,
							"action": "update",
						})
					}
				} else {
					// Create mode
					title, _ := e["title"].(string)
					content, _ := e["content"].(string)

					if title != "" {
						var projectID *int64
						if v, ok := e["project"].(string); ok && v != "" {
							proj, err := resolveProject(d, v)
							if err != nil {
								return mcp.NewToolResultError(err.Error()), nil
							}
							projectID = &proj.ID
						}

						var itemID *string
						if v, ok := e["item_id"].(string); ok && v != "" {
							if _, err := backlog.GetItem(d, v); err != nil {
								return mcp.NewToolResultError(err.Error()), nil
							}
							itemID = &v
						}

						plan, err := backlog.CreatePlan(d, title, content, projectID, itemID)
						if err != nil {
							return mcp.NewToolResultError(err.Error()), nil
						}

						writeCount++

						results = append(results, map[string]interface{}{
							"id":     plan.ID,
							"action": "create",
						})
					}
				}
			}

			// Single backup commit at end with batch count
			if writeCount > 0 {
				backupCommit(bk, fmt.Sprintf("batch: %d plans written", writeCount))
			}

			return jsonResult(results)
		}

		// List mode
		var f backlog.PlanFilter
		projectNames := parseStringSlice(req.GetArguments(), "projects")
		if len(projectNames) > 0 {
			ids, err := resolveProjects(d, projectNames)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			f.ProjectIDs = ids
		}
		if v, ok := req.GetArguments()["status"].(string); ok && v != "" {
			f.Status = &v
		}

		plans, err := backlog.ListPlans(d, f)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if plans == nil {
			plans = []backlog.Plan{}
		}
		return jsonResult(plans)
	}
}

func handleConfig(d *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, hasKey := req.GetArguments()["key"].(string)
		value, hasValue := req.GetArguments()["value"].(string)

		if !hasKey || key == "" {
			cfg, err := backlog.GetAllConfig(d)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(cfg)
		}

		if !hasValue || value == "" {
			v, err := backlog.GetConfig(d, key)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(map[string]string{"key": key, "value": v})
		}

		if err := backlog.SetConfig(d, key, value); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(map[string]interface{}{"key": key, "value": value, "updated": true})
	}
}
