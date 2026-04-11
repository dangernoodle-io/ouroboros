package kb

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"dangernoodle.io/ouroboros/internal/store"
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

// ImportJSON unmarshals JSON data and imports it into the database.
func ImportJSON(db *sql.DB, defaultProject string, data []byte) error {
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

		doc := store.Document{
			Type:     impDoc.Type,
			Project:  project,
			Category: impDoc.Category,
			Title:    impDoc.Title,
			Content:  impDoc.Content,
			Metadata: impDoc.Metadata,
			Tags:     impDoc.Tags,
		}

		_, err := store.UpsertDocument(db, doc)
		if err != nil {
			return fmt.Errorf("failed to upsert document: %w", err)
		}
	}

	return nil
}

// Import auto-detects format and imports data into the database.
func Import(db *sql.DB, defaultProject, content string) error {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return fmt.Errorf("content is empty")
	}

	// Auto-detect JSON by checking for leading { or [
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return ImportJSON(db, defaultProject, []byte(trimmed))
	}

	return fmt.Errorf("unsupported format, use JSON")
}
