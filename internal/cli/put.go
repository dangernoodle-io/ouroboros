package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/kb"
	"dangernoodle.io/ouroboros/internal/store"
)

var (
	putProjectFlag  string
	putTypeFlag     string
	putTitleFlag    string
	putContentFlag  string
	putNotesFlag    string
	putCategoryFlag string
	putTagsFlag     []string
	putStdinFlag    bool
)

var putCmd = &cobra.Command{
	Use:   "put",
	Short: "Create or update KB documents (CLI for hook integration)",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := store.InitDB()
		if err != nil {
			return fmt.Errorf("put: open database: %w", err)
		}
		defer db.Close()
		return runPut(cmd.OutOrStdout(), cmd.InOrStdin(), db, putProjectFlag, putTypeFlag, putTitleFlag, putContentFlag, putNotesFlag, putCategoryFlag, putTagsFlag, putStdinFlag)
	},
}

func init() {
	putCmd.Flags().StringVar(&putProjectFlag, "project", "", "Project name (required for flags mode)")
	putCmd.Flags().StringVar(&putTypeFlag, "type", "", "Document type: decision, fact, note, plan, relation (required for flags mode)")
	putCmd.Flags().StringVar(&putTitleFlag, "title", "", "Document title (required for flags mode)")
	putCmd.Flags().StringVar(&putContentFlag, "content", "", "Document content up to 500 chars (required for flags mode)")
	putCmd.Flags().StringVar(&putNotesFlag, "notes", "", "Document notes (optional)")
	putCmd.Flags().StringVar(&putCategoryFlag, "category", "", "Document category (optional)")
	putCmd.Flags().StringSliceVar(&putTagsFlag, "tags", []string{}, "Document tags (optional, repeatable)")
	putCmd.Flags().BoolVar(&putStdinFlag, "stdin", false, "Read JSON array or {documents:[...]} from stdin (batch mode)")
}

type kbEntry struct {
	Type     string            `json:"type"`
	Project  string            `json:"project,omitempty"`
	Category string            `json:"category,omitempty"`
	Title    string            `json:"title"`
	Content  string            `json:"content"`
	Notes    string            `json:"notes,omitempty"`
	Tags     []string          `json:"tags,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type kbBatch struct {
	Documents []kbEntry `json:"documents"`
}

type putResult struct {
	ID     int64  `json:"id"`
	Action string `json:"action"`
	Title  string `json:"title"`
}

func runPut(out io.Writer, in io.Reader, db *sql.DB, project, docType, title, content, notes, category string, tags []string, useStdin bool) error {
	if useStdin {
		return runPutStdin(out, in, db, project)
	}
	return runPutFlags(out, db, project, docType, title, content, notes, category, tags)
}

func runPutFlags(out io.Writer, db *sql.DB, project, docType, title, content, notes, category string, tags []string) error {
	if docType == "" {
		return fmt.Errorf("--type is required")
	}
	if project == "" {
		return fmt.Errorf("--project is required")
	}
	if title == "" {
		return fmt.Errorf("--title is required")
	}
	if content == "" {
		return fmt.Errorf("--content is required")
	}

	doc := store.Document{
		Type:     docType,
		Project:  project,
		Category: category,
		Title:    title,
		Content:  content,
		Notes:    notes,
		Tags:     tags,
	}

	if err := kb.ValidateDocument(doc); err != nil {
		return err
	}

	result, err := store.UpsertDocument(db, doc)
	if err != nil {
		return fmt.Errorf("put: upsert failed: %w", err)
	}

	output := putResult{
		ID:     result.ID,
		Action: result.Action,
		Title:  title,
	}
	data, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("put: marshal failed: %w", err)
	}
	fmt.Fprintln(out, string(data))
	return nil
}

func runPutStdin(out io.Writer, in io.Reader, db *sql.DB, projectFlag string) error {
	data, err := io.ReadAll(in)
	if err != nil {
		return fmt.Errorf("put: read stdin failed: %w", err)
	}

	if len(data) == 0 {
		fmt.Fprintln(out, "[]")
		return nil
	}

	// Try to unmarshal as array first
	var entries []kbEntry
	err = json.Unmarshal(data, &entries)
	if err != nil {
		// Try to unmarshal as {documents: [...]}
		var batch kbBatch
		if err2 := json.Unmarshal(data, &batch); err2 != nil {
			return fmt.Errorf("put: invalid JSON (tried array and {documents:[...]}): %w", err)
		}
		entries = batch.Documents
	}

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

		if err := kb.ValidateDocument(doc); err != nil {
			return fmt.Errorf("put: entry %d validation failed: %w", i, err)
		}
	}

	// Upsert all validated entries
	results := make([]putResult, 0, len(entries))
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
			return fmt.Errorf("put: upsert failed: %w", err)
		}

		results = append(results, putResult{
			ID:     result.ID,
			Action: result.Action,
			Title:  entry.Title,
		})
	}

	outputData, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("put: marshal results failed: %w", err)
	}
	fmt.Fprintln(out, string(outputData))
	return nil
}
