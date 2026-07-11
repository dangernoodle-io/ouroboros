package kb

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"dangernoodle.io/ouroboros/internal/edges"
	"dangernoodle.io/ouroboros/internal/store"
)

// rebuildFTS is a test seam over store.RebuildFTS: tests swap this for a
// counter to assert a batch call rebuilds the FTS index exactly once, not
// once per row.
var rebuildFTS = store.RebuildFTS

// validateEntries validates every create/upsert entry, returning the first
// validation failure (index-annotated). No DB interaction.
func validateEntries(entries []Entry, projectFlag string) error {
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
			return fmt.Errorf("entry %d validation failed: %w", i, err)
		}
	}
	return nil
}

// dedupEntries applies intra-batch dedup: last-wins by (type, project,
// category, title). No DB interaction.
func dedupEntries(entries []Entry, projectFlag string) []Entry {
	seen := make(map[string]int)
	for i, entry := range entries {
		project := entry.Project
		if project == "" {
			project = projectFlag
		}
		key := fmt.Sprintf("%s|%s|%s|%s", entry.Type, project, entry.Category, entry.Title)
		if prior, ok := seen[key]; ok {
			fmt.Fprintf(os.Stderr, "[ouroboros] put: dropped intra-batch duplicate at index %d (key: %s)\n", prior, key)
		}
		seen[key] = i
	}
	deduped := make([]Entry, 0, len(seen))
	added := make(map[string]bool)
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		project := entry.Project
		if project == "" {
			project = projectFlag
		}
		key := fmt.Sprintf("%s|%s|%s|%s", entry.Type, project, entry.Category, entry.Title)
		if !added[key] && seen[key] == i {
			deduped = append([]Entry{entry}, deduped...)
			added[key] = true
		}
	}
	return deduped
}

// writeEntriesTx upserts already-validated/deduped entries within an
// existing transaction. Callers own commit/rollback and FTS rebuild.
func writeEntriesTx(tx *sql.Tx, entries []Entry, projectFlag string) ([]PutResult, error) {
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
			return nil, fmt.Errorf("upsert failed: %w", err)
		}

		// [[Title]] autolinks: item-id-shaped titles resolve against
		// backlog items, otherwise against a same-project KB title.
		// Best-effort — unresolved titles are silently skipped, never a
		// write failure.
		if _, err := edges.AutolinkKB(tx, result.ID, project, entry.Content); err != nil {
			return nil, fmt.Errorf("autolink failed: %w", err)
		}

		pr := PutResult{
			ID:     result.ID,
			Action: result.Action,
			Title:  entry.Title,
		}
		if result.Conflict {
			pr.Conflict = true
			if len(result.PreviousContent) > 100 {
				pr.PreviousExcerpt = result.PreviousContent[:100]
			} else {
				pr.PreviousExcerpt = result.PreviousContent
			}
		}
		results = append(results, pr)
	}
	return results, nil
}

// validateUpdates validates every id-addressed update, returning the first
// validation failure (index-annotated). No DB interaction.
func validateUpdates(updates []EntryUpdate) error {
	for i, u := range updates {
		if err := ValidateEntryUpdate(u); err != nil {
			return fmt.Errorf("update %d validation failed: %w", i, err)
		}
	}
	return nil
}

// updateEntriesTx applies already-validated id-addressed partial updates
// within an existing transaction. Callers own commit/rollback and FTS
// rebuild.
func updateEntriesTx(tx *sql.Tx, updates []EntryUpdate) ([]PutResult, error) {
	results := make([]PutResult, 0, len(updates))
	for _, u := range updates {
		fields := store.UpdateDocumentFields{
			Type:     u.Type,
			Project:  u.Project,
			Category: u.Category,
			Title:    u.Title,
			Content:  u.Content,
			Notes:    u.Notes,
			Tags:     u.Tags,
			Metadata: u.Metadata,
		}

		doc, err := store.UpdateDocumentTx(tx, u.ID, fields)
		if err != nil {
			return nil, fmt.Errorf("update failed for id %d: %w", u.ID, err)
		}

		// Re-run [[Title]] autolinks only when content changed — content is
		// what's scanned for links, so an untouched content field has
		// nothing new to (re-)resolve. Best-effort, mirrors writeEntriesTx.
		if u.Content != nil {
			if _, err := edges.AutolinkKB(tx, doc.ID, doc.Project, doc.Content); err != nil {
				return nil, fmt.Errorf("autolink failed for id %d: %w", u.ID, err)
			}
		}

		results = append(results, PutResult{
			ID:     doc.ID,
			Action: "updated",
			Title:  doc.Title,
		})
	}
	return results, nil
}

// WriteBatch validates and writes a batch of KB entries atomically.
// Validates all entries first; first validation failure aborts with an error.
// All writes succeed or none persist (transaction rollback on any error).
func WriteBatch(db *sql.DB, entries []Entry, projectFlag string) ([]PutResult, error) {
	if err := validateEntries(entries, projectFlag); err != nil {
		return nil, err
	}
	entries = dedupEntries(entries, projectFlag)

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	results, err := writeEntriesTx(tx, entries, projectFlag)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	if err := rebuildFTS(db); err != nil {
		return nil, fmt.Errorf("failed to rebuild FTS: %w", err)
	}

	return results, nil
}

// UpdateBatch validates and applies a batch of id-addressed partial KB
// document updates atomically (all succeed or none persist). This is the
// retitle-without-duplicate path: unlike WriteBatch's natural-key upsert
// (type+project+category+title), an update targets an existing row by id,
// so changing the title updates that row in place instead of inserting a
// new one under the new key.
//
// No production caller currently invokes UpdateBatch directly — the kb tool
// handler dispatches through WriteAndUpdateBatch, which folds create and
// update entries into one transaction. UpdateBatch is kept as the
// standalone, update-only entry point symmetric with WriteBatch, for a
// future update-only CLI surface (e.g. `ouroboros kb update`) that has no
// create side to fold in.
func UpdateBatch(db *sql.DB, updates []EntryUpdate) ([]PutResult, error) {
	if err := validateUpdates(updates); err != nil {
		return nil, err
	}

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	results, err := updateEntriesTx(tx, updates)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	if err := rebuildFTS(db); err != nil {
		return nil, fmt.Errorf("failed to rebuild FTS: %w", err)
	}

	return results, nil
}

// WriteAndUpdateBatch processes a single kb tool call's entries[] — creates
// (id absent) and updates (id present) — as ONE atomic transaction: every
// create and update in the call commits together, or none do. This is what
// a KB-integrity tool requires: without it, a batch like
// [{new doc A}, {id: <nonexistent>}] would commit A via a separate
// create-tx and only then fail the update, leaving A persisted under a
// "failed" tool result. FTS is rebuilt once, after the single commit.
func WriteAndUpdateBatch(db *sql.DB, entries []Entry, updates []EntryUpdate, projectFlag string) (creates, updated []PutResult, err error) {
	if err := validateEntries(entries, projectFlag); err != nil {
		return nil, nil, err
	}
	if err := validateUpdates(updates); err != nil {
		return nil, nil, err
	}
	entries = dedupEntries(entries, projectFlag)

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	creates, err = writeEntriesTx(tx, entries, projectFlag)
	if err != nil {
		_ = tx.Rollback()
		return nil, nil, err
	}

	updated, err = updateEntriesTx(tx, updates)
	if err != nil {
		_ = tx.Rollback()
		return nil, nil, err
	}

	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return nil, nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	if err := rebuildFTS(db); err != nil {
		return nil, nil, fmt.Errorf("failed to rebuild FTS: %w", err)
	}

	return creates, updated, nil
}
