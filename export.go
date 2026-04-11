package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ImportData represents the structure for batch imports.
type ImportData struct {
	Decisions []ImportDecision `json:"decisions"`
	Facts     []ImportFact     `json:"facts"`
	Relations []ImportRelation `json:"relations"`
	Notes     []ImportNote     `json:"notes"`
}

// ImportDecision represents a decision for import.
type ImportDecision struct {
	Project   string   `json:"project"`
	Summary   string   `json:"summary"`
	Rationale string   `json:"rationale,omitempty"`
	Tags      []string `json:"tags,omitempty"`
}

// ImportFact represents a fact for import.
type ImportFact struct {
	Project  string `json:"project"`
	Category string `json:"category"`
	Key      string `json:"key"`
	Value    string `json:"value"`
}

// ImportRelation represents a relation for import.
type ImportRelation struct {
	SourceProject string `json:"source_project"`
	Source        string `json:"source"`
	TargetProject string `json:"target_project"`
	Target        string `json:"target"`
	RelationType  string `json:"relation_type"`
	Description   string `json:"description,omitempty"`
}

// ImportNote represents a note for import.
type ImportNote struct {
	Project  string   `json:"project"`
	Category string   `json:"category"`
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	Tags     []string `json:"tags,omitempty"`
}

// queryDecisionsForExport returns full decisions (with rationale) for export.
func queryDecisionsForExport(db *sql.DB, project string) ([]Decision, error) {
	query := "SELECT id, project, summary, rationale, tags, created_at FROM decisions"
	var args []interface{}
	if project != "" {
		query += " WHERE project = ?"
		args = append(args, project)
	}
	query += " LIMIT 500"
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var decisions []Decision
	for rows.Next() {
		var d Decision
		var tagsJSON string
		if err := rows.Scan(&d.ID, &d.Project, &d.Summary, &d.Rationale, &tagsJSON, &d.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(tagsJSON), &d.Tags); err != nil {
			d.Tags = []string{}
		}
		decisions = append(decisions, d)
	}
	return decisions, rows.Err()
}

