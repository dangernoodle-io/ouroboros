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
	descGetToolV2    = "Fetch entries by ID or filters (required domain: kb|backlog|roadmap). ids[] returns exact matches; omit ids for a filtered list."
	descSearchToolV2 = "Full-text search over entries (required domain: kb|backlog|roadmap). domain=kb supports query or a batched queries[]; domain=backlog/roadmap take a single query (backlog: title/description/notes)."
	descKBToolV2     = "Create or update knowledge entries via entries[]: id present = update that doc in place (partial, e.g. retitle without creating a duplicate), else upsert by type+project+category+title. content supports [[Title]] autolinks (an item id or a same-project KB title) creating explains edges. Reads live under get/search domain=kb."
)

// getCapability and searchCapability are mcpkit Capabilities for the get and
// search read tools (OU-1). Annotations (ReadOnlyHint etc.) are DEFERRED —
// mcpkit's AddTool chokepoint has no annotation support yet (MC-8 left it as
// future work) and hand-rolling go-sdk annotations here would leak go-sdk
// past the mcpx seam. This is advisory-only; a mcpkit follow-up will restore
// hints once it grows that knob.
type getCapability struct{ db *sql.DB }

func (c getCapability) Attach(r *mcpkit.Registrar) error {
	mcpkit.AddTool(r, &mcpx.Tool{Name: "get", Description: descGetToolV2}, handleGetV2(c.db))
	return nil
}

type searchCapability struct{ db *sql.DB }

func (c searchCapability) Attach(r *mcpkit.Registrar) error {
	mcpkit.AddTool(r, &mcpx.Tool{Name: "search", Description: descSearchToolV2}, handleSearchV2(c.db))
	return nil
}

// kbCapability is the mcpkit Capability for the kb write tool (OU-2).
// Annotations (idempotent hint) are DEFERRED per OU-293 -- same chokepoint
// limitation as getCapability/searchCapability, see their comment.
type kbCapability struct{ db *sql.DB }

func (c kbCapability) Attach(r *mcpkit.Registrar) error {
	mcpkit.AddTool(r, &mcpx.Tool{Name: "kb", Description: descKBToolV2}, handleKBV2(c.db))
	return nil
}
