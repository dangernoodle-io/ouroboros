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
	SectionNow    Section = "now"
	SectionNext   Section = "next"
	SectionParked Section = "parked"
	SectionDone   Section = "done"
)

const (
	docType     = "roadmap"
	docCategory = ""
	docTitle    = "roadmap"
	metadataKey = "data"
)

// ValidSection reports whether s is a known roadmap section.
func ValidSection(s Section) bool {
	switch s {
	case SectionNow, SectionNext, SectionParked, SectionDone:
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
	CreatedAt     string    `json:"created_at"`
	UpdatedAt     string    `json:"updated_at"`
}

// Sections holds the roadmap's four lanes.
type Sections struct {
	Now    []Item `json:"now"`
	Next   []Item `json:"next"`
	Parked []Item `json:"parked"`
	Done   []Item `json:"done"`
}

// Roadmap is the canonical per-project roadmap structure, persisted as
// metadata JSON on the singleton documents row.
type Roadmap struct {
	Sections    Sections `json:"sections"`
	ArtifactURL string   `json:"artifact_url,omitempty"`
	NextID      int      `json:"next_id"`
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
	if rm.Sections.Parked == nil {
		rm.Sections.Parked = []Item{}
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

// buildDoc renders rm into the store.Document Save/saveTx persist.
func buildDoc(project string, rm *Roadmap) (*store.Document, error) {
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
// a rendered preview mirror, so truncating it here loses no data.
func renderContent(rm *Roadmap) string {
	md := RenderMarkdown(rm)
	if len(md) <= store.MaxDocContentBytes {
		return md
	}

	count := len(rm.Sections.Now) + len(rm.Sections.Next) + len(rm.Sections.Parked) + len(rm.Sections.Done)
	marker := fmt.Sprintf("\n... (truncated; %d items — see structured data)\n", count)

	maxBody := store.MaxDocContentBytes - len(marker)
	if maxBody < 0 {
		maxBody = 0
	}
	return truncateUTF8(md, maxBody) + marker
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
	case SectionParked:
		return &rm.Sections.Parked
	case SectionDone:
		return &rm.Sections.Done
	default:
		return nil
	}
}

// findItem locates id across all sections.
func findItem(rm *Roadmap, id int) (Section, int, bool) {
	for _, s := range []Section{SectionNow, SectionNext, SectionParked, SectionDone} {
		items := *sectionSlice(rm, s)
		for i, it := range items {
			if it.ID == id {
				return s, i, true
			}
		}
	}
	return "", 0, false
}

// AddItem assigns item the next sequential ID, stamps timestamps, and
// appends it to section. Returns the assigned ID.
func AddItem(rm *Roadmap, section Section, item Item) (int, error) {
	if !ValidSection(section) {
		return 0, fmt.Errorf("invalid section %q", section)
	}

	rm.NextID++
	item.ID = rm.NextID
	now := nowStamp()
	item.CreatedAt = now
	item.UpdatedAt = now

	items := sectionSlice(rm, section)
	*items = append(*items, item)
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
	it.UpdatedAt = nowStamp()
	return nil
}

// MoveItem relocates the item with id to toSection, preserving its ID and
// stamping UpdatedAt.
func MoveItem(rm *Roadmap, id int, toSection Section) error {
	if !ValidSection(toSection) {
		return fmt.Errorf("invalid section %q", toSection)
	}

	fromSection, idx, ok := findItem(rm, id)
	if !ok {
		return fmt.Errorf("item %d not found", id)
	}
	if fromSection == toSection {
		return nil
	}

	fromItems := sectionSlice(rm, fromSection)
	item := (*fromItems)[idx]
	*fromItems = append((*fromItems)[:idx], (*fromItems)[idx+1:]...)

	item.UpdatedAt = nowStamp()
	toItems := sectionSlice(rm, toSection)
	*toItems = append(*toItems, item)
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
	*items = append((*items)[:idx], (*items)[idx+1:]...)
	return nil
}
