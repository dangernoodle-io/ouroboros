// Package edges implements the polymorphic cross-reference graph between
// backlog items and KB documents: blocks/relates/explains edges, keyed by
// (source_type, source_id, target_type, target_id, label). Integrity is
// enforced at the app level (no FK across the type-erased id columns);
// endpoint types are validated against a fixed vocabulary.
package edges

import (
	"database/sql"
	"fmt"
	"time"
)

// Endpoint type vocabulary.
const (
	TypeItem = "item"
	TypeKB   = "kb"
)

// validEdgeTypes is the allowed endpoint-type vocabulary.
var validEdgeTypes = map[string]bool{TypeItem: true, TypeKB: true}

// validEdgeLabels is the allowed edge-label vocabulary.
var validEdgeLabels = map[string]bool{"blocks": true, "relates": true, "explains": true}

// ValidType reports whether typ is a recognized endpoint type (item|kb).
func ValidType(typ string) bool { return validEdgeTypes[typ] }

// ValidLabel reports whether label is a recognized edge label
// (blocks|relates|explains).
func ValidLabel(label string) bool { return validEdgeLabels[label] }

// Edge is a single row of the polymorphic edges table.
type Edge struct {
	ID         int64  `json:"id"`
	SourceType string `json:"source_type"`
	SourceID   string `json:"source_id"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Label      string `json:"label"`
	ProjectID  int64  `json:"project_id,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// Executor is satisfied by both *sql.DB and *sql.Tx, letting edges
// operations run standalone or participate in a caller-owned transaction
// (e.g. cascade cleanup inside an item/doc delete).
//
// Intentionally parallel to internal/backlog.Executor rather than a shared
// type: two tiny package-scoped 3-method interfaces don't justify the
// cross-package coupling a shared home would add. Revisit if a third
// package needs the same shape.
type Executor interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// resolveID resolves an item id through item_id_aliases (one hop; ON UPDATE
// CASCADE keeps alias rows pointing at current ids, so one hop suffices) so
// an edge endpoint referencing a since-renamed item id still resolves. kb
// endpoint ids are never aliased.
func resolveID(ex Executor, typ, id string) string {
	if typ != TypeItem {
		return id
	}
	var resolved string
	if err := ex.QueryRow("SELECT new_id FROM item_id_aliases WHERE old_id = ?", id).Scan(&resolved); err == nil {
		return resolved
	}
	return id
}

func validateEndpoints(srcType, dstType, label string) error {
	if !ValidType(srcType) {
		return fmt.Errorf("invalid edge source type %q: must be item or kb", srcType)
	}
	if !ValidType(dstType) {
		return fmt.Errorf("invalid edge target type %q: must be item or kb", dstType)
	}
	if !ValidLabel(label) {
		return fmt.Errorf("invalid edge label %q: must be one of blocks, relates, explains", label)
	}
	return nil
}

// Link creates (or upserts) an edge from (srcType,srcID) to (dstType,dstID)
// with the given label. Safe to call repeatedly for the same edge (unique
// constraint on source/target/label; a repeat call updates project_id and
// returns the existing row rather than erroring). projectID of 0 stores
// NULL (unknown/unset).
func Link(ex Executor, srcType, srcID, label, dstType, dstID string, projectID int64) (*Edge, error) {
	if err := validateEndpoints(srcType, dstType, label); err != nil {
		return nil, err
	}
	if srcID == "" || dstID == "" {
		return nil, fmt.Errorf("edge source_id and target_id are required")
	}

	srcID = resolveID(ex, srcType, srcID)
	dstID = resolveID(ex, dstType, dstID)

	var projectArg interface{}
	if projectID != 0 {
		projectArg = projectID
	}

	now := time.Now().UTC().Format(time.RFC3339)

	var id int64
	var createdAt string
	err := ex.QueryRow(`
		INSERT INTO edges (source_type, source_id, target_type, target_id, label, project_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_type, source_id, target_type, target_id, label)
		DO UPDATE SET project_id = excluded.project_id
		RETURNING id, created_at
	`, srcType, srcID, dstType, dstID, label, projectArg, now).Scan(&id, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("link: %w", err)
	}

	return &Edge{
		ID:         id,
		SourceType: srcType,
		SourceID:   srcID,
		TargetType: dstType,
		TargetID:   dstID,
		Label:      label,
		ProjectID:  projectID,
		CreatedAt:  createdAt,
	}, nil
}

// Unlink removes the edge (srcType,srcID) -[label]-> (dstType,dstID), if
// present. Returns the number of rows removed (0 or 1).
func Unlink(ex Executor, srcType, srcID, label, dstType, dstID string) (int64, error) {
	if err := validateEndpoints(srcType, dstType, label); err != nil {
		return 0, err
	}

	srcID = resolveID(ex, srcType, srcID)
	dstID = resolveID(ex, dstType, dstID)

	result, err := ex.Exec(
		"DELETE FROM edges WHERE source_type = ? AND source_id = ? AND target_type = ? AND target_id = ? AND label = ?",
		srcType, srcID, dstType, dstID, label,
	)
	if err != nil {
		return 0, fmt.Errorf("unlink: %w", err)
	}
	return result.RowsAffected()
}

// EdgesFor returns every edge touching (typ,id), as source or target,
// ordered by id.
func EdgesFor(ex Executor, typ, id string) ([]Edge, error) {
	if !ValidType(typ) {
		return nil, fmt.Errorf("invalid edge type %q: must be item or kb", typ)
	}
	id = resolveID(ex, typ, id)

	rows, err := ex.Query(
		`SELECT id, source_type, source_id, target_type, target_id, label, project_id, created_at
		 FROM edges
		 WHERE (source_type = ? AND source_id = ?) OR (target_type = ? AND target_id = ?)
		 ORDER BY id`,
		typ, id, typ, id,
	)
	if err != nil {
		return nil, fmt.Errorf("edges for: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	return scanEdges(rows)
}

// ItemsByEdge returns the item ids that hold a "label" edge pointing at
// (targetType,targetID) — i.e. item-type sources of that edge.
func ItemsByEdge(ex Executor, targetType, targetID, label string) ([]string, error) {
	if !ValidType(targetType) {
		return nil, fmt.Errorf("invalid edge type %q: must be item or kb", targetType)
	}
	if !ValidLabel(label) {
		return nil, fmt.Errorf("invalid edge label %q: must be one of blocks, relates, explains", label)
	}
	targetID = resolveID(ex, targetType, targetID)

	rows, err := ex.Query(
		"SELECT source_id FROM edges WHERE source_type = ? AND target_type = ? AND target_id = ? AND label = ? ORDER BY source_id",
		TypeItem, targetType, targetID, label,
	)
	if err != nil {
		return nil, fmt.Errorf("items by edge: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListEdges returns every edge, optionally filtered to a single label,
// ordered by id.
func ListEdges(ex Executor, label string) ([]Edge, error) {
	query := "SELECT id, source_type, source_id, target_type, target_id, label, project_id, created_at FROM edges"
	var args []interface{}
	if label != "" {
		if !ValidLabel(label) {
			return nil, fmt.Errorf("invalid edge label %q: must be one of blocks, relates, explains", label)
		}
		query += " WHERE label = ?"
		args = append(args, label)
	}
	query += " ORDER BY id"

	rows, err := ex.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list edges: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	return scanEdges(rows)
}

// CascadeDelete removes every edge referencing (typ,id) as source or
// target. Called from the item/kb delete paths so a deleted endpoint never
// leaves an orphaned edge behind. No alias resolution: the id being deleted
// is already canonical.
func CascadeDelete(ex Executor, typ, id string) (int64, error) {
	if !ValidType(typ) {
		return 0, fmt.Errorf("invalid edge type %q: must be item or kb", typ)
	}
	result, err := ex.Exec(
		"DELETE FROM edges WHERE (source_type = ? AND source_id = ?) OR (target_type = ? AND target_id = ?)",
		typ, id, typ, id,
	)
	if err != nil {
		return 0, fmt.Errorf("cascade delete: %w", err)
	}
	return result.RowsAffected()
}

// DeleteAutolinks removes every (sourceType,sourceID)-labeled edge
// originating from a given source — used by AutolinkKB to clear a
// document's prior autolinks before recreating them from current content.
func DeleteAutolinks(ex Executor, sourceType, sourceID, label string) (int64, error) {
	result, err := ex.Exec(
		"DELETE FROM edges WHERE source_type = ? AND source_id = ? AND label = ?",
		sourceType, sourceID, label,
	)
	if err != nil {
		return 0, fmt.Errorf("delete autolinks: %w", err)
	}
	return result.RowsAffected()
}

func scanEdges(rows *sql.Rows) ([]Edge, error) {
	var result []Edge
	for rows.Next() {
		var e Edge
		var projectID sql.NullInt64
		if err := rows.Scan(&e.ID, &e.SourceType, &e.SourceID, &e.TargetType, &e.TargetID, &e.Label, &projectID, &e.CreatedAt); err != nil {
			return nil, err
		}
		if projectID.Valid {
			e.ProjectID = projectID.Int64
		}
		result = append(result, e)
	}
	return result, rows.Err()
}
