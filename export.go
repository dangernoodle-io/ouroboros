package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ImportDocument represents a document for import.
type ImportDocument struct {
	Type     string            `json:"type"`
	Project  string            `json:"project,omitempty"`
	Category string            `json:"category,omitempty"`
	Title    string            `json:"title"`
	Content  string            `json:"content,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Tags     []string          `json:"tags,omitempty"`
}

// ImportData represents the structure for batch imports.
type ImportData struct {
	Documents []ImportDocument `json:"documents"`
}

// exportMarkdown queries documents and builds a markdown export.
func exportMarkdown(db *sql.DB, project, docType string) (string, error) {
	// Query documents by type and/or project
	var query string
	var args []interface{}

	query = "SELECT id, type, project, category, title, content, tags, updated_at FROM documents WHERE 1=1"

	if docType != "" {
		query += " AND type = ?"
		args = append(args, docType)
	}

	if project != "" {
		query += " AND project = ?"
		args = append(args, project)
	}

	query += " ORDER BY type, updated_at DESC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return "", fmt.Errorf("failed to query documents: %w", err)
	}
	defer rows.Close()

	type docRow struct {
		ID        int64
		Type      string
		Project   string
		Category  string
		Title     string
		Content   string
		TagsJSON  sql.NullString
		UpdatedAt string
	}

	var docs []docRow
	for rows.Next() {
		var d docRow
		if err := rows.Scan(&d.ID, &d.Type, &d.Project, &d.Category, &d.Title, &d.Content, &d.TagsJSON, &d.UpdatedAt); err != nil {
			return "", fmt.Errorf("failed to scan document: %w", err)
		}
		docs = append(docs, d)
	}

	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("error iterating documents: %w", err)
	}

	// Build markdown
	var sb strings.Builder

	sb.WriteString("# Knowledge Base Export\n\n")

	projectLabel := "All Projects"
	if project != "" {
		projectLabel = project
	}
	typeLabel := ""
	if docType != "" {
		typeLabel = fmt.Sprintf(" | Type: %s", docType)
	}
	fmt.Fprintf(&sb, "Project: %s%s | Generated: %s\n\n", projectLabel, typeLabel, time.Now().UTC().Format(time.RFC3339))

	if len(docs) == 0 {
		sb.WriteString("_No documents found._\n")
		return sb.String(), nil
	}

	// Group by type
	docsByType := make(map[string][]docRow)
	for _, d := range docs {
		docsByType[d.Type] = append(docsByType[d.Type], d)
	}

	// Output by type
	for _, t := range []string{"decision", "fact", "relation", "note"} {
		typeDocs := docsByType[t]
		if len(typeDocs) == 0 {
			continue
		}

		fmt.Fprintf(&sb, "## %s\n\n", strings.ToUpper(t[:1])+t[1:])

		for _, d := range typeDocs {
			var tags []string
			if d.TagsJSON.Valid {
				if err := json.Unmarshal([]byte(d.TagsJSON.String), &tags); err != nil {
					tags = []string{}
				}
			}

			fmt.Fprintf(&sb, "### %s [%s/%s]\n", d.Title, d.Project, d.Category)
			fmt.Fprintf(&sb, "**Type:** %s\n", d.Type)
			if len(tags) > 0 {
				fmt.Fprintf(&sb, "**Tags:** %s\n", strings.Join(tags, ", "))
			}
			fmt.Fprintf(&sb, "**Updated:** %s\n\n", d.UpdatedAt)
			if d.Content != "" {
				fmt.Fprintf(&sb, "%s\n\n", d.Content)
			}
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

	for _, impDoc := range importData.Documents {
		project := impDoc.Project
		if project == "" {
			project = defaultProject
		}
		if project == "" {
			return fmt.Errorf("document missing project and no default provided")
		}

		doc := Document{
			Type:     impDoc.Type,
			Project:  project,
			Category: impDoc.Category,
			Title:    impDoc.Title,
			Content:  impDoc.Content,
			Metadata: impDoc.Metadata,
			Tags:     impDoc.Tags,
		}

		_, err := upsertDocument(db, doc)
		if err != nil {
			return fmt.Errorf("failed to upsert document: %w", err)
		}
	}

	return nil
}

// importData auto-detects format and imports data into the database.
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
