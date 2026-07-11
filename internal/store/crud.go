package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// Size caps for document fields.
const (
	MaxDocContentBytes = 32 * 1024
	MaxDocNotesBytes   = 32 * 1024
)

// UpsertResult indicates whether a document was created or updated.
type UpsertResult struct {
	ID              int64  `json:"id"`
	Action          string `json:"action"` // "created" or "updated"
	Conflict        bool   `json:"conflict,omitempty"`
	PreviousContent string `json:"previous_content,omitempty"`
}

// stopWords are filtered from keyword search queries to reduce noise from natural-language prompts.
var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "are": true,
	"was": true, "were": true, "be": true, "been": true, "being": true,
	"have": true, "has": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true,
	"may": true, "might": true, "can": true, "shall": true,
	"i": true, "you": true, "he": true, "she": true, "it": true,
	"we": true, "they": true, "me": true, "him": true, "her": true,
	"us": true, "them": true, "my": true, "your": true, "our": true,
	"this": true, "that": true, "these": true, "those": true,
	"in": true, "on": true, "at": true, "to": true, "for": true,
	"of": true, "with": true, "from": true, "by": true, "about": true,
	"and": true, "or": true, "but": true, "not": true, "no": true,
	"if": true, "then": true, "so": true, "just": true,
	"let": true, "lets": true, "let's": true, "what": true, "how": true,
	"why": true, "when": true, "where": true, "which": true, "who": true,
}

// TokenizeQuery splits a query string into meaningful search terms,
// stripping punctuation and stop words.
func TokenizeQuery(query string) []string {
	words := strings.Fields(strings.ToLower(query))
	terms := []string{}
	for _, w := range words {
		// Strip leading/trailing punctuation
		w = strings.Trim(w, ".,;:!?\"'`()[]{}/-")
		if w == "" || stopWords[w] {
			continue
		}
		terms = append(terms, w)
	}
	return terms
}

// projectFilter appends a project filter clause and args for the given project names.
func projectFilter(prefix string, projects []string, args *[]interface{}) string {
	if len(projects) == 0 {
		return ""
	}
	if len(projects) == 1 {
		*args = append(*args, projects[0])
		return " AND " + prefix + "project = ?"
	}
	placeholders := make([]string, len(projects))
	for i, p := range projects {
		placeholders[i] = "?"
		*args = append(*args, p)
	}
	return " AND " + prefix + "project IN (" + strings.Join(placeholders, ",") + ")"
}

