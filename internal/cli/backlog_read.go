package cli

import (
	"database/sql"
	"fmt"
	"io"
	"sort"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/query"
	"dangernoodle.io/ouroboros/internal/store"
)

// writeEdges (shared `kb get`/`backlog get` edges-sidecar renderer) lives in
// kb_render.go.

// printBacklogItemTable renders a []backlog.Item as a table (same header/
// columns as ls_backlog.go's runLSItems and the unlanded OU-324 branch's
// query_render.go printBacklogItemTable) — shared by `backlog list` and
// `backlog search`. Items in filter/list or search mode carry a real
// ProjectID (unlike an --ids fetch's result, which query.Get always zeroes
// — see internal/query/get.go's getBacklogItems), so project names are
// resolved here via a single ListProjects call rather than per-item.
func printBacklogItemTable(out io.Writer, db *sql.DB, items []backlog.Item) error {
	projects, err := backlog.ListProjects(db)
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}
	projectMap := make(map[int64]string, len(projects))
	for _, p := range projects {
		projectMap[p.ID] = p.Name
	}

	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			item.ID,
			item.Priority,
			item.Status,
			projectMap[item.ProjectID],
			item.Component,
			item.Epic,
			item.Created,
			item.Title,
		})
	}
	return printTable(out, []string{"ID", "PRIORITY", "STATUS", "PROJECT", "COMPONENT", "EPIC", "CREATED", "TITLE"}, rows)
}

// itemLess orders two backlog items the same way backlog.ListItems does:
// created-descending-then-id when sortByCreated (--sort created), else
// priority-ascending-then-id (the default). A malformed/non-"P<n>" priority
// sorts last, mirroring SQLite's CAST(...AS INTEGER) coercing unparsable
// text to 0 only for a numeric prefix — anything else is out-of-band here
// and shouldn't out-rank a valid priority.
func itemLess(a, b backlog.Item, sortByCreated bool) bool {
	if sortByCreated {
		if a.Created != b.Created {
			return a.Created > b.Created
		}
		return a.ID < b.ID
	}

	ap, aOK := backlog.ParsePriority(a.Priority)
	bp, bOK := backlog.ParsePriority(b.Priority)
	switch {
	case aOK && bOK:
		if ap != bp {
			return ap < bp
		}
	case aOK != bOK:
		return aOK
	}
	return a.ID < b.ID
}

// mergeItemsByStatus issues one query.Get call per distinct status (OU-102's
// multi-status folded in at the CLI layer, since query.Request.Status is
// single-valued), dedups the results by item id, GLOBALLY re-sorts the
// merged set by the same order backlog.ListItems applies (itemLess), and
// only then trims to base.Limit.
//
// Each per-status call already applies base.Limit individually, which is
// sufficient rather than lossy PROVIDED each per-status query.Get's SQL-side
// ordering (backlog.ListItems's "ORDER BY CAST(SUBSTR(priority,2) AS
// INTEGER), id", or "created DESC, id") agrees with itemLess's Go-side
// ordering: if an item ranks in the true global top-N across all requested
// statuses, it cannot rank worse than N-th within its own status group
// (every item that would out-rank it globally and share its status is
// already counted against that N), so it is guaranteed to survive its own
// per-status fetch. That precondition holds only for valid P0-P6 priority
// data — SQLite's CAST(SUBSTR(...) AS INTEGER) coerces a malformed
// (non-numeric) priority to 0, ranking it FIRST server-side, while itemLess
// ranks a malformed priority LAST. A malformed priority in the DB could thus
// occupy a per-status LIMIT slot and crowd out a legitimate P0 item before
// the global re-sort below ever sees it. This is enforced by write-path
// validation (internal/app/handlers_backlog_v2.go), not a DB CHECK
// constraint. What would starve later statuses even with valid data is
// applying --limit per-status and then simply CONCATENATING in status
// order without a global re-sort before trimming — that returns
// all-first-status whenever the first status alone has >= limit items.
// Re-sorting the full merged set before trimming is what this function does
// instead.
func mergeItemsByStatus(db *sql.DB, base query.Request, statuses []string) ([]backlog.Item, error) {
	seen := make(map[string]bool)
	var merged []backlog.Item
	for _, status := range statuses {
		req := base
		req.Status = status

		result, err := query.Get(db, req)
		if err != nil {
			return nil, err
		}
		for _, item := range result.Items {
			if seen[item.ID] {
				continue
			}
			seen[item.ID] = true
			merged = append(merged, item)
		}
	}

	sortByCreated := base.Sort == "created"
	sort.SliceStable(merged, func(i, j int) bool {
		return itemLess(merged[i], merged[j], sortByCreated)
	})

	// Resolve base.Limit to the same effective cap backlog.ListItems applies
	// per-status (store.ClampLimit(limit, 10, 500)) so --limit 0 behaves
	// identically for single- and multi-status requests, rather than
	// skipping the trim entirely (which let multi-status return up to
	// len(statuses)*perStatusLimit).
	effectiveLimit := store.ClampLimit(base.Limit, 10, 500)
	if len(merged) > effectiveLimit {
		merged = merged[:effectiveLimit]
	}
	return merged, nil
}

// dedupStrings returns s with duplicates removed, preserving first-seen
// order (used for --status's repeatable flag values).
func dedupStrings(s []string) []string {
	seen := make(map[string]bool, len(s))
	out := make([]string, 0, len(s))
	for _, v := range s {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// priorityMinMax mirrors ls_backlog.go's runLSItems single --priority flag
// parsing: a well-formed "P0"-"P6" value sets an exact-match min==max
// filter (query.Request.PriorityMin/Max are strings, parsed downstream by
// internal/query's buildItemFilter); anything else (empty, or malformed)
// leaves both "" — an invalid --priority value is silently ignored rather
// than erroring, matching the old `ls backlog` behavior being mirrored here.
func priorityMinMax(priority string) (string, string) {
	if priority == "" {
		return "", ""
	}
	if len(priority) == 2 && priority[0] == 'P' {
		if p, ok := backlog.ParsePriority(priority); ok && p >= 0 && p <= 6 {
			return priority, priority
		}
	}
	return "", ""
}
