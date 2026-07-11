package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dangernoodle-io/mcpkit/mcpx"

	"dangernoodle.io/ouroboros/internal/backlog"
)

// errDomainRequired is the verbatim message the old mcp-go handleGet/
// handleSearch switch default returned for both a missing domain key and
// domain="" — shared by handleGetV2/handleSearchV2's explicit early check
// (see getInput.Domain's comment for why domain is not schema-required).
const errDomainRequired = `domain is required: must be "kb", "backlog", or "roadmap"`

// jsonResultV2 is jsonResult's mcpx-typed counterpart: marshal v to a single
// text-content block, or an ErrorResult carrying the marshal error verbatim.
// Out is always nil (see the STEP-0 probe / OU-1 build spec).
func jsonResultV2(v any) (*mcpx.CallToolResult, any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return mcpx.ErrorResult(err.Error()), nil, nil
	}
	return mcpx.TextResult(string(data)), nil, nil
}

// itemLinesTextV2 renders itemLinesResult's compact one-line-per-item text
// (or "no items"), as a plain string for the caller to wrap in
// mcpx.TextResult.
func itemLinesTextV2(items []backlog.Item) string {
	if len(items) == 0 {
		return "no items"
	}

	lines := make([]string, 0, len(items))
	for _, item := range items {
		componentStr := ""
		if item.Component != "" {
			componentStr = fmt.Sprintf("(%s) ", item.Component)
		}
		lines = append(lines, fmt.Sprintf("%s %s [%s] %s%s", item.ID, item.Priority, item.Status, componentStr, item.Title))
	}
	return strings.Join(lines, "\n")
}

// parseItemFilterV2 is parseItemFilter's typed-input counterpart, building a
// backlog.ItemFilter from the same fields getInput/searchInput share,
// shared by domain=backlog reads (get list mode, search). Preserves
// parseItemFilter's error strings and project-resolution behavior verbatim.
//
// Divergence note (OU-1, dark path only): the old handler set f.Component
// whenever the "component" arg key was merely *present* (even ""), which
// let an explicit component="" filter to items with no component — every
// other field there (priority_min/max, status, since, sort, epic) required
// non-empty. A typed Go string field can't distinguish "key present but
// empty" from "key absent" after JSON unmarshaling, so this port applies the
// same non-empty check uniformly (matching the majority behavior); an
// explicit component="" filter is not reproducible here. Not exercised by
// any existing test; noted for cutover review — tracked for OU-3 cutover.
func parseItemFilterV2(db *sql.DB, projects []string, priorityMin, priorityMax, status, component, epic string, epicsOnly bool, since, sort string, limit int) (backlog.ItemFilter, error) {
	var f backlog.ItemFilter

	if len(projects) > 0 {
		ids, err := resolveProjects(db, projects)
		if err != nil {
			return f, err
		}
		f.ProjectIDs = ids
	}
	if priorityMin != "" {
		n, err := parsePriority(priorityMin)
		if err != nil {
			return f, err
		}
		f.PriorityMin = &n
	}
	if priorityMax != "" {
		n, err := parsePriority(priorityMax)
		if err != nil {
			return f, err
		}
		f.PriorityMax = &n
	}
	if status != "" {
		f.Status = &status
	}
	if component != "" {
		f.Component = &component
	}
	// epics_only takes precedence over epic when both are set — mirrors
	// parseItemFilter (they can't sensibly combine, see ItemFilter.EpicsOnly/Epic).
	if epicsOnly {
		f.EpicsOnly = true
	} else if epic != "" {
		f.Epic = &epic
	}
	if since != "" {
		cutoff, err := backlog.ParseSinceCutoff(since)
		if err != nil {
			return f, err
		}
		f.CreatedSince = &cutoff
	}
	if sort != "" {
		switch sort {
		case "created":
			f.SortByCreated = true
		default:
			return f, fmt.Errorf("invalid sort value %q: expected \"created\"", sort)
		}
	}
	f.Limit = limit

	return f, nil
}
