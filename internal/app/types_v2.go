package app

// getInput is the mcpkit-typed union of every get tool parameter across all
// three domains (kb|backlog|roadmap). Domain is logically required but
// deliberately NOT schema-required (see its field comment); the jsonschema
// tag text on every field reuses the same desc* constants server.go's
// mcp-go tool definition drew from, so tool-description parity holds across
// the mcp-go/mcpkit seam.
type getInput struct {
	// Domain is tagged omitempty so go-sdk does NOT mark it schema-required:
	// old mcp-go behavior returned the SAME verbatim "domain is required: …"
	// message for BOTH an omitted domain key and domain="" (both fell
	// through the same switch default). If go-sdk schema-validated domain as
	// required, an omitted key would be rejected before the handler ever
	// runs, with a generic schema-validation error instead of that message —
	// breaking wire parity for the single most common caller mistake. The
	// handler performs its own explicit missing/empty check instead.
	Domain      string   `json:"domain,omitempty" jsonschema:"Required: \"kb\", \"backlog\", or \"roadmap\""`
	IDs         []any    `json:"ids,omitempty" jsonschema:"IDs to fetch (kb: document IDs, backlog: item IDs)"`
	Verbose     bool     `json:"verbose,omitempty" jsonschema:"Include notes on ids[] fetch only, default false; filter/list results are always compact regardless"`
	Types       []string `json:"types,omitempty" jsonschema:"Filter by types (kb only)"`
	Projects    []string `json:"projects,omitempty" jsonschema:"Filter by project names"`
	Categories  []string `json:"categories,omitempty" jsonschema:"Filter by categories (kb only)"`
	Query       string   `json:"query,omitempty" jsonschema:"Full-text search (kb only)"`
	Tags        []string `json:"tags,omitempty" jsonschema:"Filter by tags, all match (kb only)"`
	Limit       int      `json:"limit,omitempty" jsonschema:"Limit, default 10, max 500"`
	PriorityMin string   `json:"priority_min,omitempty" jsonschema:"Min priority P0-P6 (backlog only)"`
	PriorityMax string   `json:"priority_max,omitempty" jsonschema:"Max priority P0-P6 (backlog only)"`
	Status      string   `json:"status,omitempty" jsonschema:"open or done (backlog only)"`
	Component   string   `json:"component,omitempty" jsonschema:"Component tag filter (backlog: subproject/plugin; roadmap: structural grouping axis, single-valued)"`
	Epic        string   `json:"epic,omitempty" jsonschema:"Epic backlog item id filter — that epic's children (backlog, roadmap; single-valued — an epic IS a backlog item, see the EPIC: convention)"`
	EpicsOnly   bool     `json:"epics_only,omitempty" jsonschema:"List only epic items, EPIC:-titled (backlog only); takes precedence over epic"`
	Since       string   `json:"since,omitempty" jsonschema:"Created-time window for backlog items: a duration (24h, 7d), a date (2006-01-02), or an RFC3339 timestamp (backlog only)"`
	Sort        string   `json:"sort,omitempty" jsonschema:"\"created\" sorts backlog items newest-first (backlog only)"`
	By          string   `json:"by,omitempty" jsonschema:"Grouping axis: \"component\" (default) or \"epic\"; the other axis renders as an inline chip (roadmap get format=md|html only)"`
	Format      string   `json:"format,omitempty" jsonschema:"structured, md, or html, default structured (roadmap only)"`
}

// searchInput is the mcpkit-typed union of every search tool parameter
// across all three domains. Differs from getInput: no ids/verbose/tags,
// adds Queries (kb batch mode), no by/format (search has no roadmap
// rendering mode).
type searchInput struct {
	// Domain: see getInput.Domain's comment — deliberately not schema-required,
	// for the same missing==empty verbatim-message parity reason.
	Domain      string   `json:"domain,omitempty" jsonschema:"Required: \"kb\", \"backlog\", or \"roadmap\""`
	Query       string   `json:"query,omitempty" jsonschema:"Single query"`
	Queries     []string `json:"queries,omitempty" jsonschema:"Batch queries sharing filters, kb only; response is positional [[...], [...]]"`
	Types       []string `json:"types,omitempty" jsonschema:"Filter by types (kb only)"`
	Projects    []string `json:"projects,omitempty" jsonschema:"Filter by project names"`
	Categories  []string `json:"categories,omitempty" jsonschema:"Filter by categories (kb only)"`
	PriorityMin string   `json:"priority_min,omitempty" jsonschema:"Min priority P0-P6 (backlog only)"`
	PriorityMax string   `json:"priority_max,omitempty" jsonschema:"Max priority P0-P6 (backlog only)"`
	Status      string   `json:"status,omitempty" jsonschema:"open or done (backlog only)"`
	Component   string   `json:"component,omitempty" jsonschema:"Component tag filter (backlog: subproject/plugin; roadmap: structural grouping axis, single-valued)"`
	Epic        string   `json:"epic,omitempty" jsonschema:"Epic backlog item id filter — that epic's children (backlog, roadmap; single-valued — an epic IS a backlog item, see the EPIC: convention)"`
	EpicsOnly   bool     `json:"epics_only,omitempty" jsonschema:"List only epic items, EPIC:-titled (backlog only); takes precedence over epic"`
	Since       string   `json:"since,omitempty" jsonschema:"Created-time window for backlog items: a duration (24h, 7d), a date (2006-01-02), or an RFC3339 timestamp (backlog only)"`
	Sort        string   `json:"sort,omitempty" jsonschema:"\"created\" sorts backlog items newest-first (backlog only)"`
	Limit       int      `json:"limit,omitempty" jsonschema:"Limit, default 10, max 500"`
}
