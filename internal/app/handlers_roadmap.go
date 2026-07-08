package app

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dangernoodle.io/ouroboros/internal/backup"
	"dangernoodle.io/ouroboros/internal/roadmap"
	"dangernoodle.io/ouroboros/internal/store"
)

// strArg returns args[key] as a string, or "" if absent/wrong type.
func strArg(args map[string]interface{}, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

// intArg returns args["id"] as an int, with an ok flag.
func intArg(args map[string]interface{}) (int, bool) {
	if v, ok := args["id"].(float64); ok {
		return int(v), true
	}
	return 0, false
}

// stringPtrArg returns a pointer to args[key] if present as a string, else nil
// (nil signals "unchanged" to roadmap.Patch).
func stringPtrArg(args map[string]interface{}, key string) *string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return &s
		}
	}
	return nil
}

// parseIntSlice extracts an int array from tool arguments by key.
func parseIntSlice(args map[string]interface{}, key string) []int {
	raw, ok := args[key].([]interface{})
	if !ok {
		return nil
	}
	result := make([]int, 0, len(raw))
	for _, v := range raw {
		if f, ok := v.(float64); ok {
			result = append(result, int(f))
		}
	}
	return result
}

// intSlicePtrArg returns a pointer to the parsed int slice if key is present, else nil.
func intSlicePtrArg(args map[string]interface{}, key string) *[]int {
	if _, ok := args[key]; !ok {
		return nil
	}
	v := parseIntSlice(args, key)
	return &v
}

// stringSlicePtrArg returns a pointer to the parsed string slice if key is present, else nil.
func stringSlicePtrArg(args map[string]interface{}, key string) *[]string {
	if _, ok := args[key]; !ok {
		return nil
	}
	v := parseStringSlice(args, key)
	return &v
}

// parseBlockers extracts a []roadmap.Blocker from tool arguments by key.
// Every entry must carry a non-empty project and ref; malformed entries
// error loudly rather than rendering a blank "blocked by :" line later.
func parseBlockers(args map[string]interface{}, key string) ([]roadmap.Blocker, error) {
	raw, ok := args[key].([]interface{})
	if !ok {
		return nil, nil
	}
	result := make([]roadmap.Blocker, 0, len(raw))
	for i, v := range raw {
		m, ok := v.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("blocked_by[%d] must be an object with project and ref", i)
		}
		bl := roadmap.Blocker{
			Project: strArg(m, "project"),
			Ref:     strArg(m, "ref"),
			Note:    strArg(m, "note"),
		}
		if bl.Project == "" {
			return nil, fmt.Errorf("blocked_by[%d] missing project", i)
		}
		if bl.Ref == "" {
			return nil, fmt.Errorf("blocked_by[%d] missing ref", i)
		}
		result = append(result, bl)
	}
	return result, nil
}

// blockedByPtrArg returns a pointer to the parsed blockers slice if key is present, else nil.
func blockedByPtrArg(args map[string]interface{}, key string) (*[]roadmap.Blocker, error) {
	if _, ok := args[key]; !ok {
		return nil, nil
	}
	v, err := parseBlockers(args, key)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// handleRoadmap is the roadmap write tool (destructive): op=add|update|move|done|remove
// mutates the per-project roadmap singleton via roadmap.Mutate, which serializes the
// load->mutate->save cycle in a single transaction. Reads live under get/search
// domain=roadmap.
func handleRoadmap(db *sql.DB, bk *backup.Backup) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()

		project := strArg(args, "project")
		if project == "" {
			return mcp.NewToolResultError("project is required"), nil //nolint:nilerr
		}

		op := strArg(args, "op")

		switch op {
		case "add":
			return handleRoadmapAdd(db, bk, project, args)
		case "update":
			return handleRoadmapUpdate(db, bk, project, args)
		case "move":
			return handleRoadmapMove(db, bk, project, args)
		case "done":
			return handleRoadmapDone(db, bk, project, args)
		case "remove":
			return handleRoadmapRemove(db, bk, project, args)
		default:
			return mcp.NewToolResultError(`op is required: must be "add", "update", "move", "done", or "remove"`), nil //nolint:nilerr
		}
	}
}

func handleRoadmapAdd(db *sql.DB, bk *backup.Backup, project string, args map[string]interface{}) (*mcp.CallToolResult, error) {
	section := roadmap.Section(strArg(args, "section"))
	if !roadmap.ValidSection(section) {
		return mcp.NewToolResultError(fmt.Sprintf("invalid section %q: must be now, next, parked, or done", section)), nil //nolint:nilerr
	}

	blockers, err := parseBlockers(args, "blocked_by")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	item := roadmap.Item{
		Title:         strArg(args, "title"),
		Body:          strArg(args, "body"),
		Component:     strArg(args, "component"),
		Why:           strArg(args, "why"),
		ResumeTrigger: strArg(args, "resume_trigger"),
		KB:            parseIntSlice(args, "kb"),
		Ticket:        parseStringSlice(args, "ticket"),
		BlockedBy:     blockers,
	}

	var id int
	mutateErr := roadmap.Mutate(db, project, func(rm *roadmap.Roadmap) error {
		var err error
		id, err = roadmap.AddItem(rm, section, item)
		return err
	})
	if mutateErr != nil {
		return mcp.NewToolResultError(mutateErr.Error()), nil
	}

	backupCommit(bk, fmt.Sprintf("roadmap: added item %d to %s (%s)", id, section, project))
	return jsonResult(map[string]interface{}{"id": id, "section": string(section)})
}

