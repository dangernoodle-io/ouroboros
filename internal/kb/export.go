package kb

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ExportMarkdown queries documents and builds a markdown export.
func ExportMarkdown(db *sql.DB, project, docType string) (string, error) {
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
