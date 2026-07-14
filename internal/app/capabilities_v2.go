package app

import (
	"github.com/dangernoodle-io/mcpkit"
	"github.com/dangernoodle-io/mcpkit/mcpx"
)

// Tool-level descriptions for the five MCP tools (kept tight per the
// token-conservation principle in CLAUDE.md).
const (
	descGetToolV2     = "Fetch entries by id or filters (required domain: kb|backlog|roadmap). ids[] = exact matches; omit for a filtered list."
	descSearchToolV2  = "Full-text search (required domain: kb|backlog|roadmap). kb takes query or batched queries[]; backlog/roadmap take one query."
	descKBToolV2      = "Create/update knowledge entries via entries[]: with id updates that doc in place (partial), else upserts by type+project+category+title. content [[Title]] autolinks create explains edges. Read via get/search domain=kb."
	descBacklogToolV2 = "Create/update/delete backlog items via entries[] (id = update, else create) or delete_ids[]. Read via get/search domain=backlog."
	descRoadmapToolV2 = "Mutate the per-project roadmap singleton (now/next/deferred/parked/dropped/done) via op=add|update|move|reorder|done|remove. Items group by component and epic (both single-valued). Read via get/search domain=roadmap."
)

// getCapability and searchCapability are mcpkit Capabilities for the get and
// search read tools (OU-1). Both carry ReadOnlyHint, matching buildServer's
// toolAnnotation(mcp.ToBoolPtr(true), nil, nil) mapping (server.go).
type getCapability struct{ st *serverState }

func (c getCapability) Attach(r *mcpkit.Registrar) error {
	mcpkit.AddTool(r, &mcpx.Tool{
		Name:        "get",
		Description: descGetToolV2,
		Annotations: &mcpx.ToolAnnotations{ReadOnlyHint: true},
	}, mcpkit.ReadOnly, handleGetV2(c.st))
	return nil
}

type searchCapability struct{ st *serverState }

func (c searchCapability) Attach(r *mcpkit.Registrar) error {
	mcpkit.AddTool(r, &mcpx.Tool{
		Name:        "search",
		Description: descSearchToolV2,
		Annotations: &mcpx.ToolAnnotations{ReadOnlyHint: true},
	}, mcpkit.ReadOnly, handleSearchV2(c.st))
	return nil
}

// kbCapability is the mcpkit Capability for the kb write tool (OU-2).
// Carries IdempotentHint, matching the old server's toolAnnotation(nil, nil,
// mcp.ToBoolPtr(true)) mapping (deleted at the OU-5 cutover).
type kbCapability struct{ st *serverState }

func (c kbCapability) Attach(r *mcpkit.Registrar) error {
	mcpkit.AddTool(r, &mcpx.Tool{
		Name:        "kb",
		Description: descKBToolV2,
		Annotations: &mcpx.ToolAnnotations{IdempotentHint: true},
	}, mcpkit.Write, handleKBV2(c.st))
	return nil
}

// backlogCapability is the mcpkit Capability for the backlog write tool
// (OU-3). Carries DestructiveHint, matching the old server's
// toolAnnotation(nil, mcp.ToBoolPtr(true), nil) mapping (deleted at the
// OU-5 cutover).
type backlogCapability struct{ st *serverState }

func (c backlogCapability) Attach(r *mcpkit.Registrar) error {
	mcpkit.AddTool(r, &mcpx.Tool{
		Name:        "backlog",
		Description: descBacklogToolV2,
		Annotations: &mcpx.ToolAnnotations{DestructiveHint: mcpx.BoolPtr(true)},
	}, mcpkit.Destructive, handleBacklogV2(c.st))
	return nil
}

// roadmapCapability is the mcpkit Capability for the roadmap write tool
// (OU-4, the last write tool before OU-5 cutover). Carries DestructiveHint,
// matching the old server's toolAnnotation(nil, mcp.ToBoolPtr(true), nil)
// mapping (deleted at the OU-5 cutover).
type roadmapCapability struct{ st *serverState }

func (c roadmapCapability) Attach(r *mcpkit.Registrar) error {
	mcpkit.AddTool(r, &mcpx.Tool{
		Name:        "roadmap",
		Description: descRoadmapToolV2,
		Annotations: &mcpx.ToolAnnotations{DestructiveHint: mcpx.BoolPtr(true)},
	}, mcpkit.Destructive, handleRoadmapV2(c.st))
	return nil
}
