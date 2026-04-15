package store

import (
	"database/sql"
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
	Notes     string            `json:"notes,omitempty"`
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

// migrations defines the schema evolution with versioned SQL.
var migrations = []struct {
	version int
	sql     string
}{
	{
		version: 1,
		sql: `CREATE TABLE IF NOT EXISTS documents (
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
		);`,
	},
	{
		version: 2,
		sql: `CREATE TABLE IF NOT EXISTS config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			prefix TEXT NOT NULL UNIQUE,
			created TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS items (
			id TEXT PRIMARY KEY,
			project_id INTEGER NOT NULL REFERENCES projects(id),
			seq INTEGER NOT NULL,
			priority TEXT NOT NULL,
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'open',
			created TEXT NOT NULL,
			updated TEXT NOT NULL,
			UNIQUE(project_id, seq)
		);

		CREATE TABLE IF NOT EXISTS plans (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER REFERENCES projects(id),
			item_id TEXT REFERENCES items(id),
			title TEXT NOT NULL,
			content TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'draft',
			created TEXT NOT NULL,
			updated TEXT NOT NULL
		);`,
	},
	{
		version: 3,
		sql:     `ALTER TABLE documents ADD COLUMN project_id INTEGER REFERENCES projects(id);`,
	},
	{
		version: 4,
		sql:     `ALTER TABLE documents ADD COLUMN notes TEXT NOT NULL DEFAULT '';`,
	},
	{
		version: 5,
		sql:     `ALTER TABLE items ADD COLUMN notes TEXT NOT NULL DEFAULT '';`,
	},
	{
		version: 6,
		sql:     `ALTER TABLE items ADD COLUMN component TEXT NOT NULL DEFAULT '';`,
	},
}

// ApplySchema applies all pending migrations to the database.
func ApplySchema(db *sql.DB) error {
	// Create schema_migrations table to track applied migrations
	createMigrationsTable := `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	);`

	if _, err := db.Exec(createMigrationsTable); err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	// Get the maximum applied version
	var maxVersion int
	err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&maxVersion)
	if err != nil {
		return fmt.Errorf("failed to query max migration version: %w", err)
	}

	// Apply pending migrations
	for _, m := range migrations {
		if m.version <= maxVersion {
			continue
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %d: %w", m.version, err)
		}

		if _, err := tx.Exec(m.sql); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed to execute migration %d: %w", m.version, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)",
			m.version, time.Now().UTC().Format(time.RFC3339)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed to record migration %d: %w", m.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %d: %w", m.version, err)
		}
	}

	return nil
}

// RebuildFTS rebuilds the unified documents_fts FTS index.
func RebuildFTS(db *sql.DB) error {
	_, err := db.Exec("INSERT INTO documents_fts(documents_fts) VALUES('rebuild')")
	return err
}

// FtsEscape converts a query string into FTS5 implicit AND syntax.
// Splits on whitespace, strips FTS5 meta chars from each token, and joins with spaces.
func FtsEscape(q string) string {
	tokens := strings.Fields(q)
	var result []string

	for _, token := range tokens {
		// Strip FTS5 meta characters: " * ( ) : - ^ +
		filtered := strings.Map(func(r rune) rune {
			switch r {
			case '"', '*', '(', ')', ':', '-', '^', '+':
				return -1 // drop this rune
			default:
				return r
			}
		}, token)

		if filtered != "" {
			result = append(result, "\""+filtered+"\"")
		}
	}

	if len(result) == 0 {
		return ""
	}

	return strings.Join(result, " ")
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
