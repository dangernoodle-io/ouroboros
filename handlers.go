package main

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
		id, hasID := req.GetArguments()["id"].(string)
		if hasID && id != "" {
			// Check for update fields
			fields := make(map[string]string)
			for _, key := range []string{"priority", "title", "description", "status"} {
				if v, ok := req.GetArguments()[key].(string); ok && v != "" {
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

				item, err := backlog.UpdateItem(d, id, fields)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}

				backupCommit(bk, fmt.Sprintf("update: %s", id))
				return jsonResult(item)
			}

			// Get mode — id only, no update fields
			item, err := backlog.GetItem(d, id)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(item)
		}

		// Check for create mode (project + priority + title all present)
		projectName, _ := req.GetArguments()["project"].(string)
		priority, _ := req.GetArguments()["priority"].(string)
		title, _ := req.GetArguments()["title"].(string)

		if projectName != "" && priority != "" && title != "" {
			if _, err := parsePriority(priority); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			proj, err := resolveProject(d, projectName)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			desc := ""
			if v, ok := req.GetArguments()["description"].(string); ok {
				desc = v
			}

			item, err := backlog.AddItem(d, proj.ID, proj.Prefix, priority, title, desc)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			backupCommit(bk, fmt.Sprintf("add: %s %s", item.ID, title))
			return jsonResult(item)
		}

		// List mode — apply filters
		var f backlog.ItemFilter

		if projectName != "" {
			proj, err := resolveProject(d, projectName)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			f.ProjectID = &proj.ID
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

		items, err := backlog.ListItems(d, f)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if len(items) == 0 {
			return mcp.NewToolResultText("no items"), nil
		}

		var lines []string
		for _, item := range items {
			lines = append(lines, fmt.Sprintf("%s %s [%s] %s", item.ID, item.Priority, item.Status, item.Title))
		}
		return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
	}
}

func handlePlan(d *sql.DB, bk *backup.Backup) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		idFloat, hasID := req.GetArguments()["id"].(float64)

		if hasID {
			id := int64(idFloat)

			fields := make(map[string]string)
			for _, key := range []string{"title", "content", "status"} {
				if v, ok := req.GetArguments()[key].(string); ok && v != "" {
					fields[key] = v
				}
			}

			if len(fields) > 0 {
				plan, err := backlog.UpdatePlan(d, id, fields)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				backupCommit(bk, fmt.Sprintf("plan update: %d", id))
				return jsonResult(plan)
			}

			plan, err := backlog.GetPlan(d, id)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonResult(plan)
		}

		title, hasTitle := req.GetArguments()["title"].(string)
		if hasTitle && title != "" {
			content := ""
			if v, ok := req.GetArguments()["content"].(string); ok {
				content = v
			}

			var projectID *int64
			if v, ok := req.GetArguments()["project"].(string); ok && v != "" {
				proj, err := resolveProject(d, v)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				projectID = &proj.ID
			}

			var itemID *string
			if v, ok := req.GetArguments()["item_id"].(string); ok && v != "" {
				if _, err := backlog.GetItem(d, v); err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				itemID = &v
			}

			plan, err := backlog.CreatePlan(d, title, content, projectID, itemID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			backupCommit(bk, fmt.Sprintf("plan: %s", title))
			return jsonResult(plan)
		}

		// List mode
		var f backlog.PlanFilter
		if v, ok := req.GetArguments()["project"].(string); ok && v != "" {
			proj, err := resolveProject(d, v)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			f.ProjectID = &proj.ID
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
