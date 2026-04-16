package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/store"
)

var (
	statuslineProjectFlag string
	statuslineJSONFlag    bool
)

var statuslineCmd = &cobra.Command{
	Use:   "statusline",
	Short: "Output formatted status line showing KB and backlog counts",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := store.InitDB()
		if err != nil {
			return fmt.Errorf("statusline: open database: %w", err)
		}
		defer db.Close()
		return runStatusline(cmd.OutOrStdout(), db, statuslineProjectFlag, statuslineJSONFlag)
	},
}

func init() {
	statuslineCmd.Flags().StringVar(&statuslineProjectFlag, "project", "", "Explicit project filter")
	statuslineCmd.Flags().BoolVar(&statuslineJSONFlag, "json", false, "Output JSON instead of ANSI")
}

type statuslineData struct {
	Project string            `json:"project,omitempty"`
	KB      statuslineKB      `json:"kb"`
	Backlog statuslineBacklog `json:"backlog"`
}

type statuslineKB struct {
	Total int               `json:"total"`
	Types []store.TypeCount `json:"types"`
}

type statuslineBacklog struct {
	Total int                     `json:"total"`
	Items []backlog.PriorityCount `json:"items"`
}

func runStatusline(out io.Writer, db *sql.DB, project string, jsonOutput bool) error {
	// Resolve project filter
	resolvedProject := project
	if project == "" {
		// Auto-detect from cwd
		cwd, err := os.Getwd()
		if err == nil {
			gitPath := filepath.Join(cwd, ".git")
			if _, err := os.Stat(gitPath); err == nil {
				resolvedProject = filepath.Base(cwd)
			}
		}
	}

	// Fetch KB counts
	var projects []string
	if resolvedProject != "" {
		projects = []string{resolvedProject}
	}

	kbCounts, err := store.CountDocumentsByType(db, projects)
	if err != nil {
		return fmt.Errorf("statusline: count KB documents: %w", err)
	}

	// Fetch backlog counts
	backlogFilter := backlog.ItemFilter{}
	status := "open"
	backlogFilter.Status = &status

	if resolvedProject != "" {
		p, err := backlog.GetProjectByName(db, resolvedProject)
		if err == nil {
			backlogFilter.ProjectIDs = []int64{p.ID}
		} else {
			// Project not in backlog — use sentinel ID to return 0 items
			backlogFilter.ProjectIDs = []int64{-1}
		}
	}

	backlogCounts, err := backlog.CountItemsByPriority(db, backlogFilter)
	if err != nil {
		return fmt.Errorf("statusline: count backlog items: %w", err)
	}

	// If both empty, print nothing
	kbTotal := 0
	for _, tc := range kbCounts {
		kbTotal += tc.Count
	}

	blTotal := 0
	for _, pc := range backlogCounts {
		blTotal += pc.Count
	}

	if kbTotal == 0 && blTotal == 0 {
		return nil
	}

	// Format and output
	data := statuslineData{
		KB: statuslineKB{
			Total: kbTotal,
			Types: kbCounts,
		},
		Backlog: statuslineBacklog{
			Total: blTotal,
			Items: backlogCounts,
		},
	}

	if resolvedProject != "" {
		data.Project = resolvedProject
	}

	if jsonOutput {
		jsonBytes, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("statusline: marshal JSON: %w", err)
		}
		fmt.Fprintln(out, string(jsonBytes))
		return nil
	}

	// ANSI output
	line := formatStatuslineANSI(data)
	fmt.Fprintln(out, line)
	return nil
}

func formatStatuslineANSI(data statuslineData) string {
	var sb strings.Builder

	// Prefix with project name in dim
	sb.WriteString("\033[2m")
	sb.WriteString("ouroboros:")
	sb.WriteString("\033[0m")
	sb.WriteString(" KB ")

	// KB total in white (default)
	fmt.Fprintf(&sb, "%d", data.KB.Total)

	// Project filter in dim if present
	if data.Project != "" {
		sb.WriteString(" \033[2m[")
		sb.WriteString(data.Project)
		sb.WriteString("]\033[0m")
	}

	// Type abbreviations
	if len(data.KB.Types) > 0 {
		sb.WriteString(" (")
		for i, tc := range data.KB.Types {
			if i > 0 {
				sb.WriteString(" ")
			}
			fmt.Fprintf(&sb, "%d%s", tc.Count, typeAbbrev(tc.Type))
		}
		sb.WriteString(")")
	}

	// Separator in dim
	sb.WriteString(" \033[2m|\033[0m ")

	// Backlog section
	sb.WriteString("BL ")
	fmt.Fprintf(&sb, "%d", data.Backlog.Total)
	sb.WriteString(" open")

	// Priority counts with colors
	if len(data.Backlog.Items) > 0 {
		sb.WriteString(" (")
		for i, pc := range data.Backlog.Items {
			if i > 0 {
				sb.WriteString(" ")
			}
			color := priorityColor(pc.Priority)
			sb.WriteString(color)
			fmt.Fprintf(&sb, "%d×%s", pc.Count, pc.Priority)
			sb.WriteString("\033[0m")
		}
		sb.WriteString(")")
	}

	return sb.String()
}

func typeAbbrev(t string) string {
	if len(t) > 0 {
		return strings.ToUpper(t[:1])
	}
	return "?"
}

func priorityColor(p string) string {
	switch p {
	case "P0":
		return "\033[31m" // red
	case "P1":
		return "\033[33m" // yellow
	case "P2":
		return "\033[36m" // cyan
	default:
		return "\033[2m" // dim
	}
}
