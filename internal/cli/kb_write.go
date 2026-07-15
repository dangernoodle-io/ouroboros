package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/kb"
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

// kbWriteCmd is the KB write command (OU-334: moved verbatim from the
// former bare `kb` action — this is now a noun-first `kb write`, a
// breaking CLI change; bare `kb` no longer writes).
var kbWriteCmd = &cobra.Command{
	Use:   "write",
	Short: "Create or update KB documents (CLI for hook integration)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return withDB(func(db *sql.DB) error {
			return runPut(cmd.OutOrStdout(), cmd.InOrStdin(), db, putProjectFlag, putTypeFlag, putTitleFlag, putContentFlag, putNotesFlag, putCategoryFlag, putTagsFlag, putStdinFlag)
		})
	},
}

func init() {
	kbWriteCmd.Flags().StringVar(&putProjectFlag, "project", "", "Project name (required for flags mode)")
	kbWriteCmd.Flags().StringVar(&putTypeFlag, "type", "", "Document type: decision, fact, note, plan, relation (required for flags mode)")
	kbWriteCmd.Flags().StringVar(&putTitleFlag, "title", "", "Document title (required for flags mode)")
	kbWriteCmd.Flags().StringVar(&putContentFlag, "content", "", "Document content up to 500 chars (required for flags mode)")
	kbWriteCmd.Flags().StringVar(&putNotesFlag, "notes", "", "Document notes (optional)")
	kbWriteCmd.Flags().StringVar(&putCategoryFlag, "category", "", "Document category (optional)")
	kbWriteCmd.Flags().StringSliceVar(&putTagsFlag, "tags", []string{}, "Document tags (optional, repeatable)")
	kbWriteCmd.Flags().BoolVar(&putStdinFlag, "stdin", false, "Read JSON array or {documents:[...]} from stdin (batch mode)")
}

type kbBatch struct {
	Documents []kb.Entry `json:"documents"`
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

	entries := []kb.Entry{
		{
			Type:     docType,
			Project:  project,
			Category: category,
			Title:    title,
			Content:  content,
			Notes:    notes,
			Tags:     tags,
		},
	}

	results, err := kb.WriteBatch(db, entries, "")
	if err != nil {
		return fmt.Errorf("put: batch write failed: %w", err)
	}

	// results should have exactly one element from one-element batch
	if len(results) > 0 {
		output := putResult{
			ID:     results[0].ID,
			Action: results[0].Action,
			Title:  results[0].Title,
		}
		data, err := json.Marshal(output)
		if err != nil {
			return fmt.Errorf("put: marshal failed: %w", err)
		}
		fmt.Fprintln(out, string(data))
	}
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
	var entries []kb.Entry
	err = json.Unmarshal(data, &entries)
	if err != nil {
		// Try to unmarshal as {documents: [...]}
		var batch kbBatch
		if err2 := json.Unmarshal(data, &batch); err2 != nil {
			return fmt.Errorf("put: invalid JSON (tried array and {documents:[...]}): %w", err)
		}
		entries = batch.Documents
	}

	results, err := kb.WriteBatch(db, entries, projectFlag)
	if err != nil {
		// WriteBatch returns partial results on error, still output them
		outputData, _ := json.Marshal(results)
		fmt.Fprintln(out, string(outputData))
		return fmt.Errorf("put: batch write failed: %w", err)
	}

	outputData, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("put: marshal results failed: %w", err)
	}
	fmt.Fprintln(out, string(outputData))
	return nil
}
