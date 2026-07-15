package query

import (
	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/edges"
	"dangernoodle.io/ouroboros/internal/roadmap"
	"dangernoodle.io/ouroboros/internal/store"
)

// DocWithEdges wraps a KB document with its edges sidecar, populated only on
// a verbose=true IDs fetch (see getDocuments). Moved here verbatim from
// internal/app's former docWithEdges (OU-322) — the core, not the MCP
// handler, now produces the edges-augmented shape.
type DocWithEdges struct {
	*store.Document
	Edges []edges.Edge `json:"edges,omitempty"`
}

// ItemWithEdges wraps a backlog item with its edges sidecar, populated only
// on a verbose=true IDs fetch (see getBacklogItems). Moved here verbatim
// from internal/app's former itemWithEdges (OU-322).
type ItemWithEdges struct {
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
	Docs           []any                     // IDs fetch: each *store.Document or DocWithEdges
	DocSummaries   []store.DocumentSummary   // filter-list (Get) or single-query (Search)
	DocSummarySets [][]store.DocumentSummary // Search's queries[] batch (kb only)

	// backlog
	ItemsJSON []any          // IDs fetch: each *backlog.Item or ItemWithEdges
	Items     []backlog.Item // filter-list (Get) or query match (Search); caller renders

	// roadmap
	Roadmap *roadmap.Roadmap // structured (Format == "" / "structured")
	Text    string           // rendered Markdown/HTML (Format == "md"/"html")
}
