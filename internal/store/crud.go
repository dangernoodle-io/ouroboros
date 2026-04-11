package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// UpsertDocument inserts or updates a document record using ON CONFLICT.
// Returns the ID of the inserted/updated document.
func UpsertDocument(db *sql.DB, doc Document) (int64, error) {
	dbMu.Lock()
	defer dbMu.Unlock()

	metadataJSON, err := json.Marshal(doc.Metadata)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	tagsJSON, err := json.Marshal(doc.Tags)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal tags: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	_, err = db.Exec(`
		INSERT INTO documents (type, project, category, title, content, metadata, tags, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(type, project, category, title)
		DO UPDATE SET content = excluded.content, metadata = excluded.metadata, tags = excluded.tags, updated_at = excluded.updated_at
	`, doc.Type, doc.Project, doc.Category, doc.Title, doc.Content, string(metadataJSON), string(tagsJSON), now, now)
	if err != nil {
		return 0, fmt.Errorf("failed to upsert document: %w", err)
	}

	if err := RebuildFTS(db); err != nil {
		return 0, fmt.Errorf("failed to rebuild FTS: %w", err)
	}

	// Get the ID of the inserted/updated row
	var id int64
	err = db.QueryRow(
		"SELECT id FROM documents WHERE type = ? AND project = ? AND category = ? AND title = ?",
		doc.Type, doc.Project, doc.Category, doc.Title,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to get document ID: %w", err)
	}

	return id, nil
}

// GetDocument returns a full Document by ID. Returns nil, nil if not found.
func GetDocument(db *sql.DB, id int64) (*Document, error) {
	var doc Document
	var metadataJSON sql.NullString
	var tagsJSON sql.NullString

	err := db.QueryRow(`
		SELECT id, type, project, category, title, content, metadata, tags, created_at, updated_at
		FROM documents WHERE id = ?
	`, id).Scan(&doc.ID, &doc.Type, &doc.Project, &doc.Category, &doc.Title, &doc.Content, &metadataJSON, &tagsJSON, &doc.CreatedAt, &doc.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
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
func QueryDocuments(db *sql.DB, docType, project, category, ftsQuery string, tags []string, limit int) ([]DocumentSummary, error) {
	limit = ClampLimit(limit, 50, 500)

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
		if project != "" {
			query += " AND d.project = ?"
			args = append(args, project)
		}
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
		if project != "" {
			if whereClause != "" {
				whereClause += " AND "
			}
			whereClause += "project = ?"
			args = append(args, project)
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

// SearchDocuments performs a full-text search across all documents.
// Returns DocumentSummary (no content, no metadata).
func SearchDocuments(db *sql.DB, query, docType, project string, limit int) ([]DocumentSummary, error) {
	limit = ClampLimit(limit, 50, 500)
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

	if project != "" {
		ftQuery += " AND d.project = ?"
		args = append(args, project)
	}

	ftQuery += " LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(ftQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search documents: %w", err)
	}
	defer rows.Close()

	var summaries []DocumentSummary
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
