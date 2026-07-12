package cli

import (
	"context"
	"database/sql"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dangernoodle-io/mcpkit/host/claudecode/hooks"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/store"
)

// subagentStartMaxEntries caps the number of KB rows injected. Faithful port
// of subagent-start.js's MAX_ENTRIES.
const subagentStartMaxEntries = 5

// subagentStartMaxItems caps the number of open backlog items injected.
// Faithful port of subagent-start.js's MAX_ITEMS.
const subagentStartMaxItems = 5

// hookHandleSubagentStart is the SubagentStart-hook KB/backlog context
// injector, a native port of plugin/scripts/subagent-start.js, wired as a
// hooks.Handler[hooks.SubagentStartPayload]: this is the highest-frequency
// KB reader — it fires on every subagent spawn. This hook is advisory: a
// withDB failure is logged to stderr only and never surfaces as a blocking
// Response — the zero hooks.Response ("silent allow") is returned instead,
// matching the Node script's catch-all exit(0).
func hookHandleSubagentStart(_ context.Context, _ io.Reader, p hooks.SubagentStartPayload) hooks.Response {
	var resp hooks.Response
	if err := withDB(func(db *sql.DB) error {
		resp = runHookSubagentStart(p, db)
		return nil
	}); err != nil {
		logHookEvent(map[string]any{"hook": "subagent_start", "kind": "error", "detail": err.Error(), "session_id": p.SessionID})
	}
	return resp
}

// runHookSubagentStart implements the SubagentStart-hook flow.
//
// Project resolution order (OU-283/OU-263-consistency change, deliberate
// deviation from the Node original, which resolved solely via
// projectFromPath(cwd)): mirrors runHookUserPromptSubmit's hub-aware order —
//  1. Prompt-mention first (resolveProjectFromMessage): if the subagent's
//     prompt names one of ouroboros's own REGISTERED projects
//     (backlog.ListProjects — the projects table, not a filesystem walk of
//     workspace subdirectories; see OU-309), that wins outright. Candidate
//     names are sorted longest-first so a more-specific registered project
//     name wins over a shorter one that also happens to match.
//  2. Else cwd, UNLESS the cwd's git root is the marketplace hub itself
//     (isMarketplaceRepo) — the hub is a repo, but never a "project" whose
//     KB/backlog a hook should inject.
//  3. Else inject nothing.
//
// No cooldown: unlike UserPromptSubmit (which is per-session and gated to
// avoid re-injecting on every turn of the same conversation), SubagentStart
// fires once per subagent spawn, and each subagent is a fresh, separate
// context window with no memory of any prior spawn — Claude Code injects
// the additionalContext at the start of that subagent's conversation only.
// A cooldown here would starve a later, unrelated subagent spawn of context
// it has never seen. subagent-start.js's 60s session+project cooldown was a
// bug (see rules/plugin.md: "Do NOT cooldown SubagentStart — every subagent
// is a fresh context with no memory"); this port deliberately does NOT
// carry it forward — every non-skipped spawn with something to inject
// injects.
func runHookSubagentStart(p hooks.SubagentStartPayload, db *sql.DB) hooks.Response {
	project := resolveProjectFromMessage(p.Prompt, registeredProjectNamesLongestFirst(db))
	if project == "" && p.Cwd != "" {
		gitRoot := findGitRoot(p.Cwd)
		if gitRoot != "" && !isMarketplaceRepo(gitRoot) {
			project = filepath.Base(gitRoot)
		}
	}

	// Log fire events immediately, before any early exits — mirrors the
	// Node original's explicit ordering comment (both events logged with
	// whatever project was resolved above, even for a skipped agent type).
	logHookEvent(map[string]any{"hook": "subagent_start", "kind": "fire", "session_id": p.SessionID, "project": project})
	logHookEvent(map[string]any{"hook": "subagent_start", "kind": "subagent_start", "session_id": p.SessionID, "agent_type": p.AgentType})

	if isSkippedAgentType(p.AgentType) {
		return hooks.Response{}
	}

	if project == "" {
		return hooks.Response{}
	}

	// Query KB: search-based if a prompt is available, else recent; fall
	// back to recent if the search returned nothing. Faithful port —
	// unlike the UserPromptSubmit hook's "specific" intent, this hook
	// applies no BM25 score threshold filtering.
	var rows []store.DocumentSummary
	var err error
	if p.Prompt != "" {
		search := truncateRunes(strings.ReplaceAll(p.Prompt, "'", ""), 200)
		rows, err = store.KeywordSearch(db, search, []string{project}, subagentStartMaxEntries)
	}
	if err != nil || len(rows) == 0 {
		rows, _ = store.QueryDocuments(db, nil, []string{project}, nil, "", nil, subagentStartMaxEntries)
	}

	items := subagentStartBacklogItems(db, project)

	if len(rows) == 0 && len(items) == 0 {
		return hooks.Response{}
	}

	var lines []string
	if len(rows) > 0 {
		label := "KB context (recent)"
		if p.Prompt != "" {
			label = "KB context (relevant)"
		}
		lines = append(lines, "[ouroboros] "+label+":")
		for _, r := range rows {
			lines = append(lines, "  ["+r.Type+"] "+r.Title)
		}
	}

	if len(items) > 0 {
		lines = append(lines, "[ouroboros] Open items (top "+strconv.Itoa(len(items))+"):")
		for _, item := range items {
			id := item.ID
			if id == "" {
				id = "?"
			}
			priority := item.Priority
			if priority == "" {
				priority = "?"
			}
			title := item.Title
			if title == "" {
				title = "(no title)"
			}
			lines = append(lines, "  "+id+" "+priority+" "+title)
		}
	}

	return hooks.Response{AdditionalContext: strings.Join(lines, "\n")}
}

// subagentStartBacklogItems queries project's open backlog items (native
// backlog.GetProjectByName + backlog.ListItems, replacing the Node
// original's shell-out to `ouroboros ls backlog --json`). Returns nil on
// any failure or empty result — fail-open, matching the Node original's
// try/catch silently skipping the items section.
func subagentStartBacklogItems(db *sql.DB, project string) []backlog.Item {
	proj, err := backlog.GetProjectByName(db, project)
	if err != nil || proj == nil {
		return nil
	}

	status := "open"
	items, err := backlog.ListItems(db, backlog.ItemFilter{
		ProjectIDs: []int64{proj.ID},
		Status:     &status,
		Limit:      subagentStartMaxItems,
	})
	if err != nil || len(items) == 0 {
		return nil
	}
	return items
}
