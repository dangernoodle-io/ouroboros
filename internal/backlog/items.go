package backlog

import (
	"database/sql"
	"fmt"
	"strings"

	"dangernoodle.io/ouroboros/internal/edges"
	"dangernoodle.io/ouroboros/internal/store"
)

// Size caps for item fields.
const (
	MaxItemDescBytes  = 8 * 1024
	MaxItemNotesBytes = 8 * 1024
)

// validItemStatuses is the allowed set for item status updates.
var validItemStatuses = map[string]bool{"open": true, "done": true}

// Executor is satisfied by both *sql.DB and *sql.Tx, letting the core
// item-write/read logic run standalone or participate in a caller-owned
// transaction (e.g. an item write plus its inline edge links committing
// atomically — see internal/app.handleBacklog).
//
// Intentionally parallel to internal/edges.Executor rather than a shared
// type: two tiny package-scoped 3-method interfaces don't justify the
// cross-package coupling a shared home would add. Revisit if a third
// package needs the same shape.
type Executor interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

type Item struct {
	ID        string `json:"id"`
	ProjectID int64  `json:"project_id,omitempty"`
	Priority  string `json:"priority"`
	Title     string `json:"title"`
	Component string `json:"component,omitempty"`
	// Epic holds the id of the epic backlog item this item belongs to, if
	// any (an epic IS a backlog item — the EPIC: title convention). Like
	// Component, it is single-valued: at most one epic per item.
	Epic        string `json:"epic,omitempty"`
	Description string `json:"description"`
	Notes       string `json:"notes,omitempty"`
	Status      string `json:"status"`
	Created     string `json:"created"`
	Updated     string `json:"updated"`
}

// addItemExec contains the core item-creation logic (seq compute + insert),
// operating on any Executor (db or tx). Callers own the transaction
// boundary (see AddItem / AddItemTx).
func addItemExec(ex Executor, projectID int64, prefix, priority, title, description, notes, component, epic string) (*Item, error) {
	if len(description) > MaxItemDescBytes {
		return nil, fmt.Errorf("description exceeds %d byte cap (got %d)", MaxItemDescBytes, len(description))
	}
	if len(notes) > MaxItemNotesBytes {
		return nil, fmt.Errorf("notes exceeds %d byte cap (got %d)", MaxItemNotesBytes, len(notes))
	}

	var seq int64
	err := ex.QueryRow("SELECT COALESCE(MAX(seq), 0) + 1 FROM items WHERE project_id = ?", projectID).Scan(&seq)
	if err != nil {
		return nil, err
	}

	id := fmt.Sprintf("%s-%d", prefix, seq)
	now := nowRFC3339()

	_, err = ex.Exec(
		"INSERT INTO items (id, project_id, seq, priority, title, description, notes, component, epic, status, created, updated) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'open', ?, ?)",
		id, projectID, seq, priority, title, description, notes, component, epic, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("add item: %w", err)
	}

	return &Item{
		ID: id, ProjectID: projectID, Priority: priority,
		Title: title, Component: component, Epic: epic, Description: description, Notes: notes, Status: "open",
		Created: now, Updated: now,
	}, nil
}

// AddItem inserts a new item, computing its sequence number within its own
// (self-managed) transaction.
func AddItem(d *sql.DB, projectID int64, prefix, priority, title, description, notes, component, epic string) (*Item, error) {
	tx, err := d.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	item, err := addItemExec(tx, projectID, prefix, priority, title, description, notes, component, epic)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return item, nil
}

// AddItemTx is AddItem scoped to an existing transaction, for callers that
// need item creation plus subsequent operations (e.g. inline edge links) to
// commit atomically. The caller owns commit/rollback.
func AddItemTx(tx *sql.Tx, projectID int64, prefix, priority, title, description, notes, component, epic string) (*Item, error) {
	return addItemExec(tx, projectID, prefix, priority, title, description, notes, component, epic)
}

