package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// dbMu serializes write operations to avoid SQLITE_BUSY under concurrent MCP requests.
var dbMu sync.Mutex

// Document represents a single knowledge base entry with unified schema.
type Document struct {
	ID        int64             `json:"id"`
	Type      string            `json:"type"`
	Project   string            `json:"project"`
	Category  string            `json:"category,omitempty"`
	Title     string            `json:"title"`
	Content   string            `json:"content,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Tags      []string          `json:"tags,omitempty"`
	CreatedAt string            `json:"created_at"`
	UpdatedAt string            `json:"updated_at"`
}

// DocumentSummary is a compact representation without content/metadata for list queries.
type DocumentSummary struct {
	ID        int64    `json:"id"`
	Type      string   `json:"type"`
	Project   string   `json:"project"`
	Category  string   `json:"category,omitempty"`
	Title     string   `json:"title"`
	Tags      []string `json:"tags,omitempty"`
	UpdatedAt string   `json:"updated_at"`
}

// initDB initializes the database connection and applies schema.
// nolint:unused
func initDB() (*sql.DB, error) {
	dbPath := os.Getenv("PROJECT_KB_PATH")
	if dbPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to determine home directory: %w", err)
		}
		dbPath = filepath.Join(homeDir, ".local", "share", "ouroboros", "kb.db")
	}

	// Create parent directories
	parentDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set pragmas.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("failed to set journal mode: %w", err)
	}

	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}

	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	if err := applySchema(db); err != nil {
		return nil, err
	}

	return db, nil
}

// applySchema creates the documents table and FTS index.
func applySchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS documents (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		type       TEXT NOT NULL,
		project    TEXT NOT NULL DEFAULT '',
		category   TEXT NOT NULL DEFAULT '',
		title      TEXT NOT NULL,
		content    TEXT NOT NULL DEFAULT '',
		metadata   TEXT,
		tags       TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		UNIQUE(type, project, category, title)
	);

	CREATE VIRTUAL TABLE IF NOT EXISTS documents_fts USING fts5(
		title, content, tags,
		content=documents, content_rowid=id
	);
	`

	_, err := db.Exec(schema)
	return err
}

// rebuildFTS rebuilds the unified documents_fts FTS index.
func rebuildFTS(db *sql.DB) error {
	_, err := db.Exec("INSERT INTO documents_fts(documents_fts) VALUES('rebuild')")
	return err
}

// ftsEscape escapes a query string for FTS5 matching.
func ftsEscape(q string) string {
	return "\"" + strings.ReplaceAll(q, "\"", "\"\"") + "\""
}

// clampLimit clamps a limit to a range with a default value.
func clampLimit(limit, defaultVal, maxVal int) int {
	if limit <= 0 {
		return defaultVal
	}
	if limit > maxVal {
		return maxVal
	}
	return limit
}

// upsertDocument inserts or updates a document record using ON CONFLICT.
// Returns the ID of the inserted/updated document.
func upsertDocument(db *sql.DB, doc Document) (int64, error) {
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

	if err := rebuildFTS(db); err != nil {
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

// getDocument returns a full Document by ID. Returns nil, nil if not found.
func getDocument(db *sql.DB, id int64) (*Document, error) {
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

// queryDocuments queries documents with optional filters (type, project, category, FTS, tags).
// Returns DocumentSummary (no content, no metadata) to conserve tokens.
func queryDocuments(db *sql.DB, docType, project, category, ftsQuery string, tags []string, limit int) ([]DocumentSummary, error) {
	limit = clampLimit(limit, 50, 500)

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
		args = append(args, ftsEscape(ftsQuery))

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

// deleteDocument deletes a document by ID.
func deleteDocument(db *sql.DB, id int64) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	_, err := db.Exec("DELETE FROM documents WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}

	if err := rebuildFTS(db); err != nil {
		return fmt.Errorf("failed to rebuild FTS: %w", err)
	}

	return nil
}

// searchDocuments performs a full-text search across all documents.
// Returns DocumentSummary (no content, no metadata).
func searchDocuments(db *sql.DB, query, docType, project string, limit int) ([]DocumentSummary, error) {
	limit = clampLimit(limit, 50, 500)
	escapedQuery := ftsEscape(query)

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