// queryNotesForExport returns full notes (with body) for export.
func queryNotesForExport(db *sql.DB, project string) ([]Note, error) {
	query := "SELECT id, project, category, title, body, tags, created_at, updated_at FROM notes"
	var args []interface{}
	if project != "" {
		query += " WHERE project = ?"
		args = append(args, project)
	}
	query += " LIMIT 500"
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var notes []Note
	for rows.Next() {
		var n Note
		var tagsJSON sql.NullString
		if err := rows.Scan(&n.ID, &n.Project, &n.Category, &n.Title, &n.Body, &tagsJSON, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		if tagsJSON.Valid {
			_ = json.Unmarshal([]byte(tagsJSON.String), &n.Tags)
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

// exportMarkdown queries all tables and builds a markdown export.
func exportMarkdown(db *sql.DB, project string) (string, error) {
	decisions, err := queryDecisionsForExport(db, project)
	if err != nil {
		return "", fmt.Errorf("failed to query decisions: %w", err)
	}

	facts, err := queryFacts(db, project, "", "", "", 500)
	if err != nil {
		return "", fmt.Errorf("failed to query facts: %w", err)
	}

	relations, err := queryRelations(db, project, "", "", 500)
	if err != nil {
		return "", fmt.Errorf("failed to query relations: %w", err)
	}

	notes, err := queryNotesForExport(db, project)
	if err != nil {
		return "", fmt.Errorf("failed to query notes: %w", err)
	}

	// Build markdown
	var sb strings.Builder

	sb.WriteString("# Knowledge Base Export\n\n")

	projectLabel := "All Projects"
	if project != "" {
		projectLabel = project
	}
	fmt.Fprintf(&sb, "Project: %s | Generated: %s\n\n", projectLabel, time.Now().UTC().Format(time.RFC3339))

	// Decisions section
	sb.WriteString("## Decisions\n\n")
	if len(decisions) == 0 {
		sb.WriteString("_No decisions found._\n\n")
	} else {
		for _, d := range decisions {
			fmt.Fprintf(&sb, "### #%d [%s]\n", d.ID, d.Project)
			fmt.Fprintf(&sb, "**Summary:** %s\n", d.Summary)
			if d.Rationale != "" {
				fmt.Fprintf(&sb, "**Rationale:** %s\n", d.Rationale)
			}
			if len(d.Tags) > 0 {
				fmt.Fprintf(&sb, "**Tags:** %s\n", strings.Join(d.Tags, ", "))
			}
			fmt.Fprintf(&sb, "**Created:** %s\n\n", d.CreatedAt)
		}
	}

	// Facts section
	sb.WriteString("## Facts\n\n")
	if len(facts) == 0 {
		sb.WriteString("_No facts found._\n\n")
	} else {
		sb.WriteString("| Project | Category | Key | Value |\n")
		sb.WriteString("|---------|----------|-----|-------|\n")
		for _, f := range facts {
			fmt.Fprintf(&sb, "| %s | %s | %s | %s |\n", f.Project, f.Category, f.Key, f.Value)
		}
		sb.WriteString("\n")
	}

	// Relations section
	sb.WriteString("## Relations\n\n")
	if len(relations) == 0 {
		sb.WriteString("_No relations found._\n\n")
	} else {
		sb.WriteString("| Source | Relation | Target | Description |\n")
		sb.WriteString("|--------|----------|--------|-------------|\n")
		for _, r := range relations {
			source := fmt.Sprintf("%s/%s", r.SourceProject, r.Source)
			target := fmt.Sprintf("%s/%s", r.TargetProject, r.Target)
			fmt.Fprintf(&sb, "| %s | %s | %s | %s |\n", source, r.RelationType, target, r.Description)
		}
		sb.WriteString("\n")
	}

	// Notes section
	sb.WriteString("## Notes\n\n")
	if len(notes) == 0 {
		sb.WriteString("_No notes found._\n\n")
	} else {
		for _, n := range notes {
			fmt.Fprintf(&sb, "### %s [%s/%s]\n", n.Title, n.Project, n.Category)
			if len(n.Tags) > 0 {
				fmt.Fprintf(&sb, "**Tags:** %s\n", strings.Join(n.Tags, ", "))
			}
			fmt.Fprintf(&sb, "**Updated:** %s\n\n", n.UpdatedAt)
			fmt.Fprintf(&sb, "%s\n\n", n.Body)
		}
	}

	return sb.String(), nil
}

// importJSON unmarshals JSON data and imports it into the database.
func importJSON(db *sql.DB, defaultProject string, data []byte) error {
	var importData ImportData
	if err := json.Unmarshal(data, &importData); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	// Import decisions
	for _, d := range importData.Decisions {
		project := d.Project
		if project == "" {
			project = defaultProject
		}
		if project == "" {
			return fmt.Errorf("decision missing project and no default provided")
		}

		_, err := insertDecision(db, project, d.Summary, d.Rationale, d.Tags)
		if err != nil {
			return fmt.Errorf("failed to insert decision: %w", err)
		}
	}

	// Import facts
	for _, f := range importData.Facts {
		project := f.Project
		if project == "" {
			project = defaultProject
		}
		if project == "" {
			return fmt.Errorf("fact missing project and no default provided")
		}

		_, err := upsertFact(db, project, f.Category, f.Key, f.Value)
		if err != nil {
			return fmt.Errorf("failed to upsert fact: %w", err)
		}
	}

	// Import relations
	for _, r := range importData.Relations {
		if r.SourceProject == "" || r.TargetProject == "" {
			return fmt.Errorf("relation missing project fields")
		}

		_, err := insertRelation(db, r.SourceProject, r.Source, r.TargetProject, r.Target, r.RelationType, r.Description)
		if err != nil {
			return fmt.Errorf("failed to insert relation: %w", err)
		}
	}

	// Import notes
	for _, n := range importData.Notes {
		project := n.Project
		if project == "" {
			project = defaultProject
		}
		if project == "" {
			return fmt.Errorf("note missing project and no default provided")
		}
		_, err := upsertNote(db, project, n.Category, n.Title, n.Body, n.Tags)
		if err != nil {
			return fmt.Errorf("failed to upsert note: %w", err)
		}
	}

	return nil
}

// importData auto-detects format and imports data into the database.
// nolint:unparam
func importData(db *sql.DB, defaultProject, content string) error {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return fmt.Errorf("content is empty")
	}

	// Auto-detect JSON by checking for leading { or [
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return importJSON(db, defaultProject, []byte(trimmed))
	}

	return fmt.Errorf("unsupported format, use JSON")
}