func getItemDirect(ex Executor, id string) (*Item, error) {
	var item Item
	err := ex.QueryRow(
		"SELECT id, project_id, priority, title, component, epic, description, notes, status, created, updated FROM items WHERE id = ?", id,
	).Scan(&item.ID, &item.ProjectID, &item.Priority, &item.Title, &item.Component, &item.Epic, &item.Description, &item.Notes, &item.Status, &item.Created, &item.Updated)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("item not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// GetItem fetches an item by id, following one hop of item_id_aliases on a
// miss. Accepts Executor so callers already inside a transaction (e.g. an
// inline edge target-existence check) can reuse it without a second
// connection.
func GetItem(ex Executor, id string) (*Item, error) {
	item, err := getItemDirect(ex, id)
	if err == nil {
		return item, nil
	}
	// Only attempt alias resolution on not-found; propagate all other errors immediately.
	if !strings.HasPrefix(err.Error(), "item not found:") {
		return nil, err
	}
	// ON UPDATE CASCADE keeps alias rows pointing at current IDs, so one level of indirection is sufficient.
	var resolved string
	if aerr := ex.QueryRow("SELECT new_id FROM item_id_aliases WHERE old_id = ?", id).Scan(&resolved); aerr != nil {
		return nil, err // return original not-found
	}
	return getItemDirect(ex, resolved)
}

// GetItemsByIDs fetches items by id in a single batched query (no
// alias-following — see EpicLabels for the alias-aware caller). An id with
// no matching row is simply omitted from the result, not an error.
func GetItemsByIDs(d *sql.DB, ids []string) ([]Item, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	rows, err := d.Query(
		"SELECT id, project_id, priority, title, component, epic, description, status, created, updated FROM items WHERE id IN ("+strings.Join(placeholders, ",")+")",
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("get items by ids: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	return scanItems(rows)
}

// epicLabel strips a leading "EPIC:" title prefix (the epic-item title
// convention), trimming surrounding whitespace.
func epicLabel(title string) string {
	return strings.TrimSpace(strings.TrimPrefix(title, "EPIC:"))
}

// EpicLabels resolves each id in ids to a display label: the referenced
// backlog item's title with a leading "EPIC:" prefix stripped (an epic IS
// a backlog item — the EPIC: title convention). Every id is fetched with
// one batched query (GetItemsByIDs); only ids that query misses fall back
// to a per-id GetItem lookup, which additionally resolves one alias hop
// (so a renamed epic still resolves) — a rare path, so this stays a single
// query in the common (no renames) case rather than N+1. An id with no
// matching backlog item, even after alias resolution, is simply omitted
// from the result map; callers fall back to rendering the raw id.
func EpicLabels(d *sql.DB, ids []string) map[string]string {
	labels := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return labels
	}

	if items, err := GetItemsByIDs(d, ids); err == nil {
		for _, item := range items {
			labels[item.ID] = epicLabel(item.Title)
		}
	}

	for _, id := range ids {
		if _, ok := labels[id]; ok {
			continue
		}
		item, err := GetItem(d, id)
		if err != nil || item == nil {
			continue
		}
		labels[id] = epicLabel(item.Title)
	}
	return labels
}

// updateItemExec contains the core field-update logic, operating on any
// Executor (db or tx). Callers own the transaction boundary (see UpdateItem
// / UpdateItemTx).
func updateItemExec(ex Executor, id string, fields map[string]string) (*Item, error) {
	allowed := map[string]bool{"priority": true, "title": true, "description": true, "notes": true, "status": true, "component": true, "epic": true}

	var sets []string
	var args []interface{}
	for k, v := range fields {
		if !allowed[k] {
			return nil, fmt.Errorf("invalid field: %s", k)
		}
		if k == "status" && !validItemStatuses[v] {
			return nil, fmt.Errorf("invalid status %q: must be one of open, done", v)
		}
		if k == "description" && len(v) > MaxItemDescBytes {
			return nil, fmt.Errorf("description exceeds %d byte cap (got %d)", MaxItemDescBytes, len(v))
		}
		if k == "notes" && len(v) > MaxItemNotesBytes {
			return nil, fmt.Errorf("notes exceeds %d byte cap (got %d)", MaxItemNotesBytes, len(v))
		}
		sets = append(sets, k+" = ?")
		args = append(args, v)
	}
	if len(sets) == 0 {
		return GetItem(ex, id)
	}

	now := nowRFC3339()
	sets = append(sets, "updated = ?")
	args = append(args, now)
	args = append(args, id)

	_, err := ex.Exec("UPDATE items SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	if err != nil {
		return nil, fmt.Errorf("update item: %w", err)
	}
	return GetItem(ex, id)
}

// UpdateItem patches an item's fields via a single statement (no explicit
// transaction needed — the update is already atomic).
func UpdateItem(d *sql.DB, id string, fields map[string]string) (*Item, error) {
	return updateItemExec(d, id, fields)
}

// UpdateItemTx is UpdateItem scoped to an existing transaction, for callers
// that need the field update plus subsequent operations (e.g. inline edge
// links) to commit atomically. The caller owns commit/rollback.
func UpdateItemTx(tx *sql.Tx, id string, fields map[string]string) (*Item, error) {
	return updateItemExec(tx, id, fields)
}

func MarkDone(d *sql.DB, id string) error {
	now := nowRFC3339()
	result, err := d.Exec("UPDATE items SET status = 'done', updated = ? WHERE id = ?", now, id)
	if err != nil {
		return fmt.Errorf("mark done: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("item not found: %s", id)
	}
	return nil
}

// DeleteItems deletes items by id, cascading edge cleanup so a deleted item
// never leaves an orphaned edge (source or target) behind. Runs in a single
// transaction: item rows and their edges are removed together.
func DeleteItems(d *sql.DB, ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	tx, err := d.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	result, err := tx.Exec("DELETE FROM items WHERE id IN ("+strings.Join(placeholders, ",")+") ", args...)
	if err != nil {
		return 0, fmt.Errorf("delete items: %w", err)
	}

	for _, id := range ids {
		if _, err := edges.CascadeDelete(tx, edges.TypeItem, id); err != nil {
			return 0, fmt.Errorf("delete items: cascade cleanup: %w", err)
		}
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return affected, nil
}

type ItemFilter struct {
	ProjectIDs  []int64
	PriorityMin *int
	PriorityMax *int
	Status      *string
	Component   *string
	Limit       int
}

// buildItemFilterWhere renders the shared ItemFilter predicates (project/priority/status/component)
// as SQL fragments, using colPrefix for column references (e.g. "" or "items.").
func buildItemFilterWhere(f ItemFilter, colPrefix string) (string, []interface{}) {
	var clause strings.Builder
	var args []interface{}

	if len(f.ProjectIDs) == 1 {
		clause.WriteString(" AND " + colPrefix + "project_id = ?")
		args = append(args, f.ProjectIDs[0])
	} else if len(f.ProjectIDs) > 1 {
		placeholders := make([]string, len(f.ProjectIDs))
		for i, id := range f.ProjectIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		clause.WriteString(" AND " + colPrefix + "project_id IN (" + strings.Join(placeholders, ",") + ")")
	}
	if f.PriorityMin != nil {
		clause.WriteString(" AND CAST(SUBSTR(" + colPrefix + "priority, 2) AS INTEGER) >= ?")
		args = append(args, *f.PriorityMin)
	}
	if f.PriorityMax != nil {
		clause.WriteString(" AND CAST(SUBSTR(" + colPrefix + "priority, 2) AS INTEGER) <= ?")
		args = append(args, *f.PriorityMax)
	}
	if f.Status != nil {
		clause.WriteString(" AND " + colPrefix + "status = ?")
		args = append(args, *f.Status)
	}
	if f.Component != nil {
		clause.WriteString(" AND " + colPrefix + "component = ?")
		args = append(args, *f.Component)
	}

	return clause.String(), args
}

// scanItems drains rows produced by the shared item column list (id, project_id,
// priority, title, component, epic, description, status, created, updated).
func scanItems(rows *sql.Rows) ([]Item, error) {
	var items []Item
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ID, &item.ProjectID, &item.Priority, &item.Title, &item.Component, &item.Epic, &item.Description, &item.Status, &item.Created, &item.Updated); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func ListItems(d *sql.DB, f ItemFilter) ([]Item, error) {
	query := "SELECT id, project_id, priority, title, component, epic, description, status, created, updated FROM items WHERE 1=1"
	whereClause, args := buildItemFilterWhere(f, "")
	query += whereClause

	query += " ORDER BY CAST(SUBSTR(priority, 2) AS INTEGER), id"

	query += " LIMIT ?"
	args = append(args, store.ClampLimit(f.Limit, 10, 500))

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	return scanItems(rows)
}

// SearchItems performs an FTS5 search over backlog items (title, description,
// notes), honoring the same ItemFilter as ListItems, ordered by relevance (bm25).
func SearchItems(d *sql.DB, query string, f ItemFilter) ([]Item, error) {
	ftsQuery := store.FtsEscape(query)
	if ftsQuery == "" {
		return nil, nil
	}

	q := `SELECT items.id, items.project_id, items.priority, items.title, items.component, items.epic, items.description, items.status, items.created, items.updated
		FROM items
		JOIN items_fts ON items.rowid = items_fts.rowid
		WHERE items_fts MATCH ?`
	args := []interface{}{ftsQuery}

	whereClause, filterArgs := buildItemFilterWhere(f, "items.")
	q += whereClause
	args = append(args, filterArgs...)

	q += " ORDER BY bm25(items_fts)"

	q += " LIMIT ?"
	args = append(args, store.ClampLimit(f.Limit, 10, 500))

	rows, err := d.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("search items: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	return scanItems(rows)
}

// PriorityCount holds a priority level and its count.
type PriorityCount struct {
	Priority string `json:"priority"`
	Count    int    `json:"count"`
}

// CountItemsByPriority returns item counts grouped by priority.
// Reuses ItemFilter for filtering (typically Status="open").
func CountItemsByPriority(d *sql.DB, f ItemFilter) ([]PriorityCount, error) {
	query := "SELECT priority, COUNT(*) FROM items WHERE 1=1"
	var args []interface{}

	if len(f.ProjectIDs) == 1 {
		query += " AND project_id = ?"
		args = append(args, f.ProjectIDs[0])
	} else if len(f.ProjectIDs) > 1 {
		placeholders := make([]string, len(f.ProjectIDs))
		for i, id := range f.ProjectIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		query += " AND project_id IN (" + strings.Join(placeholders, ",") + ")"
	}
	if f.Status != nil {
		query += " AND status = ?"
		args = append(args, *f.Status)
	}
	if f.Component != nil {
		query += " AND component = ?"
		args = append(args, *f.Component)
	}

	query += " GROUP BY priority ORDER BY priority"

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("count items by priority: %w", err)
	}
	defer rows.Close()

	var counts []PriorityCount
	for rows.Next() {
		var pc PriorityCount
		if err := rows.Scan(&pc.Priority, &pc.Count); err != nil {
			return nil, fmt.Errorf("scan priority count: %w", err)
		}
		counts = append(counts, pc)
	}
	return counts, rows.Err()
}
