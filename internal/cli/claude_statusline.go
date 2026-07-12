package cli

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dangernoodle-io/mcpkit/host/claudecode/statusline"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/store"
)

// ouroborosStatuslineProvider returns the statusline.StatuslineProvider
// backing `claude statusline` (OU-272), replacing the old top-level
// `ouroboros statusline` command. It derives the project from the Claude
// Code statusLine payload's cwd (falling back to os.Getwd when the
// payload carries none — e.g. a direct CLI invocation outside the plugin),
// then renders the same KB/backlog line the old ANSI/plain formatters
// produced, as Segments Render colors via termenv instead of raw ANSI.
// Returning (nil, nil) when both totals are zero matches the old
// print-nothing behavior; sessionID is unused today (ouroboros scopes by
// project, not session).
func ouroborosStatuslineProvider() statusline.StatuslineProvider {
	return statusline.StatuslineProviderFunc(func(_ context.Context, payload statusline.Payload, _ string) ([]statusline.Segment, error) {
		var segments []statusline.Segment
		err := withDB(func(db *sql.DB) error {
			segs, err := statuslineSegmentsForPayload(db, payload)
			if err != nil {
				return err
			}
			segments = segs
			return nil
		})
		return segments, err
	})
}

// statuslineProject resolves the REGISTERED ouroboros project (backlog
// project row) for a statusline payload, or nil (aggregate — the "entire
// status" render across every project, restoring the fallback regressed
// by the OU-272 refactor). An empty payload.Cwd always aggregates (no
// os.Getwd fallback); the marketplace hub repo never resolves (OU-283's
// guard — it's a repo, never a work project).
//
// Two topologies are supported, tried in order as candidates:
//  1. single-.claude workspace with subprojects: payload.Workspace.ProjectDir
//     is the workspace root; a cwd under it resolves via the first path
//     segment of cwd relative to ProjectDir (the subproject dir name).
//  2. multi-repo: cwd resolves via its nearest git root's base name.
//
// The first candidate that is a REGISTERED project wins (a single
// backlog.GetProjectByName lookup per candidate, so the caller can reuse
// the returned *Project's ID without a second lookup); if neither candidate
// is registered (or there are none), the result aggregates.
func statuslineProject(db *sql.DB, payload statusline.Payload) *backlog.Project {
	cwd := payload.Cwd
	if cwd == "" {
		return nil
	}

	gitRoot := findGitRoot(cwd)
	if gitRoot != "" && isMarketplaceRepo(gitRoot) {
		return nil
	}

	var candidates []string

	if pd := payload.Workspace.ProjectDir; pd != "" {
		if rel, err := filepath.Rel(pd, cwd); err == nil &&
			rel != "." && !strings.HasPrefix(rel, "..") {
			seg := strings.SplitN(rel, string(os.PathSeparator), 2)[0]
			if seg != "" && seg != "." {
				candidates = append(candidates, seg)
			}
		}
	}

	if gitRoot != "" {
		candidates = append(candidates, filepath.Base(gitRoot))
	}

	for _, c := range candidates {
		if c == "" {
			continue
		}
		if p, err := backlog.GetProjectByName(db, c); err == nil {
			return p
		}
	}

	return nil
}

// statuslineSegmentsForPayload runs the KB/backlog count queries for
// payload's resolved project and renders them as Segments, or (nil, nil)
// when both totals are zero.
func statuslineSegmentsForPayload(db *sql.DB, payload statusline.Payload) ([]statusline.Segment, error) {
	proj := statuslineProject(db, payload)

	var project string
	var projects []string
	if proj != nil {
		project = proj.Name
		projects = []string{project}
	}

	kbCounts, err := store.CountDocumentsByType(db, projects)
	if err != nil {
		return nil, fmt.Errorf("claude statusline: count KB documents: %w", err)
	}

	backlogFilter := backlog.ItemFilter{}
	status := "open"
	backlogFilter.Status = &status

	if proj != nil {
		backlogFilter.ProjectIDs = []int64{proj.ID}
	}

	backlogCounts, err := backlog.CountItemsByPriority(db, backlogFilter)
	if err != nil {
		return nil, fmt.Errorf("claude statusline: count backlog items: %w", err)
	}

	kbTotal := sumKBCounts(kbCounts)
	blTotal := sumBacklogCounts(backlogCounts)
	if kbTotal == 0 && blTotal == 0 {
		return nil, nil
	}

	return buildStatuslineSegments(project, kbTotal, kbCounts, blTotal, backlogCounts), nil
}

