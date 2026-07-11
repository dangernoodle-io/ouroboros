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

// kbInput is the mcpkit-typed input for the kb write tool (OU-2): a batch of
// entries[], each either a create/natural-key-upsert (id absent) or an
// id-addressed partial update (id present).
type kbInput struct {
	// Entries is tagged omitempty for the same reason getInput.Domain is
	// (see its comment): old mcp-go behavior returned the SAME verbatim
	// "entries array is required (batch-only mode)" message for both an
	// omitted entries key and an empty entries[] array. If go-sdk
	// schema-validated entries as required, an omitted key would fail
	// schema validation before handleKBV2 ever runs, with a generic
	// schema-validation error instead of that message. The handler performs
	// its own explicit len(in.Entries)==0 check instead.
	Entries []kbEntryInput `json:"entries,omitempty" jsonschema:"Documents to create/update: {id?}, type, project, category?, title, content, notes?, tags?, metadata?. content max 500 chars; put narrative in notes"`
}

// kbEntryInput uses POINTER fields (except ID, see below) for presence: a
// nil pointer means the key was absent, matching parseKBEntryUpdate's
// presence semantics exactly (a title-only update leaves content/notes/tags
// untouched instead of clobbering them with zero values). On the create path
// (id absent) a nil pointer means "unset" -> "" / nil, matching
// parseKBEntryCreate's silent-zero-value behavior for an absent field.
//
// ID is deliberately `any`, NOT a typed pointer wrapper (e.g. a custom
// `*kbID` struct with its own UnmarshalJSON), despite id needing genuine
// dual-type decode (JSON number or numeric string). A throwaway probe
// (confirmed empirically, then deleted) proved a custom struct type here is
// a hard blocker: go-sdk's input-schema inference (google/jsonschema-go) reflects
// on the Go type structure independent of any custom json.Unmarshaler --
// a struct with only unexported fields (needed to store the decoded
// int64/error out-of-band) yields an empty "type":"object" schema, and
// go-sdk validates incoming arguments against that schema BEFORE the
// handler ever runs. That rejects every real id (a JSON number or string)
// with a generic schema-validation error, breaking the tool outright. `any`
// reflects as an unrestricted schema (no type constraint, matching old
// mcp-go's untyped id field), and the raw decoded value (float64/string/
// bool/nil/map/slice, exactly as the old untyped map-based parser saw it)
// is parsed by parseKBIDV2, below, reproducing parseKBEntryID verbatim
// (including its exact error text).
//
// ACCEPTED DIVERGENCE (stricter validation on every OTHER field): the old
// untyped parser silently coerced a present-but-wrong-typed field to its
// zero value (e.g. title:123 -> title "") since a failed type-assertion
// just fell through the `if v, ok := ...; ok` guard. go-sdk's JSON
// unmarshal instead REJECTS a wrong-typed Type/Project/Category/Title/
// Content/Notes/Tags/Metadata field with a whole-call decode error. This is
// a deliberate, accepted divergence (real Claude clients always send
// correctly-typed fields) -- not replicated.
//
// ACCEPTED DIVERGENCE (id:null is LOOSER, not stricter): encoding/json
// decodes an explicit `"id": null` into the `any` field as a Go nil,
// indistinguishable from an absent key -- both route to parseKBIDV2's
// present==false branch, i.e. create. The old untyped parser instead saw
// e["id"] present with a nil value, fell into parseKBEntryID's default
// branch, and returned a hard "invalid id: <nil>" error. Unlike the
// stricter-validation divergence above, this one is LOOSER: a caller that
// explicitly (if unusually) sends id:null now silently creates a new doc
// instead of erroring. It can't be cleanly closed with the `any`-field
// approach here (a custom decode type or json.RawMessage both break go-sdk
// schema inference the same way the deleted struct-based kbID probe did,
// see ID's comment above) -- tracked for the OU-5 cutover decision.
type kbEntryInput struct {
	ID       any                `json:"id,omitempty" jsonschema:"Document id (integer or numeric string) to update in place; omit to create/upsert by type+project+category+title"`
	Type     *string            `json:"type,omitempty"`
	Project  *string            `json:"project,omitempty"`
	Category *string            `json:"category,omitempty"`
	Title    *string            `json:"title,omitempty"`
	Content  *string            `json:"content,omitempty"`
	Notes    *string            `json:"notes,omitempty"`
	Tags     *[]string          `json:"tags,omitempty"`
	Metadata *map[string]string `json:"metadata,omitempty"`
}