// KeywordSearch performs a keyword-based FTS5 search with BM25 ranking.
// Terms are ORed together so any match counts. Returns results ranked by relevance.
// Returns nil if no meaningful terms after stop word removal.
func KeywordSearch(db *sql.DB, query string, projects []string, limit int) ([]DocumentSummary, error) {
	terms := TokenizeQuery(query)
	if len(terms) == 0 {
		return []DocumentSummary{}, nil
	}

	// Build FTS5 query: term1 OR term2 OR term3
	// Each term is quoted to handle special chars
	var quoted []string
	for _, t := range terms {
		quoted = append(quoted, "\""+strings.ReplaceAll(t, "\"", "\"\"")+"\"")
	}
	ftsQuery := strings.Join(quoted, " OR ")

	limit = ClampLimit(limit, 10, 500)

	sqlQuery := `SELECT d.id, d.type, d.project, d.category, d.title, d.tags, d.updated_at, bm25(documents_fts) AS score
		FROM documents d
		JOIN documents_fts fts ON d.id = fts.rowid
		WHERE fts.documents_fts MATCH ?`
	args := []interface{}{ftsQuery}

	sqlQuery += projectFilter("d.", projects, &args)

	sqlQuery += " ORDER BY bm25(documents_fts) LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("keyword search: %w", err)
	}
	defer rows.Close()

	var results []DocumentSummary
	for rows.Next() {
		var s DocumentSummary
		var tagsJSON sql.NullString
		if err := rows.Scan(&s.ID, &s.Type, &s.Project, &s.Category, &s.Title, &tagsJSON, &s.UpdatedAt, &s.Score); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		if tagsJSON.Valid && tagsJSON.String != "" {
			if err := json.Unmarshal([]byte(tagsJSON.String), &s.Tags); err != nil {
				s.Tags = []string{}
			}
		}
		if len(s.UpdatedAt) >= 10 {
			s.UpdatedAt = s.UpdatedAt[:10]
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

// sqlExecutor is satisfied by both *sql.DB and *sql.Tx.
type sqlExecutor interface {
	QueryRow(query string, args ...any) *sql.Row
	Exec(query string, args ...any) (sql.Result, error)
}

// upsertDocumentExec contains the core upsert logic operating on a sqlExecutor.
// Callers are responsible for FTS rebuild and locking.
//
// Action is determined atomically via RETURNING: on a fresh insert created_at == updated_at;
// on a conflict update only updated_at changes, so created_at < updated_at signals "updated".
func upsertDocumentExec(q sqlExecutor, doc Document) (*UpsertResult, error) {
	if len(doc.Content) > MaxDocContentBytes {
		return nil, fmt.Errorf("content exceeds %d byte cap (got %d)", MaxDocContentBytes, len(doc.Content))
	}
	if len(doc.Notes) > MaxDocNotesBytes {
		return nil, fmt.Errorf("notes exceeds %d byte cap (got %d)", MaxDocNotesBytes, len(doc.Notes))
	}

	// Conflict detection: check for existing content drift before upsert.
	var prevContent string
	var conflict bool
	var existing sql.NullString
	err := q.QueryRow(
		`SELECT content FROM documents WHERE type=? AND project=? AND category=? AND title=? LIMIT 1`,
		doc.Type, doc.Project, doc.Category, doc.Title,
	).Scan(&existing)
	if err == nil && existing.Valid && existing.String != doc.Content {
		conflict = true
		prevContent = existing.String
	}

	metadataJSON, err := json.Marshal(doc.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	tagsJSON, err := json.Marshal(doc.Tags)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tags: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Extract session_id: prefer top-level field, fall back to metadata key.
	sessionID := doc.SessionID
	if sessionID == "" && doc.Metadata != nil {
		sessionID = doc.Metadata["session_id"]
	}
	var sessionIDArg interface{}
	if sessionID != "" {
		sessionIDArg = sessionID
	}

	// Single atomic statement: insert or update, returning id and timestamps.
	// On a fresh insert created_at == updated_at == now.
	// On conflict update, only updated_at changes to now; created_at keeps its
	// original value, so created_at != updated_at → "updated".
	// RFC3339Nano resolution makes same-nanosecond collision negligible.
	var id int64
	var createdAt, updatedAt string
	err = q.QueryRow(`
		INSERT INTO documents (type, project, category, title, content, notes, session_id, metadata, tags, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(type, project, category, title)
		DO UPDATE SET content = excluded.content, notes = excluded.notes, session_id = excluded.session_id, metadata = excluded.metadata, tags = excluded.tags, updated_at = excluded.updated_at
		RETURNING id, created_at, updated_at
	`, doc.Type, doc.Project, doc.Category, doc.Title, doc.Content, doc.Notes, sessionIDArg, string(metadataJSON), string(tagsJSON), now, now).Scan(&id, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert document: %w", err)
	}

	action := "created"
	if createdAt != updatedAt {
		action = "updated"
	}

	return &UpsertResult{ID: id, Action: action, Conflict: conflict, PreviousContent: prevContent}, nil
}

// UpsertDocument inserts or updates a document record using ON CONFLICT.
// Returns UpsertResult with ID and action (created/updated).
func UpsertDocument(db *sql.DB, doc Document) (*UpsertResult, error) {
	dbMu.Lock()
	defer dbMu.Unlock()

	result, err := upsertDocumentExec(db, doc)
	if err != nil {
		return nil, err
	}

	if err := RebuildFTS(db); err != nil {
		return nil, fmt.Errorf("failed to rebuild FTS: %w", err)
	}

	return result, nil
}

// UpsertDocumentTx inserts or updates a document within an existing transaction.
// The caller owns the transaction and is responsible for commit/rollback and FTS rebuild.
func UpsertDocumentTx(tx *sql.Tx, doc Document) (*UpsertResult, error) {
	return upsertDocumentExec(tx, doc)
}

// UpdateDocumentFields is a partial, id-addressed document patch. Nil
// pointers mean "leave unchanged" — this is what lets a title-only update
// retitle a document in place without touching its content/notes/tags, and
// is the fix for the retitle-creates-a-duplicate bug (the natural-key
// upsert treats title as part of the key).
type UpdateDocumentFields struct {
	Type     *string
	Project  *string
	Category *string
	Title    *string
	Content  *string
	Notes    *string
	Tags     *[]string
	Metadata *map[string]string
}

// updateDocumentExec contains the core id-addressed partial-update logic
// operating on a sqlExecutor. Callers are responsible for FTS rebuild.
// Returns an error if id does not exist.
func updateDocumentExec(q sqlExecutor, id int64, fields UpdateDocumentFields) (*Document, error) {
	var sets []string
	var args []interface{}

	if fields.Type != nil {
		sets = append(sets, "type = ?")
		args = append(args, *fields.Type)
	}
	if fields.Project != nil {
		sets = append(sets, "project = ?")
		args = append(args, *fields.Project)
	}
	if fields.Category != nil {
		sets = append(sets, "category = ?")
		args = append(args, *fields.Category)
	}
	if fields.Title != nil {
		sets = append(sets, "title = ?")
		args = append(args, *fields.Title)
	}
	if fields.Content != nil {
		if len(*fields.Content) > MaxDocContentBytes {
			return nil, fmt.Errorf("content exceeds %d byte cap (got %d)", MaxDocContentBytes, len(*fields.Content))
		}
		sets = append(sets, "content = ?")
		args = append(args, *fields.Content)
	}
	if fields.Notes != nil {
		if len(*fields.Notes) > MaxDocNotesBytes {
			return nil, fmt.Errorf("notes exceeds %d byte cap (got %d)", MaxDocNotesBytes, len(*fields.Notes))
		}
		sets = append(sets, "notes = ?")
		args = append(args, *fields.Notes)
	}
	if fields.Tags != nil {
		tagsJSON, err := json.Marshal(*fields.Tags)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tags: %w", err)
		}
		sets = append(sets, "tags = ?")
		args = append(args, string(tagsJSON))
	}
	if fields.Metadata != nil {
		metadataJSON, err := json.Marshal(*fields.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
		sets = append(sets, "metadata = ?")
		args = append(args, string(metadataJSON))
	}

	if len(sets) == 0 {
		doc, err := getDocumentExec(q, "id = ?", id)
		if err != nil {
			return nil, err
		}
		if doc == nil {
			return nil, fmt.Errorf("document %d not found", id)
		}
		return doc, nil
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	sets = append(sets, "updated_at = ?")
	args = append(args, now)
	args = append(args, id)

	result, err := q.Exec("UPDATE documents SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	if err != nil {
		return nil, fmt.Errorf("failed to update document: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to update document: %w", err)
	}
	if rows == 0 {
		return nil, fmt.Errorf("document %d not found", id)
	}

	return getDocumentExec(q, "id = ?", id)
}

// UpdateDocumentTx applies a partial, id-addressed update within an existing
// transaction. The caller owns commit/rollback and FTS rebuild.
func UpdateDocumentTx(tx *sql.Tx, id int64, fields UpdateDocumentFields) (*Document, error) {
	return updateDocumentExec(tx, id, fields)
}

// UpdateDocument applies a partial, id-addressed update and rebuilds FTS.
func UpdateDocument(db *sql.DB, id int64, fields UpdateDocumentFields) (*Document, error) {
	dbMu.Lock()
	defer dbMu.Unlock()

	doc, err := updateDocumentExec(db, id, fields)
	if err != nil {
		return nil, err
	}

	if err := RebuildFTS(db); err != nil {
		return nil, fmt.Errorf("failed to rebuild FTS: %w", err)
	}

	return doc, nil
}

// GetDocument returns a full Document by ID. Returns nil, nil if not found.
func GetDocument(db *sql.DB, id int64) (*Document, error) {
	return getDocumentExec(db, "id = ?", id)
}

// GetDocumentByKey returns a full Document by its natural key
// (type, project, category, title). Returns nil, nil if not found.
func GetDocumentByKey(db *sql.DB, docType, project, category, title string) (*Document, error) {
	return getDocumentExec(db, "type = ? AND project = ? AND category = ? AND title = ?", docType, project, category, title)
}

// GetDocumentByKeyTx is GetDocumentByKey scoped to an existing transaction,
// for callers that need the read to participate in a caller-owned tx (e.g.
// a serialized load-mutate-save cycle).
func GetDocumentByKeyTx(tx *sql.Tx, docType, project, category, title string) (*Document, error) {
	return getDocumentExec(tx, "type = ? AND project = ? AND category = ? AND title = ?", docType, project, category, title)
}

// getDocumentExec contains the shared scan logic for GetDocument and
// GetDocumentByKey(Tx), operating on any sqlExecutor (db or tx).
func getDocumentExec(q sqlExecutor, whereClause string, args ...interface{}) (*Document, error) {
	var doc Document
	var metadataJSON sql.NullString
	var tagsJSON sql.NullString
	var notes sql.NullString
	var sessionID sql.NullString

	err := q.QueryRow(`
		SELECT id, type, project, category, title, content, notes, session_id, metadata, tags, created_at, updated_at
		FROM documents WHERE `+whereClause, args...).Scan(&doc.ID, &doc.Type, &doc.Project, &doc.Category, &doc.Title, &doc.Content, &notes, &sessionID, &metadataJSON, &tagsJSON, &doc.CreatedAt, &doc.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}

	if notes.Valid {
		doc.Notes = notes.String
	}

	if sessionID.Valid {
		doc.SessionID = sessionID.String
	}

	if metadataJSON.Valid {
		if err := json.Unmarshal([]byte(metadataJSON.String), &doc.Metadata); err != nil {
			doc.Metadata = map[string]string{}
		}
	}

	if tagsJSON.Valid {
		if err := json.Unmarshal([]byte(tagsJSON.String), &doc.Tags); err != nil {
			doc.Tags = []string{}
		}
	}

	return &doc, nil
}

// typeFilter appends a type filter clause and args for the given type names.
func typeFilter(prefix string, types []string, args *[]interface{}) string {
	if len(types) == 0 {
		return ""
	}
	if len(types) == 1 {
		*args = append(*args, types[0])
		return " AND " + prefix + "type = ?"
	}
	placeholders := make([]string, len(types))
	for i, tp := range types {
		placeholders[i] = "?"
		*args = append(*args, tp)
	}
	return " AND " + prefix + "type IN (" + strings.Join(placeholders, ",") + ")"
}

// categoryFilter appends a category filter clause and args for the given category names.
func categoryFilter(prefix string, categories []string, args *[]interface{}) string {
	if len(categories) == 0 {
		return ""
	}
	if len(categories) == 1 {
		*args = append(*args, categories[0])
		return " AND " + prefix + "category = ?"
	}
	placeholders := make([]string, len(categories))
	for i, c := range categories {
		placeholders[i] = "?"
		*args = append(*args, c)
	}
	return " AND " + prefix + "category IN (" + strings.Join(placeholders, ",") + ")"
}

// QueryDocuments queries documents with optional filters (types, project, categories, FTS, tags, sessionID).
// Returns DocumentSummary (no content, no metadata) to conserve tokens.
func QueryDocuments(db *sql.DB, types []string, projects []string, categories []string, ftsQuery string, tags []string, limit int, sessionID ...string) ([]DocumentSummary, error) {
	sid := ""
	if len(sessionID) > 0 {
		sid = sessionID[0]
	}
	limit = ClampLimit(limit, 10, 500)

	var query string
	var args []interface{}
	escaped := FtsEscape(ftsQuery)
	isFTS := escaped != ""

	if isFTS {
		// FTS5 query
		query = `
			SELECT d.id, d.type, d.project, d.category, d.title, d.tags, d.updated_at, bm25(documents_fts) AS score
			FROM documents d
			JOIN documents_fts fts ON d.id = fts.rowid
			WHERE fts.documents_fts MATCH ?
		`
		args = append(args, escaped)

		query += typeFilter("d.", types, &args)
		query += projectFilter("d.", projects, &args)
		query += categoryFilter("d.", categories, &args)
		if sid != "" {
			query += " AND d.session_id = ?"
			args = append(args, sid)
		}

		query += " LIMIT ?"
		args = append(args, limit)
	} else {
		// Standard SQL query
		query = "SELECT id, type, project, category, title, tags, updated_at FROM documents"

		whereClause := ""
		if len(types) == 1 {
			whereClause += "type = ?"
			args = append(args, types[0])
		} else if len(types) > 1 {
			placeholders := make([]string, len(types))
			for i, tp := range types {
				placeholders[i] = "?"
				args = append(args, tp)
			}
			whereClause += "type IN (" + strings.Join(placeholders, ",") + ")"
		}
		if len(projects) > 0 {
			if whereClause != "" {
				whereClause += " AND "
			}
			if len(projects) == 1 {
				whereClause += "project = ?"
				args = append(args, projects[0])
			} else {
				placeholders := make([]string, len(projects))
				for i, p := range projects {
					placeholders[i] = "?"
					args = append(args, p)
				}
				whereClause += "project IN (" + strings.Join(placeholders, ",") + ")"
			}
		}
		if len(categories) == 1 {
			if whereClause != "" {
				whereClause += " AND "
			}
			whereClause += "category = ?"
			args = append(args, categories[0])
		} else if len(categories) > 1 {
			if whereClause != "" {
				whereClause += " AND "
			}
			placeholders := make([]string, len(categories))
			for i, c := range categories {
				placeholders[i] = "?"
				args = append(args, c)
			}
			whereClause += "category IN (" + strings.Join(placeholders, ",") + ")"
		}
		if sid != "" {
			if whereClause != "" {
				whereClause += " AND "
			}
			whereClause += "session_id = ?"
			args = append(args, sid)
		}
		for _, tag := range tags {
			if whereClause != "" {
				whereClause += " AND "
			}
			whereClause += "EXISTS (SELECT 1 FROM json_each(documents.tags) WHERE value = ?)"
			args = append(args, tag)
		}

		if whereClause != "" {
			query += " WHERE " + whereClause
		}

		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query documents: %w", err)
	}
	defer rows.Close()

	var summaries []DocumentSummary
	for rows.Next() {
		var summary DocumentSummary
		var tagsJSON sql.NullString

		if isFTS {
			if err := rows.Scan(&summary.ID, &summary.Type, &summary.Project, &summary.Category, &summary.Title, &tagsJSON, &summary.UpdatedAt, &summary.Score); err != nil {
				return nil, fmt.Errorf("failed to scan document summary: %w", err)
			}
		} else {
			if err := rows.Scan(&summary.ID, &summary.Type, &summary.Project, &summary.Category, &summary.Title, &tagsJSON, &summary.UpdatedAt); err != nil {
				return nil, fmt.Errorf("failed to scan document summary: %w", err)
			}
		}

		if tagsJSON.Valid {
			if err := json.Unmarshal([]byte(tagsJSON.String), &summary.Tags); err != nil {
				summary.Tags = []string{}
			}
		}

		if len(summary.UpdatedAt) >= 10 {
			summary.UpdatedAt = summary.UpdatedAt[:10]
		}

		summaries = append(summaries, summary)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating documents: %w", err)
	}

	return summaries, nil
}

// DeleteDocumentTx deletes a document by ID within an existing transaction.
// The caller owns the transaction and is responsible for commit/rollback
// and FTS rebuild (edge cascade cleanup is also the caller's job — see
// internal/edges.CascadeDelete — to keep this package edges-agnostic).
func DeleteDocumentTx(tx *sql.Tx, id int64) error {
	_, err := tx.Exec("DELETE FROM documents WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}
	return nil
}

// DeleteDocument deletes a document by ID.
func DeleteDocument(db *sql.DB, id int64) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	if err := DeleteDocumentTx(tx, id); err != nil {
		_ = tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	if err := RebuildFTS(db); err != nil {
		return fmt.Errorf("failed to rebuild FTS: %w", err)
	}

	return nil
}

// hasSearchableTokens checks if a query string contains any alphanumeric characters.
// Returns false if the query only contains punctuation, whitespace, or wildcards.
func hasSearchableTokens(q string) bool {
	for _, r := range q {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// SearchDocuments performs a full-text search across all documents.
// Returns DocumentSummary (no content, no metadata).
func SearchDocuments(db *sql.DB, query string, types []string, projects []string, categories []string, limit int) ([]DocumentSummary, error) {
	limit = ClampLimit(limit, 10, 500)

	// If query has no searchable tokens (only punctuation/wildcards), fall back to list mode
	if !hasSearchableTokens(query) {
		return QueryDocuments(db, types, projects, categories, "", nil, limit)
	}

	escapedQuery := FtsEscape(query)

	ftQuery := `
		SELECT d.id, d.type, d.project, d.category, d.title, d.tags, d.updated_at
		FROM documents d
		JOIN documents_fts fts ON d.id = fts.rowid
		WHERE fts.documents_fts MATCH ?
	`
	args := []interface{}{escapedQuery}

	ftQuery += typeFilter("d.", types, &args)
	ftQuery += projectFilter("d.", projects, &args)
	ftQuery += categoryFilter("d.", categories, &args)

	ftQuery += " LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(ftQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search documents: %w", err)
	}
	defer rows.Close()

	summaries := []DocumentSummary{}
	for rows.Next() {
		var summary DocumentSummary
		var tagsJSON sql.NullString

		if err := rows.Scan(&summary.ID, &summary.Type, &summary.Project, &summary.Category, &summary.Title, &tagsJSON, &summary.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", err)
		}

		if tagsJSON.Valid {
			if err := json.Unmarshal([]byte(tagsJSON.String), &summary.Tags); err != nil {
				summary.Tags = []string{}
			}
		}

		if len(summary.UpdatedAt) >= 10 {
			summary.UpdatedAt = summary.UpdatedAt[:10]
		}

		summaries = append(summaries, summary)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating search results: %w", err)
	}

	return summaries, nil
}

// TypeCount holds a document type and its count.
type TypeCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// CountDocumentsByType returns document counts grouped by type.
// If projects is non-empty, counts are filtered to those projects.
func CountDocumentsByType(db *sql.DB, projects []string) ([]TypeCount, error) {
	query := "SELECT type, COUNT(*) FROM documents"
	var args []interface{}

	if len(projects) > 0 {
		query += " WHERE"
		if len(projects) == 1 {
			// Case-insensitive to match GetProjectByName — a KB entry whose
			// project casing diverges from the canonical project name must
			// still be counted, not silently dropped.
			query += " LOWER(project) = LOWER(?)"
			args = append(args, projects[0])
		} else {
			placeholders := make([]string, len(projects))
			for i, p := range projects {
				placeholders[i] = "LOWER(?)"
				args = append(args, p)
			}
			query += " LOWER(project) IN (" + strings.Join(placeholders, ",") + ")"
		}
	}

	query += " GROUP BY type ORDER BY type"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("count documents by type: %w", err)
	}
	defer rows.Close()

	var counts []TypeCount
	for rows.Next() {
		var tc TypeCount
		if err := rows.Scan(&tc.Type, &tc.Count); err != nil {
			return nil, fmt.Errorf("scan type count: %w", err)
		}
		counts = append(counts, tc)
	}
	return counts, rows.Err()
}
