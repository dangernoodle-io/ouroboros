// Package roadmap implements the per-project roadmap doc: a singleton
// documents row (type=roadmap, category="", title="roadmap") whose
// canonical structure lives in metadata JSON and whose content is a
// regenerated Markdown mirror.
package roadmap

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf8"

	"dangernoodle.io/ouroboros/internal/store"
)

// Section names a roadmap lane.
type Section string

// Roadmap lanes, in render order.
const (
	SectionNow      Section = "now"
	SectionNext     Section = "next"
	SectionDeferred Section = "deferred"
	SectionParked   Section = "parked"
	SectionDropped  Section = "dropped"
	SectionDone     Section = "done"
)

const (
	docType     = "roadmap"
	docCategory = ""
	docTitle    = "roadmap"
	metadataKey = "data"

	// schemaVersion is stamped onto every save; a v1 doc (no schema_version
	// field) has no lanes/positions/deferred/dropped and still round-trips.
	schemaVersion = 2
)

// OU-221: the roadmap's canonical structure lives in the metadata JSON
// column (buildDoc), which -- unlike content (store.MaxDocContentBytes) and
// notes (store.MaxDocNotesBytes) -- is NOT independently truncatable: it's
// the only copy of the data, so truncating it to fit a byte cap would lose
// items outright. Left unbounded, a project that never archives its "done"
// items eventually produces a metadata blob large enough to threaten this
// store's practical size ceiling. Rather than truncate (data loss) or cap
// metadata bytes directly (a write could fail mid-item with no clean
// recovery), we cap the ITEM COUNT, which is cheap to check before mutating
// and gives a precise, actionable failure.
//
// Derivation: roadmapItemWorstCaseBytes is a generous per-item JSON size
// estimate -- a fully-populated Item (long title/body/why/resume_trigger,
// 5 kb refs, 3 tickets, 2 blockers, full timestamps) marshals to ~970 bytes
// (measured); rounded up to 1024 for margin. roadmapItemBudgetBytes is a
// deliberately generous but finite ceiling for the whole metadata blob --
// far below SQLite's own TEXT column limit (~1GB) but far above anything a
// healthy, regularly-pruned roadmap ever approaches, keeping this store's
// documents in a sane size class without constraining normal use, and set
// to divide evenly by roadmapItemWorstCaseBytes so maxRoadmapItems (500)
// needs no further rounding; the JSON envelope (schema_version/next_id/
// artifact_url/section-key/bracket bytes -- a few hundred bytes at most)
// and real items running a bit larger than the worst-case estimate are the
// headroom this leaves.
const (
	roadmapItemWorstCaseBytes = 1024
	roadmapItemBudgetBytes    = 500 * 1024

	// maxRoadmapItems caps the TOTAL item count across every section (now/
	// next/deferred/parked/dropped/done -- done counts too: archiving done
	// items via op=remove is exactly the release valve this cap exists to
	// force). Enforced only on operations that grow the total count
	// (AddItem, and transitively Seed, which adds through it); update/
	// reorder/remove/done (a move, not a growth) are unaffected.
	maxRoadmapItems = roadmapItemBudgetBytes / roadmapItemWorstCaseBytes

	// errRoadmapItemCap is the actionable error AddItem returns once
	// totalItemCount(rm) has reached maxRoadmapItems.
	errRoadmapItemCap = "roadmap is at the %d-item cap (%d items); archive or remove done items (op=remove) before adding more"
)

// ValidSection reports whether s is a known roadmap section.
func ValidSection(s Section) bool {
	switch s {
	case SectionNow, SectionNext, SectionDeferred, SectionParked, SectionDropped, SectionDone:
		return true
	default:
		return false
	}
}

// Blocker is a cross-project dependency blocking a roadmap item.
type Blocker struct {
	Project string `json:"project"`
	Ref     string `json:"ref"`
	Note    string `json:"note,omitempty"`
}

