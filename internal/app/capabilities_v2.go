package app

import (
	"database/sql"

	"github.com/dangernoodle-io/mcpkit"
	"github.com/dangernoodle-io/mcpkit/mcpx"
)

// descGetToolV2/descSearchToolV2 mirror the mcp-go tool descriptions in
// server.go verbatim, so the tool surface presented to a client is unchanged
// across the mcp-go/mcpkit seam.
const (
	descGetToolV2     = "Fetch entries by ID or filters (required domain: kb|backlog|roadmap). ids[] returns exact matches; omit ids for a filtered list."
	descSearchToolV2  = "Full-text search over entries (required domain: kb|backlog|roadmap). domain=kb supports query or a batched queries[]; domain=backlog/roadmap take a single query (backlog: title/description/notes)."
	descKBToolV2      = "Create or update knowledge entries via entries[]: id present = update that doc in place (partial, e.g. retitle without creating a duplicate), else upsert by type+project+category+title. content supports [[Title]] autolinks (an item id or a same-project KB title) creating explains edges. Reads live under get/search domain=kb."
	descBacklogToolV2 = "Create, update, or delete backlog items: entries[] (id present = update, else create) or delete_ids[]. Reads live under get/search domain=backlog."
	descRoadmapToolV2 = "Mutate the per-project roadmap singleton (now/next/deferred/parked/dropped/done sections) via op=add|update|move|reorder|done|remove. Items carry two single-valued grouping axes: component (structural) and epic (optional; an epic IS a backlog item). Reads live under get/search domain=roadmap."
)

// getCapability and searchCapability are mcpkit Capabilities for the get and
// search read tools (OU-1). Both carry ReadOnlyHint, matching buildServer's
// toolAnnotation(mcp.ToBoolPtr(true), nil, nil) mapping (server.go).
type getCapability struct{ db *sql.DB }

func (c getCapability) Attach(r *mcpkit.Registrar) error {
	mcpkit.AddTool(r, &mcpx.Tool{
		Name:        "get",
		Description: descGetToolV2,
		Annotations: &mcpx.ToolAnnotations{ReadOnlyHint: true},
	}, handleGetV2(c.db))
	return nil
}

type searchCapability struct{ db *sql.DB }

func (c searchCapability) Attach(r *mcpkit.Registrar) error {
	mcpkit.AddTool(r, &mcpx.Tool{
		Name:        "search",
		Description: descSearchToolV2,
		Annotations: &mcpx.ToolAnnotations{ReadOnlyHint: true},
	}, handleSearchV2(c.db))
	return nil
}

// kbCapability is the mcpkit Capability for the kb write tool (OU-2).
// Carries IdempotentHint, matching buildServer's toolAnnotation(nil, nil,
// mcp.ToBoolPtr(true)) mapping (server.go).
type kbCapability struct{ db *sql.DB }

func (c kbCapability) Attach(r *mcpkit.Registrar) error {
	mcpkit.AddTool(r, &mcpx.Tool{
		Name:        "kb",
		Description: descKBToolV2,
		Annotations: &mcpx.ToolAnnotations{IdempotentHint: true},
	}, handleKBV2(c.db))
	return nil
}

// backlogCapability is the mcpkit Capability for the backlog write tool
// (OU-3). Carries DestructiveHint, matching buildServer's
// toolAnnotation(nil, mcp.ToBoolPtr(true), nil) mapping (server.go).
type backlogCapability struct{ db *sql.DB }

func (c backlogCapability) Attach(r *mcpkit.Registrar) error {
	mcpkit.AddTool(r, &mcpx.Tool{
		Name:        "backlog",
		Description: descBacklogToolV2,
		Annotations: &mcpx.ToolAnnotations{DestructiveHint: mcpx.BoolPtr(true)},
	}, handleBacklogV2(c.db))
	return nil
}

// roadmapCapability is the mcpkit Capability for the roadmap write tool
// (OU-4, the last write tool before OU-5 cutover). Carries DestructiveHint,
// matching buildServer's toolAnnotation(nil, mcp.ToBoolPtr(true), nil)
// mapping (server.go).
type roadmapCapability struct{ db *sql.DB }

func (c roadmapCapability) Attach(r *mcpkit.Registrar) error {
	mcpkit.AddTool(r, &mcpx.Tool{
		Name:        "roadmap",
		Description: descRoadmapToolV2,
		Annotations: &mcpx.ToolAnnotations{DestructiveHint: mcpx.BoolPtr(true)},
	}, handleRoadmapV2(c.db))
	return nil
}
