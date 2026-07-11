package app

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dangernoodle.io/ouroboros/internal/backlog"
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

// scalarStringArg returns args[key] as a string when present ("" when
// absent), or an error when present with a non-string type. component and
// epic are single-valued grouping axes — no item may carry more than one
// value per axis — so a caller passing a list/object for either is a
// misuse, not a silent no-op.
func scalarStringArg(args map[string]interface{}, key string) (string, error) {
	v, ok := args[key]
	if !ok {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a single string value (single-valued, not a list)", key)
	}
	return s, nil
}

// scalarStringPtrArg is scalarStringArg's Patch-pointer counterpart: nil
// when key is absent (unchanged), a pointer to the value when present and
// valid, or an error when present but non-scalar.
func scalarStringPtrArg(args map[string]interface{}, key string) (*string, error) {
	if _, ok := args[key]; !ok {
		return nil, nil
	}
	s, err := scalarStringArg(args, key)
	if err != nil {
		return nil, err
	}
	return &s, nil
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

// handleRoadmap is the roadmap write tool (destructive): op=add|update|move|reorder|done|remove
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
		case "reorder":
			return handleRoadmapReorder(db, bk, project, args)
		case "done":
			return handleRoadmapDone(db, bk, project, args)
		case "remove":
			return handleRoadmapRemove(db, bk, project, args)
		default:
			return mcp.NewToolResultError(`op is required: must be "add", "update", "move", "reorder", "done", or "remove"`), nil //nolint:nilerr
		}
	}
}

// validSectionsMsg lists the valid section names for error messages.
const validSectionsMsg = "now, next, deferred, parked, dropped, or done"

func handleRoadmapAdd(db *sql.DB, bk *backup.Backup, project string, args map[string]interface{}) (*mcp.CallToolResult, error) {
	section := roadmap.Section(strArg(args, "section"))
	if !roadmap.ValidSection(section) {
		return mcp.NewToolResultError(fmt.Sprintf("invalid section %q: must be %s", section, validSectionsMsg)), nil //nolint:nilerr
	}

	blockers, err := parseBlockers(args, "blocked_by")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	component, err := scalarStringArg(args, "component")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	epic, err := scalarStringArg(args, "epic")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	item := roadmap.Item{
		Title:         strArg(args, "title"),
		Body:          strArg(args, "body"),
		Component:     component,
		Why:           strArg(args, "why"),
		ResumeTrigger: strArg(args, "resume_trigger"),
		KB:            parseIntSlice(args, "kb"),
		Ticket:        parseStringSlice(args, "ticket"),
		BlockedBy:     blockers,
		Epic:          epic,
	}

	// AddItem's position is presence-signaled (mirrors move/reorder) —
	// item.Position is never an ordering input, so only pass position
	// through when the caller actually supplied the arg.
	var position []int
	if v, ok := args["position"].(float64); ok {
		position = []int{int(v)}
	}

	var id int
	mutateErr := roadmap.Mutate(db, project, func(rm *roadmap.Roadmap) error {
		var err error
		id, err = roadmap.AddItem(rm, section, item, position...)
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

	component, err := scalarStringPtrArg(args, "component")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	epic, err := scalarStringPtrArg(args, "epic")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	patch := roadmap.Patch{
		Title:         stringPtrArg(args, "title"),
		Body:          stringPtrArg(args, "body"),
		Component:     component,
		Why:           stringPtrArg(args, "why"),
		ResumeTrigger: stringPtrArg(args, "resume_trigger"),
		KB:            intSlicePtrArg(args, "kb"),
		Ticket:        stringSlicePtrArg(args, "ticket"),
		BlockedBy:     blockedBy,
		Epic:          epic,
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
		return mcp.NewToolResultError(fmt.Sprintf("invalid section %q: must be %s", to, validSectionsMsg)), nil //nolint:nilerr
	}

	var position []int
	if v, ok := args["position"].(float64); ok {
		position = []int{int(v)}
	}

	mutateErr := roadmap.Mutate(db, project, func(rm *roadmap.Roadmap) error {
		return roadmap.MoveItem(rm, id, to, position...)
	})
	if mutateErr != nil {
		return mcp.NewToolResultError(mutateErr.Error()), nil
	}

	backupCommit(bk, fmt.Sprintf("roadmap: moved item %d to %s (%s)", id, to, project))
	return jsonResult(map[string]interface{}{"id": id, "section": string(to)})
}

func handleRoadmapReorder(db *sql.DB, bk *backup.Backup, project string, args map[string]interface{}) (*mcp.CallToolResult, error) {
	id, ok := intArg(args)
	if !ok {
		return mcp.NewToolResultError("id is required"), nil //nolint:nilerr
	}

	position, ok := args["position"].(float64)
	if !ok {
		return mcp.NewToolResultError("position is required"), nil //nolint:nilerr
	}

	mutateErr := roadmap.Mutate(db, project, func(rm *roadmap.Roadmap) error {
		return roadmap.ReorderItem(rm, id, int(position))
	})
	if mutateErr != nil {
		return mcp.NewToolResultError(mutateErr.Error()), nil
	}

	backupCommit(bk, fmt.Sprintf("roadmap: reordered item %d to position %d (%s)", id, int(position), project))
	return jsonResult(map[string]interface{}{"id": id, "position": int(position)})
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
// component/epic filter items on their respective axis (both apply together
// when both are set); by picks the Markdown grouping axis (component|epic,
// default component) — it has no effect on structured output, which always
// returns every item's full field set.
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

	rm = roadmap.Filter(rm, strArg(args, "component"), strArg(args, "epic"))

	switch strArg(args, "format") {
	case "md":
		by := strArg(args, "by")
		epicLabels := backlog.EpicLabels(db, roadmap.EpicIDs(rm))
		return mcp.NewToolResultText(roadmap.RenderMarkdown(rm, by, epicLabels, nil)), nil
	case "html":
		by := strArg(args, "by")
		epicIDs := roadmap.EpicIDs(rm)
		epicLabels := backlog.EpicLabels(db, epicIDs)
		blockedEpics := backlog.BlockedEpicsFor(db, epicIDs)
		return mcp.NewToolResultText(roadmap.RenderHTML(rm, by, epicLabels, blockedEpics)), nil
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
