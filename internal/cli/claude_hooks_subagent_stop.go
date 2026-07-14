package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/dangernoodle-io/mcpkit/host/claudecode/hooks"
)

// hookHandleSubagentStop is the SubagentStop-hook persist-only port of
// plugin/scripts/subagent-stop.js (extended per OU-316), wired as a
// hooks.Handler[hooks.SubagentStopPayload]: it opportunistically persists
// every fenced ```kb block found in the subagent's final message (not just
// the first), and otherwise does nothing. A malformed/failed block surfaces
// as a Response.SystemMessage warning — never Block (see CRITICAL below).
// This hook is advisory: a withDB failure (unwritable path, disk full,
// migration mismatch) is logged to stderr only and never surfaces as a
// blocking Response — the zero hooks.Response ("silent allow") is returned
// instead, matching subagent-stop.js's catch-all exit(0). The raw stdin
// reader is unused: the SubagentStop payload carries every field this
// handler needs.
//
// CRITICAL (OU-222/OU-254): unlike the Stop hook, this handler NEVER emits a
// decision-language nudge and NEVER sets Response.Block, even to warn on a
// malformed block. A subagent's final message IS its return value to the
// orchestrator — a blocking Response would force the subagent to take
// another turn to address it, and that meta-acknowledgement turn would
// overwrite the real report as the "final" message, destroying the report
// the caller actually needs. SystemMessage is safe here because it does not
// block or force another turn — only Block does. See runHookSubagentStop.
func hookHandleSubagentStop(_ context.Context, _ io.Reader, p hooks.SubagentStopPayload) hooks.Response {
	var resp hooks.Response
	if err := withDB(func(db *sql.DB) error {
		resp = runHookSubagentStop(p, db)
		return nil
	}); err != nil {
		fmt.Fprintf(os.Stderr, "[ouroboros] hook subagent-stop: %s\n", err)
	}
	return resp
}

// runHookSubagentStop implements the SubagentStop-hook flow. It NEVER sets
// Response.Block (OU-222/OU-254 — see hookHandleSubagentStop's doc comment):
// every path either returns the zero hooks.Response ("silent allow") or, on
// a malformed/failed kb block, a Response with ONLY SystemMessage set. There
// is no nudge/Block path at all.
func runHookSubagentStop(p hooks.SubagentStopPayload, db *sql.DB) hooks.Response {
	project := resolveHookProject(p.ProjectDir, p.Cwd)

	logHookEvent(map[string]any{"hook": "subagent_stop", "kind": "fire", "session_id": p.SessionID, "project": project})

	message := p.LastAssistantMessage

	// Log the subagent_stop event unconditionally, before any early exits —
	// mirrors subagent-stop.js's explicit ordering comment.
	logHookEvent(map[string]any{
		"hook": "subagent_stop", "kind": "subagent_stop",
		"session_id": p.SessionID, "agent_id": p.AgentID, "agent_type": p.AgentType,
		"last_message_excerpt": excerptMessage(message, 120),
	})

	if isSkippedAgentType(p.AgentType) {
		return hooks.Response{}
	}

	if utf8.RuneCountInString(message) < 80 {
		return hooks.Response{}
	}
	message = truncateMessage(message, 5000)

	agentIDShort := p.AgentID
	if len(agentIDShort) > 8 {
		agentIDShort = agentIDShort[:8]
	}

	extraMeta := map[string]string{}
	if p.AgentID != "" {
		extraMeta["agent_id"] = p.AgentID
	}
	if p.AgentType != "" {
		extraMeta["agent_type"] = p.AgentType
	}

	// Persist EVERY kb block in the message (OU-316), not just the first.
	_, warnings, foundAnyBlock := persistAllKbBlocksInMessage(message, db, "subagent", agentIDShort, "subagent_stop", p.SessionID, project, extraMeta)
	if foundAnyBlock {
		if len(warnings) > 0 {
			return hooks.Response{SystemMessage: "[ouroboros] " + strings.Join(warnings, "; ") + " — persist reliably by calling the kb tool, then search to confirm."}
		}
		return hooks.Response{}
	}

	// No decision-language nudge here (unlike the Stop hook): see
	// hookHandleSubagentStop's doc comment (OU-222/OU-254).

	// Default: exit silently (exploratory output).
	logHookEvent(map[string]any{"hook": "subagent_stop", "kind": "noop", "session_id": p.SessionID, "project": project})
	return hooks.Response{}
}

// excerptMessage caps message at maxRunes runes and replaces newlines with
// spaces, for a compact single-line log excerpt. Faithful port of
// subagent-stop.js's `message.substring(0, 120).replace(/\n/g, ' ')`.
func excerptMessage(message string, maxRunes int) string {
	runes := []rune(message)
	if len(runes) > maxRunes {
		runes = runes[:maxRunes]
	}
	return strings.ReplaceAll(string(runes), "\n", " ")
}
