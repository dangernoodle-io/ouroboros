package app

import (
	"database/sql"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	descFilterProjects = "Filter by project names"
	descFilterTypes    = "Filter by types (kb only)"
	descFilterCats     = "Filter by categories (kb only)"
	descLimit          = "Limit, default 10, max 500"
	descVerbose        = "Include notes on ids[] fetch only, default false; filter/list results are always compact regardless"
	descDomain         = `Required: "kb", "backlog", or "roadmap"`
	descPriorityMin    = "Min priority P0-P6 (backlog only)"
	descPriorityMax    = "Max priority P0-P6 (backlog only)"
	descStatus         = "open or done (backlog only)"
	descComponent      = "Component tag filter (backlog: subproject/plugin; roadmap: structural grouping axis, single-valued)"
	descEpic           = "Epic backlog item id filter — that epic's children (backlog, roadmap; single-valued — an epic IS a backlog item, see the EPIC: convention)"
	descEpicsOnly      = "List only epic items, EPIC:-titled (backlog only); takes precedence over epic"
	descSince          = `Created-time window for backlog items: a duration (24h, 7d), a date (2006-01-02), or an RFC3339 timestamp (backlog only)`
	descSort           = `"created" sorts backlog items newest-first (backlog only)`
	descRoadmapBy      = `Grouping axis: "component" (default) or "epic"; the other axis renders as an inline chip (roadmap get format=md|html only)`
	descFormat         = "structured, md, or html, default structured (roadmap only)"
	descRoadmapOp      = `Required: "add", "update", "move", "reorder", "done", or "remove"`
	descRoadmapProject = "Project name (roadmap singleton)"
	descRoadmapSection = "now|next|deferred|parked|dropped|done"
)

const serverInstructions = `Persist decisions and track work items across conversations.
get/search are reads (required domain: kb|backlog|roadmap); kb/backlog/roadmap are writes.

- Search before writing — avoid duplicates; kb upserts by type+project+category+title when id is absent, or updates in place when id is present (use this to retitle).
- Default response is summary; verbose=true only when full content/notes are needed.
- roadmap is a per-project singleton (now/next/deferred/parked/dropped/done sections); items carry two single-valued grouping axes, component and epic (an epic is a backlog item); get format=md|html&by=component|epic renders Markdown/HTML grouped on that axis, filterable by component/epic.
- Edges (blocks/relates/explains) link items/kb docs: backlog entries[].edges[] creates them inline at write time; kb content [[Title]] autolinks; get verbose=true surfaces an edges sidecar; CLI link/unlink/ls edges for retrofits.
- Checkpoint after multi-step tasks; persist non-obvious decisions, update or delete stale ones.
- Never run sqlite3/raw SQL against the ouroboros DB file — on a tool failure, stop and report rather than improvising.`

