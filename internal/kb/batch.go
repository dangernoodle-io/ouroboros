package kb

import (
	"database/sql"
	"fmt"

	"dangernoodle.io/ouroboros/internal/store"
)

// WriteBatch validates and writes a batch of KB entries. Validates all entries first;
// first validation failure aborts the entire batch with an error. On write, returns
// partial results if any write fails.
func WriteBatch(db *sql.DB, entries []Entry, projectFlag string) ([]PutResult, error) {
	// Validate all entries first
	for i, entry := range entries {
		project := entry.Project
		if project == "" {
			project = projectFlag
		}

		doc := store.Document{
			Type:     entry.Type,
			Project:  project,
			Category: entry.Category,
			Title:    entry.Title,
			Content:  entry.Content,
			Notes:    entry.Notes,
			Tags:     entry.Tags,
			Metadata: entry.Metadata,
		}

		if err := ValidateDocument(doc); err != nil {
			return nil, fmt.Errorf("entry %d validation failed: %w", i, err)
		}
	}

	// Write all validated entries
	results := make([]PutResult, 0, len(entries))
	for _, entry := range entries {
		project := entry.Project
		if project == "" {
			project = projectFlag
		}

		doc := store.Document{
			Type:     entry.Type,
			Project:  project,
			Category: entry.Category,
			Title:    entry.Title,
			Content:  entry.Content,
			Notes:    entry.Notes,
			Tags:     entry.Tags,
			Metadata: entry.Metadata,
		}

		result, err := store.UpsertDocument(db, doc)
		if err != nil {
			// Return error but include partial results
			return results, fmt.Errorf("upsert failed: %w", err)
		}

		results = append(results, PutResult{
			ID:     result.ID,
			Action: result.Action,
			Title:  entry.Title,
		})
	}

	return results, nil
}