func handleRoadmapUpdate(db *sql.DB, bk *backup.Backup, project string, args map[string]interface{}) (*mcp.CallToolResult, error) {
	id, ok := intArg(args)
	if !ok {
		return mcp.NewToolResultError("id is required"), nil //nolint:nilerr
	}

	blockedBy, err := blockedByPtrArg(args, "blocked_by")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	patch := roadmap.Patch{
		Title:         stringPtrArg(args, "title"),
		Body:          stringPtrArg(args, "body"),
		Component:     stringPtrArg(args, "component"),
		Why:           stringPtrArg(args, "why"),
		ResumeTrigger: stringPtrArg(args, "resume_trigger"),
		KB:            intSlicePtrArg(args, "kb"),
		Ticket:        stringSlicePtrArg(args, "ticket"),
		BlockedBy:     blockedBy,
	}

	mutateErr := roadmap.Mutate(db, project, func(rm *roadmap.Roadmap) error {
		return roadmap.UpdateItem(rm, id, patch)
	})
	if mutateErr != nil {
		return mcp.NewToolResultError(mutateErr.Error()), nil
	}

	backupCommit(bk, fmt.Sprintf("roadmap: updated item %d (%s)", id, project))
	return jsonResult(map[string]interface{}{"id": id})
}

func handleRoadmapMove(db *sql.DB, bk *backup.Backup, project string, args map[string]interface{}) (*mcp.CallToolResult, error) {
	id, ok := intArg(args)
	if !ok {
		return mcp.NewToolResultError("id is required"), nil //nolint:nilerr
	}

	to := roadmap.Section(strArg(args, "to"))
	if !roadmap.ValidSection(to) {
		return mcp.NewToolResultError(fmt.Sprintf("invalid section %q: must be now, next, parked, or done", to)), nil //nolint:nilerr
	}

	mutateErr := roadmap.Mutate(db, project, func(rm *roadmap.Roadmap) error {
		return roadmap.MoveItem(rm, id, to)
	})
	if mutateErr != nil {
		return mcp.NewToolResultError(mutateErr.Error()), nil
	}

	backupCommit(bk, fmt.Sprintf("roadmap: moved item %d to %s (%s)", id, to, project))
	return jsonResult(map[string]interface{}{"id": id, "section": string(to)})
}

func handleRoadmapDone(db *sql.DB, bk *backup.Backup, project string, args map[string]interface{}) (*mcp.CallToolResult, error) {
	id, ok := intArg(args)
	if !ok {
		return mcp.NewToolResultError("id is required"), nil //nolint:nilerr
	}

	mutateErr := roadmap.Mutate(db, project, func(rm *roadmap.Roadmap) error {
		return roadmap.MarkDone(rm, id)
	})
	if mutateErr != nil {
		return mcp.NewToolResultError(mutateErr.Error()), nil
	}

	backupCommit(bk, fmt.Sprintf("roadmap: marked item %d done (%s)", id, project))
	return jsonResult(map[string]interface{}{"id": id, "section": string(roadmap.SectionDone)})
}

func handleRoadmapRemove(db *sql.DB, bk *backup.Backup, project string, args map[string]interface{}) (*mcp.CallToolResult, error) {
	id, ok := intArg(args)
	if !ok {
		return mcp.NewToolResultError("id is required"), nil //nolint:nilerr
	}

	mutateErr := roadmap.Mutate(db, project, func(rm *roadmap.Roadmap) error {
		return roadmap.RemoveItem(rm, id)
	})
	if mutateErr != nil {
		return mcp.NewToolResultError(mutateErr.Error()), nil
	}

	backupCommit(bk, fmt.Sprintf("roadmap: removed item %d (%s)", id, project))
	return jsonResult(map[string]interface{}{"id": id, "removed": true})
}

// getRoadmap handles domain=roadmap reads for the get tool: the singleton roadmap
// for projects[0], as structured JSON (default) or a Markdown mirror (format=md).
func getRoadmap(db *sql.DB, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	projects := parseStringSlice(args, "projects")
	if len(projects) == 0 {
		return mcp.NewToolResultError("projects[0] (project name) is required for domain=roadmap"), nil //nolint:nilerr
	}

	rm, err := roadmap.Load(db, projects[0])
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if strArg(args, "format") == "md" {
		return mcp.NewToolResultText(roadmap.RenderMarkdown(rm)), nil
	}
	return jsonResult(rm)
}

// searchRoadmap handles domain=roadmap search: FTS over documents type=roadmap,
// reusing store.SearchDocuments.
func searchRoadmap(db *sql.DB, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	query := strArg(args, "query")
	if query == "" {
		return mcp.NewToolResultError("query is required"), nil //nolint:nilerr
	}

	projects := parseStringSlice(args, "projects")

	limit := 0
	if v, ok := args["limit"].(float64); ok {
		limit = int(v)
	}

	summaries, err := store.SearchDocuments(db, query, []string{"roadmap"}, projects, nil, limit)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return jsonResult(summaries)
}