// Item is a single roadmap entry.
type Item struct {
	ID            int       `json:"id"`
	Title         string    `json:"title"`
	Body          string    `json:"body"`
	Component     string    `json:"component,omitempty"`
	Why           string    `json:"why,omitempty"`
	ResumeTrigger string    `json:"resume_trigger,omitempty"`
	KB            []int     `json:"kb,omitempty"`
	Ticket        []string  `json:"ticket,omitempty"`
	BlockedBy     []Blocker `json:"blocked_by,omitempty"`
	// Epic holds the id of the epic backlog item this item belongs to, if
	// any (an epic IS a backlog item — see the EPIC: convention). Epic and
	// Component are the two grouping axes; render picks one via `by` and
	// shows the other as an inline chip. Both are single-valued (scalar) —
	// no item belongs to more than one component or epic.
	Epic string `json:"epic,omitempty"`
	// Position is the item's 1-based sort slot within its section. A
	// position write (ReorderItem, or a position-bearing MoveItem)
	// physically splices the item to that 0-based target index and then
	// densely renumbers every item in the section to 1..N (idx+1) — so
	// slice order and Position order always agree exactly and there's
	// never a tie to break. 0 means unset (legacy) — a section nothing
	// has repositioned yet stays all-0 and renders in add/slice order.
	Position  int    `json:"position,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Sections holds the roadmap's six lanes.
type Sections struct {
	Now      []Item `json:"now"`
	Next     []Item `json:"next"`
	Deferred []Item `json:"deferred"`
	Parked   []Item `json:"parked"`
	Dropped  []Item `json:"dropped"`
	Done     []Item `json:"done"`
}

// Roadmap is the canonical per-project roadmap structure, persisted as
// metadata JSON on the singleton documents row.
type Roadmap struct {
	Sections    Sections `json:"sections"`
	ArtifactURL string   `json:"artifact_url,omitempty"`
	NextID      int      `json:"next_id"`
	// SchemaVersion is 0/absent on a v1 doc (pre-lanes/positions/deferred/
	// dropped); Save always stamps the current version.
	SchemaVersion int `json:"schema_version,omitempty"`
}

// Patch carries optional field updates for UpdateItem; nil fields are left
// unchanged.
type Patch struct {
	Title         *string
	Body          *string
	Component     *string
	Why           *string
	ResumeTrigger *string
	KB            *[]int
	Ticket        *[]string
	BlockedBy     *[]Blocker
	Epic          *string
}

func nowStamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// New returns an empty roadmap with every section slice initialized to a
// non-nil empty slice, so JSON marshaling yields [] rather than null for a
// brand-new project.
func New() *Roadmap {
	rm := &Roadmap{}
	ensureInitialized(rm)
	return rm
}

// ensureInitialized replaces any nil section slice with a non-nil empty one.
func ensureInitialized(rm *Roadmap) {
	if rm.Sections.Now == nil {
		rm.Sections.Now = []Item{}
	}
	if rm.Sections.Next == nil {
		rm.Sections.Next = []Item{}
	}
	if rm.Sections.Deferred == nil {
		rm.Sections.Deferred = []Item{}
	}
	if rm.Sections.Parked == nil {
		rm.Sections.Parked = []Item{}
	}
	if rm.Sections.Dropped == nil {
		rm.Sections.Dropped = []Item{}
	}
	if rm.Sections.Done == nil {
		rm.Sections.Done = []Item{}
	}
}

// Load reads the singleton roadmap doc for project. If none exists yet,
// returns an empty, initialized Roadmap (not an error).
func Load(db *sql.DB, project string) (*Roadmap, error) {
	doc, err := store.GetDocumentByKey(db, docType, project, docCategory, docTitle)
	if err != nil {
		return nil, fmt.Errorf("load roadmap: %w", err)
	}
	return docToRoadmap(doc)
}

// loadTx is Load scoped to an existing transaction, for use inside Mutate.
func loadTx(tx *sql.Tx, project string) (*Roadmap, error) {
	doc, err := store.GetDocumentByKeyTx(tx, docType, project, docCategory, docTitle)
	if err != nil {
		return nil, fmt.Errorf("load roadmap: %w", err)
	}
	return docToRoadmap(doc)
}

// docToRoadmap parses the roadmap metadata off doc, or returns a fresh
// initialized Roadmap if doc is nil or carries no metadata.
func docToRoadmap(doc *store.Document) (*Roadmap, error) {
	if doc == nil {
		return New(), nil
	}

	raw, ok := doc.Metadata[metadataKey]
	if !ok || raw == "" {
		return New(), nil
	}

	var rm Roadmap
	if err := json.Unmarshal([]byte(raw), &rm); err != nil {
		return nil, fmt.Errorf("parse roadmap metadata: %w", err)
	}
	ensureInitialized(&rm)
	return &rm, nil
}

// Save serializes rm to the singleton roadmap doc for project: metadata
// carries the canonical structure, content carries the Markdown mirror.
func Save(db *sql.DB, project string, rm *Roadmap) error {
	doc, err := buildDoc(project, rm)
	if err != nil {
		return err
	}

	if _, err := store.UpsertDocument(db, *doc); err != nil {
		return fmt.Errorf("save roadmap: %w", err)
	}
	return nil
}

// saveTx is Save scoped to an existing transaction, for use inside Mutate.
// Callers are responsible for the subsequent FTS rebuild.
func saveTx(tx *sql.Tx, project string, rm *Roadmap) error {
	doc, err := buildDoc(project, rm)
	if err != nil {
		return err
	}

	if _, err := store.UpsertDocumentTx(tx, *doc); err != nil {
		return fmt.Errorf("save roadmap: %w", err)
	}
	return nil
}

// buildDoc renders rm into the store.Document Save/saveTx persist. Every
// save stamps SchemaVersion to the current version, so a v1 doc (loaded
// with SchemaVersion 0) is upgraded on its next save without losing data —
// v1's now/next/parked/done and their items are untouched additive fields.
func buildDoc(project string, rm *Roadmap) (*store.Document, error) {
	rm.SchemaVersion = schemaVersion
	canonicalizeSections(rm)
	dataJSON, err := json.Marshal(rm)
	if err != nil {
		return nil, fmt.Errorf("marshal roadmap: %w", err)
	}

	return &store.Document{
		Type:     docType,
		Project:  project,
		Category: docCategory,
		Title:    docTitle,
		Content:  renderContent(rm),
		Metadata: map[string]string{metadataKey: string(dataJSON)},
	}, nil
}

// renderContent renders rm as Markdown for the content column, truncating
// to fit under store.MaxDocContentBytes if necessary so a save never fails
// on an oversized roadmap. The canonical structure always lives in the
// metadata JSON column (marshaled untruncated in buildDoc); content is only
// a rendered preview mirror, so truncating it here loses no data. The
// preview always groups by the default axis (component) with no epic
// labels/blocked-status resolved — it's a storage-side mirror, not a
// caller-driven render.
func renderContent(rm *Roadmap) string {
	md := RenderMarkdown(rm, "component", nil, nil)
	if len(md) <= store.MaxDocContentBytes {
		return md
	}

	marker := fmt.Sprintf("\n... (truncated; %d items — see structured data)\n", totalItemCount(rm))

	maxBody := store.MaxDocContentBytes - len(marker)
	if maxBody < 0 {
		maxBody = 0
	}
	return truncateUTF8(md, maxBody) + marker
}

// canonicalizeSections stable-sorts every section slice in rm by Position
// ascending (legacy/unset Position 0 keeps add/ID order on ties), so the
// stored metadata JSON and the structured get domain=roadmap output always
// agree. Axis grouping (component/epic) is a render-time concern (see
// groupByAxis in render.go) — storage order carries no axis bucketing,
// since a section can be rendered grouped by either axis. Called on every
// Save/saveTx before marshaling. Idempotent: canonicalizing an
// already-ordered roadmap is a no-op (same bytes on re-save).
func canonicalizeSections(rm *Roadmap) {
	rm.Sections.Now = sortByPosition(rm.Sections.Now)
	rm.Sections.Next = sortByPosition(rm.Sections.Next)
	rm.Sections.Deferred = sortByPosition(rm.Sections.Deferred)
	rm.Sections.Parked = sortByPosition(rm.Sections.Parked)
	rm.Sections.Dropped = sortByPosition(rm.Sections.Dropped)
	rm.Sections.Done = sortByPosition(rm.Sections.Done)
}

// truncateUTF8 truncates s to at most maxBytes bytes, backing off as needed
// to avoid splitting a multi-byte UTF-8 rune.
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	b := s[:maxBytes]
	for len(b) > 0 && !utf8.ValidString(b) {
		b = b[:len(b)-1]
	}
	return b
}

// Mutate serializes a full load->mutate->save cycle for project's roadmap in
// a single database transaction: it loads the roadmap, runs fn against it,
// and saves the result, all inside one BEGIN IMMEDIATE transaction (the
// store package's connection DSN sets _txlock=immediate — see
// store.InitDB). BEGIN IMMEDIATE acquires SQLite's write lock at the start
// of the transaction rather than deferring it to the first write, so a
// concurrent writer (another goroutine in this process, or another
// ouroboros process against the same file) cannot interleave between this
// call's load and save and lose an update or duplicate an item ID; a
// second concurrent Mutate simply blocks (up to the busy_timeout pragma)
// until this one commits.
//
// Any error from fn, or from the load/save steps, rolls back the
// transaction and is returned unwrapped (fn's error) or wrapped (load/save
// errors). The FTS index is rebuilt only after a successful commit.
func Mutate(db *sql.DB, project string, fn func(*Roadmap) error) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("mutate roadmap: begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	rm, err := loadTx(tx, project)
	if err != nil {
		return fmt.Errorf("mutate roadmap: %w", err)
	}

	if err := fn(rm); err != nil {
		return err
	}

	if err := saveTx(tx, project, rm); err != nil {
		return fmt.Errorf("mutate roadmap: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("mutate roadmap: commit: %w", err)
	}
	committed = true

	if err := store.RebuildFTS(db); err != nil {
		return fmt.Errorf("mutate roadmap: rebuild fts: %w", err)
	}
	return nil
}

// sectionSlice returns a pointer to the slice backing section, or nil for
// an unknown section.
func sectionSlice(rm *Roadmap, section Section) *[]Item {
	switch section {
	case SectionNow:
		return &rm.Sections.Now
	case SectionNext:
		return &rm.Sections.Next
	case SectionDeferred:
		return &rm.Sections.Deferred
	case SectionParked:
		return &rm.Sections.Parked
	case SectionDropped:
		return &rm.Sections.Dropped
	case SectionDone:
		return &rm.Sections.Done
	default:
		return nil
	}
}

// totalItemCount sums item counts across every section (see OU-221's
// maxRoadmapItems comment above).
func totalItemCount(rm *Roadmap) int {
	return len(rm.Sections.Now) + len(rm.Sections.Next) + len(rm.Sections.Deferred) +
		len(rm.Sections.Parked) + len(rm.Sections.Dropped) + len(rm.Sections.Done)
}

// allSections lists every roadmap section, in render order.
var allSections = []Section{SectionNow, SectionNext, SectionDeferred, SectionParked, SectionDropped, SectionDone}

// findItem locates id across all sections.
func findItem(rm *Roadmap, id int) (Section, int, bool) {
	for _, s := range allSections {
		items := *sectionSlice(rm, s)
		for i, it := range items {
			if it.ID == id {
				return s, i, true
			}
		}
	}
	return "", 0, false
}

// AddItem assigns item the next sequential ID, stamps timestamps, and adds
// it to section. Returns the assigned ID.
//
// position is optional and mirrors MoveItem/ReorderItem's variadic — its
// PRESENCE, not item.Position, signals intent (item.Position is never read
// as an ordering input; it's reset to 0 before insertion). When position is
// given, AddItem splices the new item to that 0-based target index within
// section and densely renumbers the whole section (see insertAtIndex) —
// index 0 lands the item first even among untouched (Position-0) siblings.
// When omitted, AddItem plain-appends and calls renumberIfDense, which
// keeps a DENSE section's item order correct (the new item sorts last)
// while leaving a LEGACY (all-0) section untouched.
func AddItem(rm *Roadmap, section Section, item Item, position ...int) (int, error) {
	if !ValidSection(section) {
		return 0, fmt.Errorf("invalid section %q", section)
	}

	if n := totalItemCount(rm); n >= maxRoadmapItems {
		return 0, fmt.Errorf(errRoadmapItemCap, maxRoadmapItems, n)
	}

	rm.NextID++
	item.ID = rm.NextID
	now := nowStamp()
	item.CreatedAt = now
	item.UpdatedAt = now
	item.Position = 0

	items := sectionSlice(rm, section)
	if len(position) > 0 {
		*items = insertAtIndex(*items, item, position[0])
	} else {
		*items = append(*items, item)
		renumberIfDense(*items)
	}
	return item.ID, nil
}

// UpdateItem applies patch's non-nil fields to the item with id.
func UpdateItem(rm *Roadmap, id int, patch Patch) error {
	section, idx, ok := findItem(rm, id)
	if !ok {
		return fmt.Errorf("item %d not found", id)
	}

	items := sectionSlice(rm, section)
	it := &(*items)[idx]

	if patch.Title != nil {
		it.Title = *patch.Title
	}
	if patch.Body != nil {
		it.Body = *patch.Body
	}
	if patch.Component != nil {
		it.Component = *patch.Component
	}
	if patch.Why != nil {
		it.Why = *patch.Why
	}
	if patch.ResumeTrigger != nil {
		it.ResumeTrigger = *patch.ResumeTrigger
	}
	if patch.KB != nil {
		it.KB = *patch.KB
	}
	if patch.Ticket != nil {
		it.Ticket = *patch.Ticket
	}
	if patch.BlockedBy != nil {
		it.BlockedBy = *patch.BlockedBy
	}
	if patch.Epic != nil {
		it.Epic = *patch.Epic
	}
	it.UpdatedAt = nowStamp()
	return nil
}

// insertAtIndex splices item into items at the given 0-based target index
// (clamped to [0, len(items)]), then densely renumbers every item's
// Position to its 1-based slot (i+1). The splice fixes the physical order;
// the renumber makes that order durable and unambiguous under
// canonicalizeSections' Position sort — no two items in the section can
// tie afterward, so index 0 always sorts first.
func insertAtIndex(items []Item, item Item, index int) []Item {
	if index < 0 {
		index = 0
	}
	if index > len(items) {
		index = len(items)
	}

	out := make([]Item, 0, len(items)+1)
	out = append(out, items[:index]...)
	out = append(out, item)
	out = append(out, items[index:]...)

	for i := range out {
		out[i].Position = i + 1
	}
	return out
}

// removeAt returns a copy of items with the element at idx removed,
// without aliasing items' backing array (safe to keep using items after).
func removeAt(items []Item, idx int) []Item {
	out := make([]Item, 0, len(items)-1)
	out = append(out, items[:idx]...)
	out = append(out, items[idx+1:]...)
	return out
}

// renumberIfDense enforces the section invariant after a membership or
// order change: a section is either LEGACY (every item's Position == 0,
// sorting by slice/add order) or DENSE (Position is exactly 1..N, mirroring
// slice order) — never a mix. If any item in items already carries a
// nonzero Position (the section is DENSE), every item is renumbered to its
// 1-based slot (i+1) by current slice order, so an appended item lands last
// and a removed item leaves no gap. A LEGACY section (all-0) is left
// untouched — renumbering it would needlessly promote it to DENSE.
func renumberIfDense(items []Item) {
	dense := false
	for _, it := range items {
		if it.Position != 0 {
			dense = true
			break
		}
	}
	if !dense {
		return
	}
	for i := range items {
		items[i].Position = i + 1
	}
}

// MoveItem relocates the item with id to toSection, preserving its ID and
// stamping UpdatedAt. An optional position is a 0-based target index within
// the item's new section: when given, MoveItem physically splices the item
// to that index and densely renumbers the whole section (see
// insertAtIndex) so slice order and Position order agree exactly. Omit
// position to leave Position unchanged. At most one position value is
// honored — extras are ignored.
//
// A same-section move (fromSection == toSection) with no position given is
// a true no-op — it never re-appends the item to the end of its slice,
// which would disturb sibling order for no reason. With a position given,
// same- and cross-section moves both splice+renumber identically.
func MoveItem(rm *Roadmap, id int, toSection Section, position ...int) error {
	if !ValidSection(toSection) {
		return fmt.Errorf("invalid section %q", toSection)
	}

	fromSection, idx, ok := findItem(rm, id)
	if !ok {
		return fmt.Errorf("item %d not found", id)
	}

	if fromSection == toSection {
		if len(position) == 0 {
			return nil
		}
		items := sectionSlice(rm, fromSection)
		item := (*items)[idx]
		item.UpdatedAt = nowStamp()
		*items = insertAtIndex(removeAt(*items, idx), item, position[0])
		return nil
	}

	fromItems := sectionSlice(rm, fromSection)
	item := (*fromItems)[idx]
	*fromItems = removeAt(*fromItems, idx)
	// Removing an item from a DENSE source section can leave a gap in its
	// 1..N numbering; close it. A LEGACY (all-0) source stays untouched.
	renumberIfDense(*fromItems)

	item.UpdatedAt = nowStamp()
	toItems := sectionSlice(rm, toSection)
	if len(position) > 0 {
		*toItems = insertAtIndex(*toItems, item, position[0])
	} else {
		// No explicit position: the item's Position is whatever it was in
		// the SOURCE section — stale and meaningless in the target, and
		// (if nonzero) would wrongly promote a LEGACY target to DENSE by
		// accident. Reset it before appending; renumberIfDense then
		// correctly makes it land last if the target is already DENSE, or
		// leaves a LEGACY target untouched.
		item.Position = 0
		*toItems = append(*toItems, item)
		renumberIfDense(*toItems)
	}
	return nil
}

// ReorderItem relocates the item with id to the given 0-based target index
// within its current section, without changing section: it physically
// splices the item to that index and densely renumbers the whole section
// (see insertAtIndex), so slice order and Position order agree exactly and
// index 0 always sorts first — even when every sibling is still at the
// legacy/unset Position 0.
func ReorderItem(rm *Roadmap, id int, position int) error {
	section, idx, ok := findItem(rm, id)
	if !ok {
		return fmt.Errorf("item %d not found", id)
	}

	items := sectionSlice(rm, section)
	item := (*items)[idx]
	item.UpdatedAt = nowStamp()
	*items = insertAtIndex(removeAt(*items, idx), item, position)
	return nil
}

// MarkDone moves the item with id to the done section.
func MarkDone(rm *Roadmap, id int) error {
	return MoveItem(rm, id, SectionDone)
}

// RemoveItem deletes the item with id from whichever section holds it.
func RemoveItem(rm *Roadmap, id int) error {
	section, idx, ok := findItem(rm, id)
	if !ok {
		return fmt.Errorf("item %d not found", id)
	}

	items := sectionSlice(rm, section)
	*items = removeAt(*items, idx)
	// Removing an item from a DENSE section can leave a gap in its 1..N
	// numbering; close it. A LEGACY (all-0) section stays untouched.
	renumberIfDense(*items)
	return nil
}

// Filter returns a copy of rm containing only items matching component and
// epic (each ignored when empty; both filters apply together when both are
// set). Section order is preserved. Returns rm unchanged (not a copy) when
// both filters are empty.
func Filter(rm *Roadmap, component, epic string) *Roadmap {
	if component == "" && epic == "" {
		return rm
	}

	out := &Roadmap{ArtifactURL: rm.ArtifactURL, NextID: rm.NextID, SchemaVersion: rm.SchemaVersion}
	for _, s := range allSections {
		*sectionSlice(out, s) = filterItems(*sectionSlice(rm, s), component, epic)
	}
	ensureInitialized(out)
	return out
}

func filterItems(items []Item, component, epic string) []Item {
	out := make([]Item, 0, len(items))
	for _, it := range items {
		if component != "" && it.Component != component {
			continue
		}
		if epic != "" && it.Epic != epic {
			continue
		}
		out = append(out, it)
	}
	return out
}

// EpicIDs returns every distinct non-empty Epic id referenced by rm's
// items, in first-appearance order across sections (render order).
func EpicIDs(rm *Roadmap) []string {
	seen := map[string]bool{}
	var ids []string
	for _, s := range allSections {
		for _, it := range *sectionSlice(rm, s) {
			if it.Epic != "" && !seen[it.Epic] {
				seen[it.Epic] = true
				ids = append(ids, it.Epic)
			}
		}
	}
	return ids
}
