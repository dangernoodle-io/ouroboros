package query

import (
	"database/sql"
	"fmt"
	"strconv"

	"dangernoodle.io/ouroboros/internal/backlog"
)

// parsePriority parses a "P0"-"P6" priority string, verbatim (including
// error text) from internal/app's former parsePriority. Duplicated rather
// than shared with internal/app's write-handler copy (handlers_backlog_v2.go
// also calls it, transactionally) so this extraction touches zero write-path
// code, per the OU-322 build spec.
func parsePriority(s string) (int, error) {
	if len(s) != 2 || s[0] != 'P' {
		return 0, fmt.Errorf("invalid priority: %s (expected P0-P6)", s)
	}
	n, err := strconv.Atoi(string(s[1]))
	if err != nil || n < 0 || n > 6 {
		return 0, fmt.Errorf("invalid priority: %s (expected P0-P6)", s)
	}
	return n, nil
}

// resolveProjects resolves each project name to its id, verbatim from
// internal/app's former resolveProjects (see parsePriority's comment on why
// this is a duplicate, not a shared call, of the write-path's copy).
func resolveProjects(db *sql.DB, names []string) ([]int64, error) {
	ids := make([]int64, 0, len(names))
	for _, name := range names {
		proj, err := backlog.GetProjectByName(db, name)
		if err != nil {
			return nil, err
		}
		ids = append(ids, proj.ID)
	}
	return ids, nil
}

// buildItemFilter is the former internal/app parseItemFilterV2, ported
// verbatim (including its divergence-note comment below) to build a
// backlog.ItemFilter from a Request's shared filter fields — the single
// filter builder for domain=backlog reads (Get list mode, Search), now
// shared by both instead of being an app-private helper.
//
// Divergence note (OU-1, dark path only, carried forward from the original
// parseItemFilter/parseItemFilterV2): the old handler set f.Component
// whenever the "component" arg key was merely *present* (even ""), which let
// an explicit component="" filter match items with no component — every
// other field here (priority_min/max, status, since, sort, epic) required
// non-empty. A typed Go string field can't distinguish "key present but
// empty" from "key absent" after JSON unmarshaling, so this applies the same
// non-empty check uniformly (matching the majority behavior); an explicit
// component="" filter is not reproducible here. Not exercised by any
// existing test; noted for cutover review — tracked for OU-3 cutover.
func buildItemFilter(db *sql.DB, req Request) (backlog.ItemFilter, error) {
	var f backlog.ItemFilter

	if len(req.Projects) > 0 {
		ids, err := resolveProjects(db, req.Projects)
		if err != nil {
			return f, err
		}
		f.ProjectIDs = ids
	}
	if req.PriorityMin != "" {
		n, err := parsePriority(req.PriorityMin)
		if err != nil {
			return f, err
		}
		f.PriorityMin = &n
	}
	if req.PriorityMax != "" {
		n, err := parsePriority(req.PriorityMax)
		if err != nil {
			return f, err
		}
		f.PriorityMax = &n
	}
	if req.Status != "" {
		status := req.Status
		f.Status = &status
	}
	if req.Component != "" {
		component := req.Component
		f.Component = &component
	}
	// epics_only takes precedence over epic when both are set — mirrors
	// parseItemFilter (they can't sensibly combine, see ItemFilter.EpicsOnly/Epic).
	if req.EpicsOnly {
		f.EpicsOnly = true
	} else if req.Epic != "" {
		epic := req.Epic
		f.Epic = &epic
	}
	if req.Since != "" {
		cutoff, err := backlog.ParseSinceCutoff(req.Since)
		if err != nil {
			return f, err
		}
		f.CreatedSince = &cutoff
	}
	if req.Sort != "" {
		switch req.Sort {
		case "created":
			f.SortByCreated = true
		default:
			return f, fmt.Errorf("invalid sort value %q: expected \"created\"", req.Sort)
		}
	}
	f.Limit = req.Limit

	return f, nil
}
