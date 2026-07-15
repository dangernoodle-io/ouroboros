// Package query is the surface-neutral read core (OU-322): a Get/Search
// dispatch shared by every read caller. It deliberately does NOT import
// mcpx/mcpkit/go-sdk, so it is callable from both the MCP handlers
// (internal/app) and, eventually, a CLI query command (OU-324) without
// pulling the MCP transport along. It only imports the domain packages
// (store/backlog/roadmap/edges) that already held the read logic before
// this extraction.
package query

// Request is the normalized read request both Get and Search dispatch on —
// the union of every parameter the old get/search MCP tools accepted across
// all three domains (kb|backlog|roadmap). A caller's surface (MCP handler
// input, eventually CLI flags) builds one of these; unused fields for a
// given (Domain, mode) combination are simply left zero.
type Request struct {
	// Domain selects kb, backlog, or roadmap. Required; Get/Search return an
	// error (ErrDomainRequired) for "" or any other value.
	Domain string

	// IDs selects fetch-by-id mode for Get (kb: float64-valued entries,
	// backlog: string entries); non-empty takes precedence over the filter
	// fields below. Unused by Search.
	IDs []any

	// Verbose includes notes/edges sidecar on an IDs fetch (kb/backlog).
	Verbose bool

	// Query is a single full-text query: Get's kb-domain summary filter and
	// Search's single-query mode (kb/backlog/roadmap).
	Query string

	// Queries is Search's kb-only batched-queries mode, sharing the other
	// filter fields across each query; response order mirrors input order.
	Queries []string

	// Shared filters. Types/Categories/Tags are kb-only; PriorityMin/Max,
	// Status, EpicsOnly, Since, Sort are backlog-only; Component/Epic apply
	// to both backlog and roadmap; Projects/Limit apply broadly (roadmap
	// reads Projects[0] as its singleton key).
	Types       []string
	Projects    []string
	Categories  []string
	Tags        []string
	Limit       int
	PriorityMin string
	PriorityMax string
	Status      string
	Component   string
	Epic        string
	EpicsOnly   bool
	Since       string
	Sort        string

	// Roadmap-only rendering controls. By selects the grouping axis
	// (component default, or epic); Format selects structured (default),
	// md, or html.
	By     string
	Format string
}
