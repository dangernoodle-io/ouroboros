package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

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

// InitDB initializes the database connection and applies schema.
func InitDB() (*sql.DB, error) {
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

	if err := ApplySchema(db); err != nil {
		return nil, err
	}

	return db, nil
}

// ApplySchema creates the documents table and FTS index.
func ApplySchema(db *sql.DB) error {
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

// RebuildFTS rebuilds the unified documents_fts FTS index.
func RebuildFTS(db *sql.DB) error {
	_, err := db.Exec("INSERT INTO documents_fts(documents_fts) VALUES('rebuild')")
	return err
}

// FtsEscape escapes a query string for FTS5 matching.
func FtsEscape(q string) string {
	return "\"" + strings.ReplaceAll(q, "\"", "\"\"") + "\""
}

// ClampLimit clamps a limit to a range with a default value.
func ClampLimit(limit, defaultVal, maxVal int) int {
	if limit <= 0 {
		return defaultVal
	}
	if limit > maxVal {
		return maxVal
	}
	return limit
}