func sumKBCounts(counts []store.TypeCount) int {
	total := 0
	for _, tc := range counts {
		total += tc.Count
	}
	return total
}

func sumBacklogCounts(counts []backlog.PriorityCount) int {
	total := 0
	for _, pc := range counts {
		total += pc.Count
	}
	return total
}

// buildStatuslineSegments renders the same visible text the old
// formatStatuslineANSI/formatStatuslinePlain produced — "ouroboros:
// [project] KB <total> (<counts>) | BL <total> open (<counts>)" — as
// Segments; Render (termenv) owns the color-profile degradation the old
// code hand-rolled via raw ANSI escapes.
func buildStatuslineSegments(
	project string, kbTotal int, kbCounts []store.TypeCount,
	blTotal int, backlogCounts []backlog.PriorityCount,
) []statusline.Segment {
	segs := []statusline.Segment{{Text: "ouroboros:", Dim: true}}

	if project != "" {
		segs = append(segs, statusline.Segment{Text: " [" + project + "]", Dim: true})
	}

	segs = append(segs, statusline.Segment{Text: " KB " + strconv.Itoa(kbTotal) + kbTypesSuffix(kbCounts)})
	segs = append(segs, statusline.Segment{Text: " | ", Dim: true})
	segs = append(segs, statusline.Segment{Text: "BL " + strconv.Itoa(blTotal) + " open"})

	if len(backlogCounts) > 0 {
		segs = append(segs, statusline.Segment{Text: " ("})
		for i, pc := range backlogCounts {
			if i > 0 {
				segs = append(segs, statusline.Segment{Text: " "})
			}
			segs = append(segs, prioritySegment(pc))
		}
		segs = append(segs, statusline.Segment{Text: ")"})
	}

	return segs
}

// kbTypesSuffix renders the "(2D 1F)" KB-type-abbreviation suffix, or ""
// when counts is empty.
func kbTypesSuffix(counts []store.TypeCount) string {
	if len(counts) == 0 {
		return ""
	}
	parts := make([]string, len(counts))
	for i, tc := range counts {
		parts[i] = strconv.Itoa(tc.Count) + typeAbbrev(tc.Type)
	}
	return " (" + strings.Join(parts, " ") + ")"
}

// prioritySegment renders one backlog priority count as a Segment colored
// per the old priorityColor semantics, expressed as termenv ANSI color
// indices: P0 red ("1"), P1 yellow ("3"), P2 cyan ("6"); anything else
// dim, uncolored.
func prioritySegment(pc backlog.PriorityCount) statusline.Segment {
	text := strconv.Itoa(pc.Count) + "×" + pc.Priority
	switch pc.Priority {
	case "P0":
		return statusline.Segment{Text: text, Color: "1"}
	case "P1":
		return statusline.Segment{Text: text, Color: "3"}
	case "P2":
		return statusline.Segment{Text: text, Color: "6"}
	default:
		return statusline.Segment{Text: text, Dim: true}
	}
}

// typeAbbrev returns the single-letter uppercase abbreviation for a KB
// document type (e.g. "decision" -> "D"), or "?" for an empty type.
func typeAbbrev(t string) string {
	if len(t) > 0 {
		return strings.ToUpper(t[:1])
	}
	return "?"
}
