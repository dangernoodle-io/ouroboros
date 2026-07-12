package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"unicode/utf8"

	"github.com/dangernoodle-io/mcpkit/host/claudecode/hooks"
)

// hookHandleStop is the Stop-hook persist-nudge, a native port of
// plugin/scripts/stop.js, wired as a hooks.Handler[hooks.StopPayload]: given
// the Stop event's decoded payload, it either auto-persists a fenced ```kb
// block from the last assistant turn or nudges the user to persist decision
// language, returning at most one hooks.Response.Block. This hook is
// advisory: a withDB failure (unwritable path, disk full, migration
// mismatch) is logged to stderr only and never surfaces as a blocking
// Response — the zero hooks.Response ("silent allow") is returned instead,
// matching stop.js's catch-all exit(0). The raw stdin reader is unused: the
// Stop payload carries every field this handler needs.
func hookHandleStop(_ context.Context, _ io.Reader, p hooks.StopPayload) hooks.Response {
	var resp hooks.Response
	if err := withDB(func(db *sql.DB) error {
		resp = runHookStop(p, db)
		return nil
	}); err != nil {
		fmt.Fprintf(os.Stderr, "[ouroboros] hook stop: %s\n", err)
	}
	return resp
}

// runHookStop implements the Stop-hook flow. It never blocks the advisory
// hook path on failure — every early-exit/failure case returns the zero
// hooks.Response value ("silent allow"), matching stop.js's catch-all
// exit(0) behavior.
func runHookStop(p hooks.StopPayload, db *sql.DB) hooks.Response {
	// CRITICAL: avoid infinite loop when this hook causes the next turn.
	if p.StopHookActive {
		return hooks.Response{}
	}
	if p.TranscriptPath == "" {
		return hooks.Response{}
	}

	project := ""
	if p.Cwd != "" {
		project = projectFromPath(p.Cwd)
	}

	logHookEvent(map[string]any{"hook": "stop", "kind": "fire", "session_id": p.SessionID, "project": project})

	message := readLastMainAssistantText(p.TranscriptPath)
	if utf8.RuneCountInString(message) < 80 {
		return hooks.Response{}
	}
	message = truncateMessage(message, 5000)

	sessionShort := p.SessionID
	if sessionShort == "" {
		sessionShort = "main"
	}
	if len(sessionShort) > 8 {
		sessionShort = sessionShort[:8]
	}

	if persistKbBlock(message, db, "main", sessionShort, "stop", p.SessionID, project, nil) {
		return hooks.Response{}
	}

	nudge, tier := checkNudgePatterns(message, "main", sessionShort, "stop", p.SessionID, project)
	if nudge != nil {
		// Tier-1 fires on decision language with no kb block in the FINAL
		// message — but the session may have already persisted earlier THIS
		// TURN (an ouroboros MCP write tool_use, or a prior ```kb block).
		// Suppress only the tier-1 nudge in that case; tier-2 (self-claim) is
		// unaffected since it already implies persistence was referenced.
		if tier == 1 && turnAlreadyPersisted(p.TranscriptPath) {
			logHookEvent(map[string]any{"hook": "stop", "kind": "suppressed", "session_id": p.SessionID, "project": project, "reason": "tier-1-persisted-this-turn"})
			return hooks.Response{}
		}
		return hooks.Response{Block: nudge.Reason}
	}

	// Default: exit silently (exploratory output).
	logHookEvent(map[string]any{"hook": "stop", "kind": "noop", "session_id": p.SessionID, "project": project})
	return hooks.Response{}
}
