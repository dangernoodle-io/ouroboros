package query

import (
	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/edges"
	"dangernoodle.io/ouroboros/internal/roadmap"
	"dangernoodle.io/ouroboros/internal/store"
)

// DocResult is the typed carrier for a single KB document IDs-fetch result
// (see getDocuments): it always embeds the document, with Edges populated
// only on a verbose=true fetch (nil/empty otherwise, dropped from the wire
// by the omitempty tag) — replaces the former []any Docs union (OU-328) with
// one shape covering both the plain and edges-augmented cases, byte-identical
// on the wire in both.
type DocResult struct {
	*store.Document
	Edges []edges.Edge `json:"edges,omitempty"`
}

// ItemResult is the typed carrier for a single backlog item IDs-fetch result
// (see getBacklogItems): it always embeds the item, with Edges populated
// only on a verbose=true fetch (nil/empty otherwise, dropped from the wire
// by the omitempty tag) — replaces the former []any ItemsJSON union
// (OU-328) with one shape covering both the plain and edges-augmented
// cases, byte-identical on the wire in both.
type ItemResult struct {
	*backlog.Item
	Edges []edges.Edge `json:"edges,omitempty"`
}

// Result is the structured, surface-neutral outcome of Get/Search: exactly
// one field (group) is populated, selected by the same (Domain, mode)
// combination as the Request that produced it. Callers (the MCP handlers
// today, a CLI command later) render the populated field for their own
// surface — this type carries no rendering decision beyond the
// roadmap md/html Text, which is rendering shared verbatim by both surfaces
// (see roadmap.RenderMarkdown/RenderHTML).
type Result struct {
	// kb
	Docs           []DocResult               // IDs fetch, one entry per found id
	DocSummaries   []store.DocumentSummary   // filter-list (Get) or single-query (Search)
	DocSummarySets [][]store.DocumentSummary // Search's queries[] batch (kb only)

	// backlog
	ItemsJSON []ItemResult   // IDs fetch, one entry per found id
	Items     []backlog.Item // filter-list (Get) or query match (Search); caller renders
	// Relaxed is true when Search's backlog path widened a zero-row
	// implicit-AND query to OR (OU-346, see backlog.SearchItemsRelaxed) and
	// Items came from that OR retry. Always false for Get (list/IDs mode).
	// kb's equivalent signal lives per-row on DocSummaries/DocSummarySets
	// (store.DocumentSummary.Relaxed) since queries[] batch needs a flag per
	// query, not one for the whole Result.
	Relaxed bool

	// roadmap
	Roadmap *roadmap.Roadmap // structured (Format == "" / "structured")
	Text    string           // rendered Markdown/HTML (Format == "md"/"html")
}