// backlogInput is the mcpkit-typed input for the backlog write tool (OU-3):
// entries[] (mixed create/update batch) or delete_ids[] (batch delete).
// Neither is schema-required (see the comment on Entries below and
// getInput.Domain's comment for the same missing==empty verbatim-message
// parity reason); the handler enforces "one of" at runtime.
type backlogInput struct {
	// Entries is tagged omitempty for the same reason kbInput.Entries is:
	// old mcp-go behavior fell through to the SAME verbatim
	// "delete_ids or entries is required" message whether entries was
	// omitted or an empty array (both len(entries)==0). If go-sdk
	// schema-validated entries as required, an omitted key would fail
	// schema validation before handleBacklogV2 ever runs. The handler
	// performs its own explicit "neither present" check instead.
	Entries   []backlogEntryInput `json:"entries,omitempty" jsonschema:"Items to create/update: {id?}, project, priority, title, description?, notes?, component?, epic?, status?, edges?[{label,target}] (label: blocks|relates|explains, target: item id). description max 500 chars; put narrative in notes. epic may be \"$N\": the item created/updated by entries[N] earlier in this same write, so a child can reference its not-yet-created epic parent"`
	DeleteIDs []string            `json:"delete_ids,omitempty" jsonschema:"Item IDs to delete"`
}

// backlogEntryInput uses POINTER fields (except ID, Edges) for presence,
// mirroring kbEntryInput: a nil pointer means the key was absent. ID is a
// plain *string (not the kbEntryInput.ID `any` dual-type dance) since
// backlog ids are always prefixed strings (e.g. "B1-728") -- no numeric-id
// decode ambiguity to reproduce here.
//
// ACCEPTED DIVERGENCE (numeric/wrong-typed id): the old untyped parser did
// `e["id"].(string)` and treated a failed assertion (e.g. a JSON number, or
// omitted) the same as "no id" -> silent create. A typed *string field can't
// receive a JSON number at all -- go-sdk's decode rejects the whole call
// with a schema/decode error instead of silently creating. Not replicated
// (real callers always send a string id or omit the key).
//
// ACCEPTED DIVERGENCE (wrong-typed non-id fields): the old untyped parser
// silently treated a present-but-wrong-typed priority/title/description/
// notes/status as absent (failed type assertion -> zero value, e.g.
// `ok` false so the field is never added to `fields`/never satisfies the
// create gate). go-sdk instead REJECTS a wrong-typed field with a whole-call
// decode error. Not replicated (real callers always send correctly-typed
// fields).
//
// ACCEPTED DIVERGENCE (component/epic not-scalar message): the old
// scalarStringArg returned the verbatim
// "%s must be a single string value (single-valued, not a list)" error when
// component/epic was present but non-string (e.g. a list). With typed
// *string fields, a JSON list/object for component or epic is instead
// rejected by go-sdk's decode with a DIFFERENT (generic schema/decode)
// message before handleBacklogV2 ever runs. This diverges from the old
// verbatim message; RECOMMENDATION (per the OU-3 build spec) is to accept
// this stricter-but-different-message divergence rather than reintroduce an
// `any`+scalarStringArg-style shim purely to preserve wire-text parity for a
// misuse case. Flagged for reconsideration at cutover if exact verbatim
// parity on this specific error matters.
type backlogEntryInput struct {
	ID          *string     `json:"id,omitempty" jsonschema:"Item id to update in place; omit to create"`
	Project     *string     `json:"project,omitempty" jsonschema:"Project name (create only)"`
	Priority    *string     `json:"priority,omitempty" jsonschema:"P0-P6"`
	Title       *string     `json:"title,omitempty"`
	Description *string     `json:"description,omitempty" jsonschema:"Max 500 chars; put narrative in notes"`
	Notes       *string     `json:"notes,omitempty"`
	Component   *string     `json:"component,omitempty" jsonschema:"Single-valued component tag"`
	Epic        *string     `json:"epic,omitempty" jsonschema:"Epic backlog item id, single-valued; may be \"$N\", a back-reference to entries[N] earlier in this same write"`
	Status      *string     `json:"status,omitempty"`
	Edges       []edgeInput `json:"edges,omitempty" jsonschema:"Inline edges to create from this item: [{label,target}], label: blocks|relates|explains, target: item id"`
}

// edgeInput is the typed counterpart to parseEdgeSpecs' map-reading raw
// edges[] element ({label,target}). Label/Target are tagged omitempty for
// the same missing-key-parity reason as every other required-looking field
// in this file (see getInput.Domain's comment): if go-sdk schema-validated
// them as required, an edges[] element with the "label" or "target" key
// ABSENT (e.g. {"target":"AC-1"}) would be rejected before
// buildEdgeSpecsV2 ever runs, with a generic "required: missing
// properties" schema error instead of the old verbatim "edges[] entry
// requires label and target" message. omitempty decodes a missing key to
// "" instead, reaching buildEdgeSpecsV2's existing empty-label/target
// check, which fires that verbatim message exactly as before -- full
// parity restored, not a divergence.
//
// ACCEPTED DIVERGENCE (non-object element): the old parseEdgeSpecs treated
// a non-object edges[] element (e.g. a bare string or number) as a hard
// error ("invalid edges[] entry: expected object with label and target").
// A typed []edgeInput can't receive a non-object element at all -- go-sdk's
// decode rejects the whole call with a decode error instead. This is the
// only remaining edges[] divergence; the empty-label/target and
// invalid-label messages (produced by buildEdgeSpecsV2, which still runs
// after decode) are unaffected and reproduced exactly.
type edgeInput struct {
	Label  string `json:"label,omitempty"`
	Target string `json:"target,omitempty"`
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
