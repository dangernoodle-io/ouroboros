package app

import (
	"github.com/dangernoodle-io/mcpkit"
	"github.com/dangernoodle-io/mcpkit/mcpx"
)

// Tool-level descriptions for the four MCP tools (kept tight per the
// token-conservation principle in CLAUDE.md).
const (
	descQueryToolV2   = "Read entries (required domain: kb|backlog|roadmap). ids[] = exact fetch; query or queries[] (kb batch) = full-text search; else a filtered list. verbose=true adds notes/edges (ids[] only). roadmap: format=md|html&by=component|epic."
	descKBToolV2      = "Create/update knowledge entries via entries[]: with id updates that doc in place (partial), else upserts by type+project+category+title. content [[Title]] autolinks create explains edges. Read via query domain=kb."
	descBacklogToolV2 = "Create/update/delete backlog items via entries[] (id = update, else create) or delete_ids[]. Read via query domain=backlog."
	descRoadmapToolV2 = "Mutate the per-project roadmap singleton (now/next/deferred/parked/dropped/done) via op=add|update|move|reorder|done|remove. Items group by component and epic (both single-valued). Read via query domain=roadmap."
)

// queryCapability is the mcpkit Capability for the single query read tool
// (OU-323, collapsing OU-1's get+search). Carries ReadOnlyHint, matching
// buildServer's toolAnnotation(mcp.ToBoolPtr(true), nil, nil) mapping
// (server.go).
type queryCapability struct{ st *serverState }

func (c queryCapability) Attach(r *mcpkit.Registrar) error {
	mcpkit.AddTool(r, &mcpx.Tool{
		Name:        "query",
		Description: descQueryToolV2,
		Annotations: &mcpx.ToolAnnotations{ReadOnlyHint: true},
	}, mcpkit.ReadOnly, handleQueryV2(c.st))
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
