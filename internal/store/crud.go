package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// UpsertResult indicates whether a document was created or updated.
type UpsertResult struct {
	ID     int64  `json:"id"`
	Action string `json:"action"` // "created" or "updated"
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

	sqlQuery := `SELECT d.id, d.type, d.project, d.category, d.title, d.tags, d.updated_at
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
		if err := rows.Scan(&s.ID, &s.Type, &s.Project, &s.Category, &s.Title, &tagsJSON, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		if tagsJSON.Valid && tagsJSON.String != "" {
			if err := json.Unmarshal([]byte(tagsJSON.String), &s.Tags); err != nil {
				s.Tags = []string{}
			}
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

// UpsertDocument inserts or updates a document record using ON CONFLICT.
// Returns UpsertResult with ID and action (created/updated).
func UpsertDocument(db *sql.DB, doc Document) (*UpsertResult, error) {
	dbMu.Lock()
	defer dbMu.Unlock()

	metadataJSON, err := json.Marshal(doc.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	tagsJSON, err := json.Marshal(doc.Tags)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tags: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Check if document exists before insert
	var existingID int64
	err = db.QueryRow(
		"SELECT id FROM documents WHERE type = ? AND project = ? AND category = ? AND title = ?",
		doc.Type, doc.Project, doc.Category, doc.Title,
	).Scan(&existingID)
	isUpdate := err == nil

	_, err = db.Exec(`
		INSERT INTO documents (type, project, category, title, content, notes, metadata, tags, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(type, project, category, title)
		DO UPDATE SET content = excluded.content, notes = excluded.notes, metadata = excluded.metadata, tags = excluded.tags, updated_at = excluded.updated_at
	`, doc.Type, doc.Project, doc.Category, doc.Title, doc.Content, doc.Notes, string(metadataJSON), string(tagsJSON), now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert document: %w", err)
	}

	if err := RebuildFTS(db); err != nil {
		return nil, fmt.Errorf("failed to rebuild FTS: %w", err)
	}

	// Get the ID of the inserted/updated row
	var id int64
	err = db.QueryRow(
		"SELECT id FROM documents WHERE type = ? AND project = ? AND category = ? AND title = ?",
		doc.Type, doc.Project, doc.Category, doc.Title,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("failed to get document ID: %w", err)
	}

	action := "created"
	if isUpdate {
		action = "updated"
	}

	return &UpsertResult{ID: id, Action: action}, nil
}

// GetDocument returns a full Document by ID. Returns nil, nil if not found.
func GetDocument(db *sql.DB, id int64) (*Document, error) {
	var doc Document
	var metadataJSON sql.NullString
	var tagsJSON sql.NullString
	var notes sql.NullString

	err := db.QueryRow(`
		SELECT id, type, project, category, title, content, notes, metadata, tags, created_at, updated_at
		FROM documents WHERE id = ?
	`, id).Scan(&doc.ID, &doc.Type, &doc.Project, &doc.Category, &doc.Title, &doc.Content, &notes, &metadataJSON, &tagsJSON, &doc.CreatedAt, &doc.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}

	if notes.Valid {
		doc.Notes = notes.String
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

// QueryDocuments queries documents with optional filters (type, project, category, FTS, tags).
// Returns DocumentSummary (no content, no metadata) to conserve tokens.
func QueryDocuments(db *sql.DB, docType string, projects []string, category, ftsQuery string, tags []string, limit int) ([]DocumentSummary, error) {
	limit = ClampLimit(limit, 10, 500)

	var query string
	var args []interface{}

	if ftsQuery != "" {
		// FTS5 query
		query = `
			SELECT d.id, d.type, d.project, d.category, d.title, d.tags, d.updated_at
			FROM documents d
			JOIN documents_fts fts ON d.id = fts.rowid
			WHERE fts.documents_fts MATCH ?
		`
		args = append(args, FtsEscape(ftsQuery))

		if docType != "" {
			query += " AND d.type = ?"
			args = append(args, docType)
		}
		query += projectFilter("d.", projects, &args)
		if category != "" {
			query += " AND d.category = ?"
			args = append(args, category)
		}

		query += " LIMIT ?"
		args = append(args, limit)
	} else {
		// Standard SQL query
		query = "SELECT id, type, project, category, title, tags, updated_at FROM documents"

		whereClause := ""
		if docType != "" {
			if whereClause != "" {
				whereClause += " AND "
			}
			whereClause += "type = ?"
			args = append(args, docType)
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
		if category != "" {
			if whereClause != "" {
				whereClause += " AND "
			}
			whereClause += "category = ?"
			args = append(args, category)
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

		if err := rows.Scan(&summary.ID, &summary.Type, &summary.Project, &summary.Category, &summary.Title, &tagsJSON, &summary.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan document summary: %w", err)
		}

		if tagsJSON.Valid {
			if err := json.Unmarshal([]byte(tagsJSON.String), &summary.Tags); err != nil {
				summary.Tags = []string{}
			}
		}

		// Filter by requested tags (all must match)
		if len(tags) > 0 {
			tagSet := make(map[string]bool)
			for _, t := range summary.Tags {
				tagSet[t] = true
			}
			match := true
			for _, t := range tags {
				if !tagSet[t] {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}

		summaries = append(summaries, summary)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating documents: %w", err)
	}

	return summaries, nil
}

// DeleteDocument deletes a document by ID.
func DeleteDocument(db *sql.DB, id int64) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	_, err := db.Exec("DELETE FROM documents WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
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
func SearchDocuments(db *sql.DB, query, docType string, projects []string, limit int) ([]DocumentSummary, error) {
	limit = ClampLimit(limit, 10, 500)

	// If query has no searchable tokens (only punctuation/wildcards), fall back to list mode
	if !hasSearchableTokens(query) {
		return QueryDocuments(db, docType, projects, "", "", nil, limit)
	}

	escapedQuery := FtsEscape(query)

	ftQuery := `
		SELECT d.id, d.type, d.project, d.category, d.title, d.tags, d.updated_at
		FROM documents d
		JOIN documents_fts fts ON d.id = fts.rowid
		WHERE fts.documents_fts MATCH ?
	`
	args := []interface{}{escapedQuery}

	if docType != "" {
		ftQuery += " AND d.type = ?"
		args = append(args, docType)
	}

	ftQuery += projectFilter("d.", projects, &args)

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
			query += " project = ?"
			args = append(args, projects[0])
		} else {
			placeholders := make([]string, len(projects))
			for i, p := range projects {
				placeholders[i] = "?"
				args = append(args, p)
			}
			query += " project IN (" + strings.Join(placeholders, ",") + ")"
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
