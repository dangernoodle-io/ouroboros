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
// plugin/scripts/subagent-stop.js, wired as a
// hooks.Handler[hooks.SubagentStopPayload]: it opportunistically persists an
// explicit fenced ```kb block found in the subagent's final message, and
// otherwise does nothing. This hook is advisory: a withDB failure
// (unwritable path, disk full, migration mismatch) is logged to stderr only
// and never surfaces as a blocking Response — the zero hooks.Response
// ("silent allow") is returned instead, matching subagent-stop.js's
// catch-all exit(0). The raw stdin reader is unused: the SubagentStop
// payload carries every field this handler needs.
//
// CRITICAL (OU-222/OU-254): unlike the Stop hook, this handler NEVER emits a
// decision-language nudge and NEVER blocks. A subagent's final message IS
// its return value to the orchestrator — a blocking Response would force the
// subagent to take another turn to address it, and that meta-acknowledgement
// turn would overwrite the real report as the "final" message, destroying
// the report the caller actually needs. See runHookSubagentStop.
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

// runHookSubagentStop implements the SubagentStop-hook flow. It never
// blocks: every path — including a matched kb block, a too-short message, a
// skipped agent type, or the re-entrancy guard — returns the zero
// hooks.Response value ("silent allow"), matching subagent-stop.js's
// catch-all exit(0) behavior. There is no nudge/Block path at all (see
// hookHandleSubagentStop's doc comment).
func runHookSubagentStop(p hooks.SubagentStopPayload, db *sql.DB) hooks.Response { //nolint:unparam // always the zero Response by design (OU-222/OU-254: never block/nudge a subagent's final turn); hooks.Response's return shape is kept for hooks.Handler[SubagentStopPayload] conformance, mirroring the Stop-hook's handler signature.
	project := ""
	if p.Cwd != "" {
		project = projectFromPath(p.Cwd)
	}

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

	if persistKbBlock(message, db, "subagent", agentIDShort, "subagent_stop", p.SessionID, project, extraMeta) {
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
