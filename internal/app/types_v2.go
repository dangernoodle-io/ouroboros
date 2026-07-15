package app

// queryInputV2 is the mcpkit-typed union of every parameter across the
// former get and search tools (collapsed into the single query tool at
// OU-323), spanning all three domains (kb|backlog|roadmap). Domain is
// logically required but deliberately NOT schema-required (see its field
// comment). ids[] selects an exact fetch; query/queries[] selects full-text
// search mode; otherwise a filtered list. The jsonschema tag text reuses the
// same desc* constants capabilities_v2.go's tool definition draws from.
type queryInputV2 struct {
	// Domain is tagged omitempty so go-sdk does NOT mark it schema-required:
	// old mcp-go behavior returned the SAME verbatim "domain is required: …"
	// message for BOTH an omitted domain key and domain="" (both fell
	// through the same switch default). If go-sdk schema-validated domain as
	// required, an omitted key would be rejected before the handler ever
	// runs, with a generic schema-validation error instead of that message —
	// breaking wire parity for the single most common caller mistake. The
	// handler performs its own explicit missing/empty check instead.
	Domain      string   `json:"domain,omitempty" jsonschema:"Required: \"kb\", \"backlog\", or \"roadmap\""`
	IDs         []any    `json:"ids,omitempty" jsonschema:"IDs to fetch (kb: document IDs, backlog: item IDs); exact match, skips search/filter mode"`
	Verbose     bool     `json:"verbose,omitempty" jsonschema:"Include notes (ids[] fetch only), default false"`
	Query       string   `json:"query,omitempty" jsonschema:"Full-text search; present -> search mode (kb/backlog/roadmap)"`
	Queries     []string `json:"queries,omitempty" jsonschema:"Batch full-text search sharing filters, kb only; response is positional [[...], [...]]"`
	Types       []string `json:"types,omitempty" jsonschema:"Filter by types (kb only)"`
	Projects    []string `json:"projects,omitempty" jsonschema:"Filter by project names"`
	Categories  []string `json:"categories,omitempty" jsonschema:"Filter by categories (kb only)"`
	Tags        []string `json:"tags,omitempty" jsonschema:"Filter by tags, all match (kb only; combines with query)"`
	Limit       int      `json:"limit,omitempty" jsonschema:"Limit, default 10, max 500"`
	PriorityMin string   `json:"priority_min,omitempty" jsonschema:"Min priority P0-P6 (backlog only)"`
	PriorityMax string   `json:"priority_max,omitempty" jsonschema:"Max priority P0-P6 (backlog only)"`
	Status      string   `json:"status,omitempty" jsonschema:"open or done (backlog only)"`
	Component   string   `json:"component,omitempty" jsonschema:"Component tag filter (backlog subproject/plugin; roadmap grouping axis)"`
	Epic        string   `json:"epic,omitempty" jsonschema:"Epic item id filter — its children (backlog/roadmap)"`
	EpicsOnly   bool     `json:"epics_only,omitempty" jsonschema:"List only epics (backlog only); overrides epic"`
	Since       string   `json:"since,omitempty" jsonschema:"Created-at cutoff (backlog only): duration (24h, 7d), date (2006-01-02), or RFC3339"`
	Sort        string   `json:"sort,omitempty" jsonschema:"\"created\" sorts backlog items newest-first (backlog only)"`
	By          string   `json:"by,omitempty" jsonschema:"Grouping axis: component (default) or epic (roadmap md/html only); other axis shown inline"`
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
	Entries []kbEntryInput `json:"entries,omitempty" jsonschema:"KB docs to create (id absent) or update in place (id present). content max 500 chars; put narrative in notes"`
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
	ID          any                `json:"id,omitempty" jsonschema:"Document id (integer or numeric string) to update in place; omit to create/upsert by type+project+category+title"`
	Type        *string            `json:"type,omitempty"`
	Project     *string            `json:"project,omitempty"`
	Category    *string            `json:"category,omitempty"`
	Title       *string            `json:"title,omitempty"`
	Content     *string            `json:"content,omitempty"`
	Notes       *string            `json:"notes,omitempty"`
	AppendNotes bool               `json:"append_notes,omitempty" jsonschema:"Append notes to the existing value instead of replacing (update-only; ignored on create)"`
	Tags        *[]string          `json:"tags,omitempty"`
	Metadata    *map[string]string `json:"metadata,omitempty"`
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
	Entries   []backlogEntryInput `json:"entries,omitempty" jsonschema:"Backlog items to create (id absent) or update in place (id present)."`
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
	AppendNotes bool        `json:"append_notes,omitempty" jsonschema:"Append notes to the existing value instead of replacing (update-only; ignored on create)"`
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

// roadmapInput is the mcpkit-typed input for the roadmap write tool (OU-4):
// a single flat arg set (NOT entries[]) dispatched by Op across
// add|update|move|reorder|done|remove. Every field the old handler
// validates with a verbatim message (Op, Project, Section, To, per-op
// ID-presence, BlockedBy[].Project/Ref) is tagged omitempty -- NOT
// schema-required -- for the same missing-key-parity reason as every
// other required-looking field in this file (see getInput.Domain's
// comment): schema-requiring them would let go-sdk reject a missing key
// before handleRoadmapV2 ever runs, with a generic schema-validation error
// instead of the old verbatim text. handleRoadmapV2 and its per-op helpers
// reproduce every check itself.
//
// ID/Position are typed pointers (not the kbEntryInput.ID `any` dual-type
// dance): roadmap has no number-or-string dual-type field to reproduce
// here (the old intArg only ever read a JSON number), so plain pointers
// give presence (nil = absent) while staying compatible with go-sdk's
// schema inference -- unlike a custom decode type, which broke inference
// for kbEntryInput.ID (see its comment).
//
// The pointer fields (Title..Position) serve BOTH add and update: add
// reads them as "value or absent -> zero" (deref-or-default, matching the
// old handler's strArg/parseIntSlice/parseStringSlice/parseBlockers
// absent-is-zero behavior); update carries their presence straight into
// roadmap.Patch (nil = untouched field, non-nil (even an empty slice/"")
// = clears -- matches the old handler's stringPtrArg/intSlicePtrArg/
// stringSlicePtrArg/blockedByPtrArg present-but-empty-clears semantics
// exactly). One struct serves both operations; handleRoadmapV2 branches on
// Op.
//
// ACCEPTED DIVERGENCE (explicit JSON null on update, slice fields only):
// on the update op, an explicit `"kb": null` / `"ticket": null` /
// `"blocked_by": null` decodes the pointer-to-slice field (KB/Ticket/
// BlockedBy) to a nil pointer -- indistinguishable from the key being
// absent -- so handleRoadmapUpdateV2 leaves the field UNTOUCHED. The old
// handler instead CLEARED it: its map-key-presence check (`if _, ok :=
// args[key]; !ok`) passes for a present null value, so intSlicePtrArg/
// stringSlicePtrArg/blockedByPtrArg went on to parse it (a non-object/
// non-array null parses to an empty slice) and returned a non-nil pointer
// to that empty slice, clearing the field. This is the flip side of the
// present-empty-clears rule above: an empty ARRAY `[]` still clears
// identically in both old and new (present -> non-nil pointer to an empty
// slice); only the null-to-clear idiom differs. To clear a slice field
// here, send `[]`, not `null`. Can't be fixed with typed pointers alone --
// the same schema-inference constraint kbEntryInput.ID's comment
// describes for id:null at OU-2: distinguishing "key present with value
// null" from "key absent" needs a raw-args fallback, which a plain
// `*[]T` field can't provide. Tracked for the OU-5 cutover decision.
type roadmapInput struct {
	Op            string          `json:"op,omitempty" jsonschema:"Required: \"add\", \"update\", \"move\", \"reorder\", \"done\", or \"remove\""`
	Project       string          `json:"project,omitempty" jsonschema:"Project name (roadmap singleton)"`
	Section       string          `json:"section,omitempty" jsonschema:"now|next|deferred|parked|dropped|done (add only)"`
	To            string          `json:"to,omitempty" jsonschema:"now|next|deferred|parked|dropped|done (move only)"`
	ID            *int            `json:"id,omitempty" jsonschema:"Item ID (update/move/reorder/done/remove)"`
	Title         *string         `json:"title,omitempty" jsonschema:"Item title (add/update)"`
	Body          *string         `json:"body,omitempty" jsonschema:"Item body (add/update)"`
	Component     *string         `json:"component,omitempty" jsonschema:"Component tag, single-valued (add/update)"`
	Why           *string         `json:"why,omitempty" jsonschema:"Why parked (add/update)"`
	ResumeTrigger *string         `json:"resume_trigger,omitempty" jsonschema:"Resume trigger (add/update)"`
	KB            *[]int          `json:"kb,omitempty" jsonschema:"Related KB doc IDs (add/update)"`
	Ticket        *[]string       `json:"ticket,omitempty" jsonschema:"Ticket refs (add/update)"`
	BlockedBy     *[]blockerInput `json:"blocked_by,omitempty" jsonschema:"Cross-project blockers: {project,ref,note} (add/update)"`
	Epic          *string         `json:"epic,omitempty" jsonschema:"Epic backlog item id, single-valued (add/update)"`
	Position      *float64        `json:"position,omitempty" jsonschema:"Sort position within the section (add/move/reorder; required for reorder)"`
}

// blockerInput is the typed counterpart to parseBlockers' raw blocked_by[]
// element ({project,ref,note}). Project/Ref are tagged omitempty for the
// same missing-key-parity reason as every other required-looking field in
// this file: an absent key decodes to "" instead of failing go-sdk schema
// validation, reaching blockersFromInputV2's existing empty-project/ref
// check, which fires the old parseBlockers verbatim message exactly
// ("blocked_by[%d] missing project"/"missing ref") -- full parity
// restored, not a divergence.
//
// ACCEPTED DIVERGENCE (non-object element): the old parseBlockers treated
// a non-object blocked_by[] element (e.g. a bare string) as a hard error
// ("blocked_by[%d] must be an object with project and ref"). A typed
// []blockerInput can't receive a non-object element at all -- go-sdk's
// decode rejects the whole call with a decode error instead. This mirrors
// edgeInput's identical accepted divergence (see its comment); the
// missing-project/missing-ref messages (produced by blockersFromInputV2,
// which still runs after decode) are unaffected and reproduced exactly.
type blockerInput struct {
	Project string `json:"project,omitempty" jsonschema:"Blocking project name"`
	Ref     string `json:"ref,omitempty" jsonschema:"Blocking item/ticket ref"`
	Note    string `json:"note,omitempty" jsonschema:"Optional note"`
}
