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

func backupCommit(bk *backup.Backup, msg string) {
	if bk == nil {
		return
	}
	if err := bk.Commit(msg); err != nil {
		log.Printf("backup: %v", err)
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
				// MCP callers don't need internal project_id; project name is implicit in the ID prefix
				item.ProjectID = 0

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
