package kb

import (
	"context"
	"database/sql"
	"fmt"

	"dangernoodle.io/ouroboros/internal/store"
)

// WriteBatch validates and writes a batch of KB entries atomically.
// Validates all entries first; first validation failure aborts with an error.
// All writes succeed or none persist (transaction rollback on any error).
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

	// Write all validated entries atomically
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

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

		result, err := store.UpsertDocumentTx(tx, doc)
		if err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("upsert failed: %w", err)
		}

		results = append(results, PutResult{
			ID:     result.ID,
			Action: result.Action,
			Title:  entry.Title,
		})
	}

	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	if err := store.RebuildFTS(db); err != nil {
		return nil, fmt.Errorf("failed to rebuild FTS: %w", err)
	}

	return results, nil
}
