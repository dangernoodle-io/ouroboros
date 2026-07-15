package app

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/edges"
)

// This file holds the type-neutral consts/helpers/types the old mcp-go-based
// server (server.go/handlers_backlog.go/handlers_kb.go/handlers_roadmap.go,
// deleted at the OU-5 cutover) and buildServerV2's handlers both depended
// on. Extracted here — verbatim, unchanged — BEFORE the old server files
// were deleted, so V2 keeps them without carrying that old transport
// dependency along.

// validSectionsMsg lists the valid section names for error messages.
const validSectionsMsg = "now, next, deferred, parked, dropped, or done"

// edgeSpec is one {label,target} element of a backlog entry's edges[]: an
// item->item edge, source = the item being written.
type edgeSpec struct {
	Label  string
	Target string
}

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

// resolveProject accepts backlog.Executor (not just *sql.DB) so a caller
// already inside a shared transaction (e.g. the batch write in
// handleBacklogEntriesV2) can resolve a project on that same connection —
// the store enforces SetMaxOpenConns(1), so a second *sql.DB-level query
// while a tx holds the only connection would deadlock.
func resolveProject(d backlog.Executor, name string) (*backlog.Project, error) {
	return backlog.GetProjectByName(d, name)
}

// validateEpicTx confirms a non-empty epic value resolves to an existing
// backlog item (alias-aware, via backlog.GetItem — the same resolution path
// EpicLabels uses, so a renamed epic still validates), mirroring
// linkEdgesTx's target-exists check for edges: a typo'd/dangling epic id
// must not be silently accepted. An empty epic (clearing the field) is
// always allowed and skips validation.
func validateEpicTx(tx *sql.Tx, epic string) error {
	if epic == "" {
		return nil
	}
	if _, err := backlog.GetItem(tx, epic); err != nil {
		return fmt.Errorf("epic item %q not found: %w", epic, err)
	}
	return nil
}

// resolveEpicRef resolves a batch entry's raw epic value, substituting a
// "$N" intra-batch back-reference with the item id posMap recorded for
// entries[N] earlier in this same write — the whole point being that a
// child can name its not-yet-created epic parent (server-assigned ids can't
// be known in advance). N must strictly precede idx (parent-before-child;
// rejects self/forward refs and, incidentally, one-entry cycles), and must
// be in range. A non-"$"-prefixed value (including empty, i.e. "clear")
// passes through unchanged. Called before validateEpicTx in both the
// create and update paths so the substituted id gets the same existence
// check as any ordinary epic reference.
func resolveEpicRef(epic string, idx int, numEntries int, posMap map[int]string) (string, error) {
	if !strings.HasPrefix(epic, "$") {
		return epic, nil
	}

	n, err := strconv.Atoi(epic[1:])
	if err != nil {
		return "", fmt.Errorf("invalid epic back-reference %q: expected $N where N is an integer entry index", epic)
	}
	if n < 0 || n >= numEntries {
		return "", fmt.Errorf("epic back-reference %q out of range: batch has %d entries", epic, numEntries)
	}
	if n >= idx {
		return "", fmt.Errorf("epic back-reference %q must point to an earlier entry (index %d comes at or after this entry, index %d)", epic, n, idx)
	}

	id, ok := posMap[n]
	if !ok {
		return "", fmt.Errorf("epic back-reference %q refers to an entry that produced no item", epic)
	}
	return id, nil
}

// linkEdgesTx creates each spec as an item->item edge sourced from itemID,
// on the given transaction so the edge links commit atomically with the
// item write that produced them (see handleBacklogEntriesV2). Validates
// each target item exists first — a typo'd target must not silently
// produce a permanently dangling edge.
func linkEdgesTx(tx *sql.Tx, itemID string, projectID int64, specs []edgeSpec) error {
	for _, spec := range specs {
		if _, err := backlog.GetItem(tx, spec.Target); err != nil {
			return fmt.Errorf("edge target item %q not found: %w", spec.Target, err)
		}
		if _, err := edges.Link(tx, edges.TypeItem, itemID, spec.Label, edges.TypeItem, spec.Target, projectID); err != nil {
			return err
		}
	}
	return nil
}
