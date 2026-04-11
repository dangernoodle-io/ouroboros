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

// Decision represents an architectural or strategic decision.
type Decision struct {
	ID        int64    `json:"id"`
	Project   string   `json:"project"`
	Summary   string   `json:"summary"`
	Rationale string   `json:"rationale,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	CreatedAt string   `json:"created_at"`
}

// Fact represents a documented fact or piece of information.
type Fact struct {
	ID        int64  `json:"id"`
	Project   string `json:"project"`
	Category  string `json:"category"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Relation represents a relationship between entities.
type Relation struct {
	ID            int64  `json:"id"`
	SourceProject string `json:"source_project"`
	Source        string `json:"source"`
	TargetProject string `json:"target_project"`
	Target        string `json:"target"`
	RelationType  string `json:"relation_type"`
	Description   string `json:"description,omitempty"`
	CreatedAt     string `json:"created_at"`
}

// Note represents a longer-form knowledge base entry.
type Note struct {
	ID        int64    `json:"id"`
	Project   string   `json:"project"`
	Category  string   `json:"category"`
	Title     string   `json:"title"`
	Body      string   `json:"body,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

// NoteSummary is a compact representation without body for list queries.
type NoteSummary struct {
	ID        int64    `json:"id"`
	Project   string   `json:"project"`
	Category  string   `json:"category"`
	Title     string   `json:"title"`
	Tags      []string `json:"tags,omitempty"`
	UpdatedAt string   `json:"updated_at"`
}

// DecisionSummary is a compact representation without rationale for list queries.
type DecisionSummary struct {
	ID        int64    `json:"id"`
	Project   string   `json:"project"`
	Summary   string   `json:"summary"`
	Tags      []string `json:"tags,omitempty"`
	CreatedAt string   `json:"created_at"`
}

// SearchResult combines search results from multiple tables.
type SearchResult struct {
	Decisions []DecisionSummary `json:"decisions,omitempty"`
	Facts     []Fact            `json:"facts,omitempty"`
	Notes     []NoteSummary     `json:"notes,omitempty"`
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

// applySchema creates all necessary tables and virtual tables.
func applySchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS decisions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project TEXT NOT NULL,
		summary TEXT NOT NULL,
		rationale TEXT,
		tags TEXT,
		created_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS facts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project TEXT NOT NULL,
		category TEXT NOT NULL,
		key TEXT NOT NULL,
		value TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		UNIQUE(project, category, key)
	);

	CREATE TABLE IF NOT EXISTS relations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source_project TEXT NOT NULL,
		source TEXT NOT NULL,
		target_project TEXT NOT NULL,
		target TEXT NOT NULL,
		relation_type TEXT NOT NULL,
		description TEXT,
		created_at TEXT NOT NULL
	);

	CREATE VIRTUAL TABLE IF NOT EXISTS decisions_fts USING fts5(
		summary, rationale, tags,
		content=decisions, content_rowid=id
	);

	CREATE VIRTUAL TABLE IF NOT EXISTS facts_fts USING fts5(
		category, key, value,
		content=facts, content_rowid=id
	);

	CREATE TABLE IF NOT EXISTS notes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project TEXT NOT NULL,
		category TEXT NOT NULL,
		title TEXT NOT NULL,
		body TEXT NOT NULL,
		tags TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		UNIQUE(project, category, title)
	);

	CREATE VIRTUAL TABLE IF NOT EXISTS notes_fts USING fts5(
		title, body, tags,
		content=notes, content_rowid=id
	);
	`

	_, err := db.Exec(schema)
	return err
}

// rebuildFTS rebuilds the FTS index for the specified table.
func rebuildFTS(db *sql.DB, table string) error {
	query := fmt.Sprintf("INSERT INTO %s_fts(%s_fts) VALUES('rebuild')", table, table)
	_, err := db.Exec(query)
	return err
}

// ftsEscape escapes a query string for FTS5 matching.
func ftsEscape(q string) string {
	return "\"" + strings.ReplaceAll(q, "\"", "\"\"") + "\""
}

// insertDecision inserts a new decision record.
func insertDecision(db *sql.DB, project, summary, rationale string, tags []string) (int64, error) {
	dbMu.Lock()
	defer dbMu.Unlock()
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal tags: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	result, err := db.Exec(
		"INSERT INTO decisions (project, summary, rationale, tags, created_at) VALUES (?, ?, ?, ?, ?)",
		project, summary, rationale, string(tagsJSON), now,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to insert decision: %w", err)
	}

	if err := rebuildFTS(db, "decisions"); err != nil {
		return 0, fmt.Errorf("failed to rebuild FTS: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert ID: %w", err)
	}

	return id, nil
}

// getDecision returns a full Decision with rationale. Returns nil, nil if not found.
func getDecision(db *sql.DB, id int64) (*Decision, error) {
	var d Decision
	var tagsJSON string

	err := db.QueryRow(
		"SELECT id, project, summary, rationale, tags, created_at FROM decisions WHERE id = ?",
		id,
	).Scan(&d.ID, &d.Project, &d.Summary, &d.Rationale, &tagsJSON, &d.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get decision: %w", err)
	}

	if err := json.Unmarshal([]byte(tagsJSON), &d.Tags); err != nil {
		d.Tags = []string{}
	}

	return &d, nil
}

// queryDecisions queries decisions with optional filters.
// Returns DecisionSummary (no rationale) to conserve tokens.
// nolint:unparam
func queryDecisions(db *sql.DB, project string, tags []string, ftsQuery string, limit int) ([]DecisionSummary, error) {
	limit = clampLimit(limit, 50, 500)

	var query string
	var args []interface{}

	if ftsQuery != "" {
		query = `
			SELECT d.id, d.project, d.summary, d.tags, d.created_at
			FROM decisions d
			JOIN decisions_fts fts ON d.id = fts.rowid
			WHERE fts.decisions_fts MATCH ?
		`
		args = append(args, ftsEscape(ftsQuery))

		if project != "" {
			query += " AND d.project = ?"
			args = append(args, project)
		}

		query += " LIMIT ?"
		args = append(args, limit)
	} else {
		query = "SELECT id, project, summary, tags, created_at FROM decisions"

		if project != "" {
			query += " WHERE project = ?"
			args = append(args, project)
		}

		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query decisions: %w", err)
	}
	defer rows.Close()

	var decisions []DecisionSummary
	for rows.Next() {
		var d DecisionSummary
		var tagsJSON string

		if err := rows.Scan(&d.ID, &d.Project, &d.Summary, &tagsJSON, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan decision: %w", err)
		}

		if err := json.Unmarshal([]byte(tagsJSON), &d.Tags); err != nil {
			d.Tags = []string{}
		}

		// Filter by requested tags.
		if len(tags) > 0 {
			tagSet := make(map[string]bool)
			for _, t := range d.Tags {
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

		decisions = append(decisions, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating decisions: %w", err)
	}

	return decisions, nil
}

// deleteDecision deletes a decision by ID.
func deleteDecision(db *sql.DB, id int64) error {
	dbMu.Lock()
	defer dbMu.Unlock()
	_, err := db.Exec("DELETE FROM decisions WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete decision: %w", err)
	}

	if err := rebuildFTS(db, "decisions"); err != nil {
		return fmt.Errorf("failed to rebuild FTS: %w", err)
	}

	return nil
}

// upsertFact inserts or updates a fact record.
func upsertFact(db *sql.DB, project, category, key, value string) (int64, error) {
	dbMu.Lock()
	defer dbMu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339)

	// Use INSERT OR REPLACE to implement upsert
	result, err := db.Exec(`
		INSERT INTO facts (project, category, key, value, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(project, category, key)
		DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, project, category, key, value, now, now)

	if err != nil {
		return 0, fmt.Errorf("failed to upsert fact: %w", err)
	}

	if err := rebuildFTS(db, "facts"); err != nil {
		return 0, fmt.Errorf("failed to rebuild FTS: %w", err)
	}

	// Get the ID of the inserted/updated row
	var id int64
	err = db.QueryRow(
		"SELECT id FROM facts WHERE project = ? AND category = ? AND key = ?",
		project, category, key,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to get fact ID: %w", err)
	}

	_ = result // Suppress unused variable warning

	return id, nil
}

// queryFacts queries facts with optional filters.
// nolint:unparam
func queryFacts(db *sql.DB, project, category, key, ftsQuery string, limit int) ([]Fact, error) {
	limit = clampLimit(limit, 50, 500)

	var query string
	var args []interface{}

	if ftsQuery != "" {
		// FTS5 query
		query = `
			SELECT f.id, f.project, f.category, f.key, f.value, f.created_at, f.updated_at
			FROM facts f
			JOIN facts_fts fts ON f.id = fts.rowid
			WHERE fts.facts_fts MATCH ?
		`
		args = append(args, ftsEscape(ftsQuery))

		if project != "" {
			query += " AND f.project = ?"
			args = append(args, project)
		}
		if category != "" {
			query += " AND f.category = ?"
			args = append(args, category)
		}
		if key != "" {
			query += " AND f.key = ?"
			args = append(args, key)
		}

		query += " LIMIT ?"
		args = append(args, limit)
	} else {
		// Standard SQL query
		query = "SELECT id, project, category, key, value, created_at, updated_at FROM facts"

		whereClause := ""
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
		if key != "" {
			if whereClause != "" {
				whereClause += " AND "
			}
			whereClause += "key = ?"
			args = append(args, key)
		}

		if whereClause != "" {
			query += " WHERE " + whereClause
		}

		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query facts: %w", err)
	}
	defer rows.Close()

	var facts []Fact
	for rows.Next() {
		var f Fact
		if err := rows.Scan(&f.ID, &f.Project, &f.Category, &f.Key, &f.Value, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan fact: %w", err)
		}
		facts = append(facts, f)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating facts: %w", err)
	}

	return facts, nil
}

// deleteFact deletes a fact by project, category, and key.
func deleteFact(db *sql.DB, project, category, key string) error {
	dbMu.Lock()
	defer dbMu.Unlock()
	_, err := db.Exec(
		"DELETE FROM facts WHERE project = ? AND category = ? AND key = ?",
		project, category, key,
	)
	if err != nil {
		return fmt.Errorf("failed to delete fact: %w", err)
	}

	if err := rebuildFTS(db, "facts"); err != nil {
		return fmt.Errorf("failed to rebuild FTS: %w", err)
	}

	return nil
}

// insertRelation inserts a new relation record.
// nolint:unparam
func insertRelation(db *sql.DB, sp, s, tp, t, relType, desc string) (int64, error) {
	dbMu.Lock()
	defer dbMu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339)

	result, err := db.Exec(`
		INSERT INTO relations (source_project, source, target_project, target, relation_type, description, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, sp, s, tp, t, relType, desc, now)

	if err != nil {
		return 0, fmt.Errorf("failed to insert relation: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert ID: %w", err)
	}

	return id, nil
}

// queryRelations queries relations with optional filters.
func queryRelations(db *sql.DB, project, entity, relType string, limit int) ([]Relation, error) {
	limit = clampLimit(limit, 50, 500)

	var query string
	var args []interface{}

	query = `
		SELECT id, source_project, source, target_project, target, relation_type, description, created_at
		FROM relations
		WHERE 1=1
	`

	if entity != "" {
		if project != "" {
			query += " AND ((source = ? AND source_project = ?) OR (target = ? AND target_project = ?))"
			args = append(args, entity, project, entity, project)
		} else {
			query += " AND (source = ? OR target = ?)"
			args = append(args, entity, entity)
		}
	} else if project != "" {
		query += " AND (source_project = ? OR target_project = ?)"
		args = append(args, project, project)
	}

	if relType != "" {
		query += " AND relation_type = ?"
		args = append(args, relType)
	}

	query += " LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query relations: %w", err)
	}
	defer rows.Close()

	var relations []Relation
	for rows.Next() {
		var r Relation
		if err := rows.Scan(&r.ID, &r.SourceProject, &r.Source, &r.TargetProject, &r.Target, &r.RelationType, &r.Description, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan relation: %w", err)
		}
		relations = append(relations, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating relations: %w", err)
	}

	return relations, nil
}

// deleteRelation deletes a relation by ID.
func deleteRelation(db *sql.DB, id int64) error {
	dbMu.Lock()
	defer dbMu.Unlock()
	_, err := db.Exec("DELETE FROM relations WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete relation: %w", err)
	}
	return nil
}

// upsertNote inserts or updates a note record.
func upsertNote(db *sql.DB, project, category, title, body string, tags []string) (int64, error) {
	dbMu.Lock()
	defer dbMu.Unlock()
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal tags: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Use INSERT OR REPLACE to implement upsert
	result, err := db.Exec(`
		INSERT INTO notes (project, category, title, body, tags, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project, category, title)
		DO UPDATE SET body = excluded.body, tags = excluded.tags, updated_at = excluded.updated_at
	`, project, category, title, body, string(tagsJSON), now, now)

	if err != nil {
		return 0, fmt.Errorf("failed to upsert note: %w", err)
	}

	if err := rebuildFTS(db, "notes"); err != nil {
		return 0, fmt.Errorf("failed to rebuild FTS: %w", err)
	}

	// Get the ID of the inserted/updated row
	var id int64
	err = db.QueryRow(
		"SELECT id FROM notes WHERE project = ? AND category = ? AND title = ?",
		project, category, title,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to get note ID: %w", err)
	}

	_ = result // Suppress unused variable warning

	return id, nil
}

// queryNotes queries notes with optional filters.
// Returns NoteSummary (NO body field). If ftsQuery set, uses FTS5 MATCH.
// Otherwise SELECT with optional WHERE filters. Filter tags in Go.
// Default limit 50, cap 500.
// nolint:unparam
func queryNotes(db *sql.DB, project, category, ftsQuery string, tags []string, limit int) ([]NoteSummary, error) {
	limit = clampLimit(limit, 50, 500)

	var query string
	var args []interface{}

	if ftsQuery != "" {
		// FTS5 query
		query = `
			SELECT n.id, n.project, n.category, n.title, n.tags, n.updated_at
			FROM notes n
			JOIN notes_fts fts ON n.id = fts.rowid
			WHERE fts.notes_fts MATCH ?
		`
		args = append(args, ftsEscape(ftsQuery))

		if project != "" {
			query += " AND n.project = ?"
			args = append(args, project)
		}
		if category != "" {
			query += " AND n.category = ?"
			args = append(args, category)
		}

		query += " LIMIT ?"
		args = append(args, limit)
	} else {
		// Standard SQL query
		query = "SELECT id, project, category, title, tags, updated_at FROM notes"

		whereClause := ""
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
		return nil, fmt.Errorf("failed to query notes: %w", err)
	}
	defer rows.Close()

	var summaries []NoteSummary
	for rows.Next() {
		var n NoteSummary
		var tagsJSON sql.NullString

		if err := rows.Scan(&n.ID, &n.Project, &n.Category, &n.Title, &tagsJSON, &n.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan note: %w", err)
		}

		if tagsJSON.Valid {
			if err := json.Unmarshal([]byte(tagsJSON.String), &n.Tags); err != nil {
				n.Tags = []string{}
			}
		}

		// Filter by requested tags
		if len(tags) > 0 {
			tagSet := make(map[string]bool)
			for _, t := range n.Tags {
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

		summaries = append(summaries, n)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating notes: %w", err)
	}

	return summaries, nil
}

// getNote returns full Note WITH body. Returns nil, nil if not found.
func getNote(db *sql.DB, id int64) (*Note, error) {
	var n Note
	var tagsJSON sql.NullString

	err := db.QueryRow(
		"SELECT id, project, category, title, body, tags, created_at, updated_at FROM notes WHERE id = ?",
		id,
	).Scan(&n.ID, &n.Project, &n.Category, &n.Title, &n.Body, &tagsJSON, &n.CreatedAt, &n.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get note: %w", err)
	}

	if tagsJSON.Valid {
		if err := json.Unmarshal([]byte(tagsJSON.String), &n.Tags); err != nil {
			n.Tags = []string{}
		}
	}

	return &n, nil
}

// deleteNote deletes a note by ID, then rebuilds FTS.
func deleteNote(db *sql.DB, id int64) error {
	dbMu.Lock()
	defer dbMu.Unlock()
	_, err := db.Exec("DELETE FROM notes WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete note: %w", err)
	}

	if err := rebuildFTS(db, "notes"); err != nil {
		return fmt.Errorf("failed to rebuild FTS: %w", err)
	}

	return nil
}

// searchAll performs a full-text search across decisions and facts.
func searchAll(db *sql.DB, query, project string, limit int) (*SearchResult, error) {
	limit = clampLimit(limit, 50, 500)
	escapedQuery := ftsEscape(query)

	result := &SearchResult{}

	// Search decisions (summaries only — no rationale).
	decisionQuery := `
		SELECT d.id, d.project, d.summary, d.tags, d.created_at
		FROM decisions d
		JOIN decisions_fts fts ON d.id = fts.rowid
		WHERE fts.decisions_fts MATCH ?
	`
	decisionArgs := []interface{}{escapedQuery}

	if project != "" {
		decisionQuery += " AND d.project = ?"
		decisionArgs = append(decisionArgs, project)
	}

	decisionQuery += " LIMIT ?"
	decisionArgs = append(decisionArgs, limit)

	rows, err := db.Query(decisionQuery, decisionArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to search decisions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var d DecisionSummary
		var tagsJSON string

		if err := rows.Scan(&d.ID, &d.Project, &d.Summary, &tagsJSON, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan decision: %w", err)
		}

		if err := json.Unmarshal([]byte(tagsJSON), &d.Tags); err != nil {
			d.Tags = []string{}
		}

		result.Decisions = append(result.Decisions, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating decisions: %w", err)
	}

	// Search facts
	factQuery := `
		SELECT f.id, f.project, f.category, f.key, f.value, f.created_at, f.updated_at
		FROM facts f
		JOIN facts_fts fts ON f.id = fts.rowid
		WHERE fts.facts_fts MATCH ?
	`
	factArgs := []interface{}{escapedQuery}

	if project != "" {
		factQuery += " AND f.project = ?"
		factArgs = append(factArgs, project)
	}

	factQuery += " LIMIT ?"
	factArgs = append(factArgs, limit)

	rows, err = db.Query(factQuery, factArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to search facts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var f Fact
		if err := rows.Scan(&f.ID, &f.Project, &f.Category, &f.Key, &f.Value, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan fact: %w", err)
		}
		result.Facts = append(result.Facts, f)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating facts: %w", err)
	}

	// Search notes (summaries only)
	noteQuery := `
		SELECT n.id, n.project, n.category, n.title, n.tags, n.updated_at
		FROM notes n
		JOIN notes_fts fts ON n.id = fts.rowid
		WHERE fts.notes_fts MATCH ?
	`
	noteArgs := []interface{}{escapedQuery}
	if project != "" {
		noteQuery += " AND n.project = ?"
		noteArgs = append(noteArgs, project)
	}
	noteQuery += " LIMIT ?"
	noteArgs = append(noteArgs, limit)

	rows, err = db.Query(noteQuery, noteArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to search notes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var n NoteSummary
		var tagsJSON sql.NullString
		if err := rows.Scan(&n.ID, &n.Project, &n.Category, &n.Title, &tagsJSON, &n.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan note: %w", err)
		}
		if tagsJSON.Valid {
			if err := json.Unmarshal([]byte(tagsJSON.String), &n.Tags); err != nil {
				n.Tags = []string{}
			}
		}
		result.Notes = append(result.Notes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating notes: %w", err)
	}

	return result, nil
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
