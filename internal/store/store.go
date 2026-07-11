package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

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
	SessionID string            `json:"session_id,omitempty"`
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
	Score     float64  `json:"score,omitempty"`
}

// InitDB initializes the database connection and applies schema. path must
// be non-empty; callers resolve the default/fallback path via
// config.Load().DBPath before calling InitDB.
func InitDB(path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("InitDB: empty db path")
	}
	dbPath := path

	// Create parent directories
	parentDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// _txlock=immediate: db.Begin() acquires SQLite's write lock immediately
	// rather than deferring it to the first write statement. This lets
	// roadmap.Mutate serialize a full load-mutate-save cycle against
	// concurrent writers, including another ouroboros process on the same
	// file (SQLITE_BUSY + busy_timeout below handles lock contention).
	db, err := sql.Open("sqlite", dbPath+"?_txlock=immediate")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(1)

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
	{
		version: 7,
		sql: `ALTER TABLE documents ADD COLUMN session_id TEXT;

CREATE INDEX IF NOT EXISTS idx_documents_session_id ON documents(session_id) WHERE session_id IS NOT NULL;

UPDATE documents SET session_id = json_extract(metadata, '$.session_id') WHERE session_id IS NULL AND json_extract(metadata, '$.session_id') IS NOT NULL;`,
	},
	{
		version: 8,
		sql:     `CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_name_ci ON projects(LOWER(name));`,
	},
	{
		version: 9,
		sql: `CREATE TABLE IF NOT EXISTS item_id_aliases (
    old_id TEXT PRIMARY KEY,
    new_id TEXT NOT NULL REFERENCES items(id) ON UPDATE CASCADE ON DELETE CASCADE,
    renamed_at TEXT NOT NULL
);`,
	},
	{
		version: 10,
		sql: `CREATE VIRTUAL TABLE IF NOT EXISTS items_fts USING fts5(
    title, description, notes,
    content=items, content_rowid=rowid
);

CREATE TRIGGER IF NOT EXISTS items_ai AFTER INSERT ON items BEGIN
    INSERT INTO items_fts(rowid, title, description, notes) VALUES (new.rowid, new.title, new.description, new.notes);
END;

CREATE TRIGGER IF NOT EXISTS items_ad AFTER DELETE ON items BEGIN
    INSERT INTO items_fts(items_fts, rowid, title, description, notes) VALUES ('delete', old.rowid, old.title, old.description, old.notes);
END;

CREATE TRIGGER IF NOT EXISTS items_au AFTER UPDATE ON items BEGIN
    INSERT INTO items_fts(items_fts, rowid, title, description, notes) VALUES ('delete', old.rowid, old.title, old.description, old.notes);
    INSERT INTO items_fts(rowid, title, description, notes) VALUES (new.rowid, new.title, new.description, new.notes);
END;

INSERT INTO items_fts(rowid, title, description, notes) SELECT rowid, title, description, notes FROM items;`,
	},
	{
		version: 11,
		sql:     `CREATE UNIQUE INDEX IF NOT EXISTS idx_documents_roadmap_singleton ON documents(project) WHERE type='roadmap';`,
	},
	{
		version: 12,
		sql:     `ALTER TABLE items ADD COLUMN epic TEXT NOT NULL DEFAULT '';`,
	},
	{
		version: 13,
		sql: `CREATE TABLE IF NOT EXISTS edges (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    source_type TEXT NOT NULL,
    source_id   TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id   TEXT NOT NULL,
    label       TEXT NOT NULL,
    project_id  INTEGER REFERENCES projects(id) ON DELETE CASCADE,
    created_at  TEXT NOT NULL,
    UNIQUE(source_type, source_id, target_type, target_id, label)
);

CREATE INDEX IF NOT EXISTS idx_edges_source ON edges(source_type, source_id);
CREATE INDEX IF NOT EXISTS idx_edges_target ON edges(target_type, target_id);`,
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
			m.version, nowRFC3339()); err != nil {
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

// FtsEscape converts a query string into FTS5 implicit AND syntax. Splits the
// query on FTS5/unicode61 token boundaries (any rune that isn't a letter or
// digit — the same boundary unicode61's default tokenizer splits on, e.g.
// hyphen, underscore, whitespace, and other punctuation), quotes each token
// as its own phrase, and joins with spaces (implicit AND in FTS5 MATCH
// syntax). A single hyphenated word like "old-title" is indexed by unicode61
// as two tokens ("old","title"), so it must be split the same way here —
// collapsing it into one "oldtitle" phrase would never match anything.
func FtsEscape(q string) string {
	tokens := strings.FieldsFunc(q, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	result := make([]string, 0, len(tokens))
	for _, token := range tokens {
		// Defensive: unreachable given the FieldsFunc boundary above splits
		// on every non-letter/digit rune, so a token can never contain a
		// quote. Kept in case a future tokenizer-boundary change allows it.
		escaped := strings.ReplaceAll(token, "\"", "\"\"")
		result = append(result, "\""+escaped+"\"")
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