// buildServer creates a new MCP server with all tools registered at startup.
func buildServer(db *sql.DB, version string) *server.MCPServer {
	s := server.NewMCPServer("ouroboros", version,
		server.WithToolCapabilities(true),
		server.WithInstructions(serverInstructions),
	)

	s.AddTool(mcp.NewTool("get",
		mcp.WithDescription("Fetch entries by ID or filters (required domain: kb|backlog|roadmap). ids[] returns exact matches; omit ids for a filtered list."),
		mcp.WithString("domain", mcp.Required(), mcp.Description(descDomain)),
		mcp.WithArray("ids", mcp.Description("IDs to fetch (kb: document IDs, backlog: item IDs)")),
		mcp.WithArray("types", mcp.Description(descFilterTypes), mcp.Items(map[string]any{"type": "string"})),
		mcp.WithArray("projects", mcp.Description(descFilterProjects)),
		mcp.WithArray("categories", mcp.Description(descFilterCats), mcp.Items(map[string]any{"type": "string"})),
		mcp.WithString("query", mcp.Description("Full-text search (kb only)")),
		mcp.WithArray("tags", mcp.Description("Filter by tags, all match (kb only)")),
		mcp.WithString("priority_min", mcp.Description(descPriorityMin)),
		mcp.WithString("priority_max", mcp.Description(descPriorityMax)),
		mcp.WithString("status", mcp.Description(descStatus)),
		mcp.WithString("component", mcp.Description(descComponent)),
		mcp.WithString("epic", mcp.Description(descEpic)),
		mcp.WithBoolean("epics_only", mcp.Description(descEpicsOnly)),
		mcp.WithString("since", mcp.Description(descSince)),
		mcp.WithString("sort", mcp.Description(descSort)),
		mcp.WithString("by", mcp.Description(descRoadmapBy)),
		mcp.WithNumber("limit", mcp.Description(descLimit)),
		mcp.WithBoolean("verbose", mcp.Description(descVerbose)),
		mcp.WithString("format", mcp.Description(descFormat)),
		toolAnnotation(mcp.ToBoolPtr(true), nil, nil),
	), withRecover(handleGet(db)))

	s.AddTool(mcp.NewTool("search",
		mcp.WithDescription("Full-text search over entries (required domain: kb|backlog|roadmap). domain=kb supports query or a batched queries[]; domain=backlog/roadmap take a single query (backlog: title/description/notes)."),
		mcp.WithString("domain", mcp.Required(), mcp.Description(descDomain)),
		mcp.WithString("query", mcp.Description("Single query")),
		mcp.WithArray("queries", mcp.Description("Batch queries sharing filters, kb only; response is positional [[...], [...]]")),
		mcp.WithArray("types", mcp.Description(descFilterTypes), mcp.Items(map[string]any{"type": "string"})),
		mcp.WithArray("projects", mcp.Description(descFilterProjects)),
		mcp.WithArray("categories", mcp.Description(descFilterCats), mcp.Items(map[string]any{"type": "string"})),
		mcp.WithString("priority_min", mcp.Description(descPriorityMin)),
		mcp.WithString("priority_max", mcp.Description(descPriorityMax)),
		mcp.WithString("status", mcp.Description(descStatus)),
		mcp.WithString("component", mcp.Description(descComponent)),
		mcp.WithString("epic", mcp.Description(descEpic)),
		mcp.WithBoolean("epics_only", mcp.Description(descEpicsOnly)),
		mcp.WithString("since", mcp.Description(descSince)),
		mcp.WithString("sort", mcp.Description(descSort)),
		mcp.WithNumber("limit", mcp.Description(descLimit)),
		toolAnnotation(mcp.ToBoolPtr(true), nil, nil),
	), withRecover(handleSearch(db)))

	s.AddTool(mcp.NewTool("kb",
		mcp.WithDescription("Create or update knowledge entries via entries[]: id present = update that doc in place (partial, e.g. retitle without creating a duplicate), else upsert by type+project+category+title. content supports [[Title]] autolinks (an item id or a same-project KB title) creating explains edges. Reads live under get/search domain=kb."),
		mcp.WithArray("entries", mcp.Required(), mcp.Description("Documents to create/update: {id?}, type, project, category?, title, content, notes?, tags?, metadata?. content max 500 chars; put narrative in notes")),
		toolAnnotation(nil, nil, mcp.ToBoolPtr(true)),
	), withRecover(handleKB(db)))

	s.AddTool(mcp.NewTool("backlog",
		mcp.WithDescription("Create, update, or delete backlog items: entries[] (id present = update, else create) or delete_ids[]. Reads live under get/search domain=backlog."),
		mcp.WithArray("entries", mcp.Description("Items to create/update: {id?}, project, priority, title, description?, notes?, component?, epic?, status?, edges?[{label,target}] (label: blocks|relates|explains, target: item id). description max 500 chars; put narrative in notes")),
		mcp.WithArray("delete_ids", mcp.Description("Item IDs to delete")),
		toolAnnotation(nil, mcp.ToBoolPtr(true), nil),
	), withRecover(handleBacklog(db)))

	s.AddTool(mcp.NewTool("roadmap",
		mcp.WithDescription("Mutate the per-project roadmap singleton (now/next/deferred/parked/dropped/done sections) via op=add|update|move|reorder|done|remove. Items carry two single-valued grouping axes: component (structural) and epic (optional; an epic IS a backlog item). Reads live under get/search domain=roadmap."),
		mcp.WithString("op", mcp.Required(), mcp.Description(descRoadmapOp)),
		mcp.WithString("project", mcp.Required(), mcp.Description(descRoadmapProject)),
		mcp.WithString("section", mcp.Description(descRoadmapSection+" (add only)")),
		mcp.WithString("to", mcp.Description(descRoadmapSection+" (move only)")),
		mcp.WithNumber("id", mcp.Description("Item ID (update/move/reorder/done/remove)")),
		mcp.WithString("title", mcp.Description("Item title (add/update)")),
		mcp.WithString("body", mcp.Description("Item body (add/update)")),
		mcp.WithString("component", mcp.Description("Component tag, single-valued (add/update)")),
		mcp.WithString("why", mcp.Description("Why parked (add/update)")),
		mcp.WithString("resume_trigger", mcp.Description("Resume trigger (add/update)")),
		mcp.WithArray("kb", mcp.Description("Related KB doc IDs (add/update)")),
		mcp.WithArray("ticket", mcp.Description("Ticket refs (add/update)")),
		mcp.WithArray("blocked_by", mcp.Description("Cross-project blockers: {project,ref,note} (add/update)")),
		mcp.WithString("epic", mcp.Description("Epic backlog item id, single-valued (add/update)")),
		mcp.WithNumber("position", mcp.Description("Sort position within the section (add/move/reorder; required for reorder)")),
		toolAnnotation(nil, mcp.ToBoolPtr(true), nil),
	), withRecover(handleRoadmap(db)))

	return s
}

// toolAnnotation constructs a mcp.WithToolAnnotation option with only the
// specified hint fields set (others remain nil to drop from JSON via omitempty).
func toolAnnotation(readOnly, destructive, idempotent *bool) mcp.ToolOption {
	return mcp.WithToolAnnotation(mcp.ToolAnnotation{
		ReadOnlyHint:    readOnly,
		DestructiveHint: destructive,
		IdempotentHint:  idempotent,
		OpenWorldHint:   nil, // always nil: local SQLite, no external calls
	})
}
