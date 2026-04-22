package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/store"
)

func printTable(out io.Writer, header []string, rows [][]string) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(header, "\t"))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	return w.Flush()
}

func printJSON(out io.Writer, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	fmt.Fprintln(out, string(data))
	return nil
}

func formatItemDetail(out io.Writer, item *backlog.Item, projectName string) {
	header := fmt.Sprintf("%s  [%s]  [%s]  %s", item.ID, item.Priority, item.Status, projectName)
	fmt.Fprintln(out, header)

	if item.Component != "" {
		fmt.Fprintf(out, "Component: %s\n", item.Component)
	}
	fmt.Fprintf(out, "Title:     %s\n", item.Title)

	if item.Description != "" {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Description:")
		fmt.Fprintln(out, item.Description)
	}

	if item.Notes != "" {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Notes:")
		fmt.Fprintln(out, item.Notes)
	}
}

func formatKBDetail(out io.Writer, doc *store.Document) {
	header := fmt.Sprintf("%d  [%s]  %s", doc.ID, doc.Type, doc.Project)
	fmt.Fprintln(out, header)

	if doc.Category != "" {
		fmt.Fprintf(out, "Category: %s\n", doc.Category)
	}
	fmt.Fprintf(out, "Title:    %s\n", doc.Title)

	if len(doc.Tags) > 0 {
		fmt.Fprintf(out, "Tags:     %s\n", strings.Join(doc.Tags, ", "))
	}

	if doc.Content != "" {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Content:")
		fmt.Fprintln(out, doc.Content)
	}

	if doc.Notes != "" {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Notes:")
		fmt.Fprintln(out, doc.Notes)
	}
}

func formatPlanDetail(out io.Writer, plan *backlog.Plan, projectName string) {
	header := fmt.Sprintf("%d  [%s]  %s", plan.ID, plan.Status, projectName)
	fmt.Fprintln(out, header)
	fmt.Fprintf(out, "Title: %s\n", plan.Title)

	if plan.Content != "" {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Content:")
		fmt.Fprintln(out, plan.Content)
	}
}
