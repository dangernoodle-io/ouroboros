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

// hookHandleStop is the Stop-hook persist-nudge, a native port of
// plugin/scripts/stop.js (extended per OU-316), wired as a
// hooks.Handler[hooks.StopPayload]: given the Stop event's decoded payload,
// it auto-persists every fenced ```kb block found anywhere in the CURRENT
// turn (not just the final message — see persistWholeTurnKbBlocks, a pure
// DATA side-action), surfacing any malformed/failed block as a
// Response.SystemMessage warning that coexists with whatever else the
// Response carries. The nudge/Block decision itself is gated ONLY on the
// FINAL message (exactly the pre-OU-316 behavior — see runHookStop): a kb
// block in the final message short-circuits the nudge; otherwise decision
// language triggers a nudge, returning at most one hooks.Response.Block.
// This hook is advisory: a withDB failure (unwritable path, disk full,
// migration mismatch) is logged to stderr only and never surfaces as a
// blocking Response — the zero hooks.Response ("silent allow") is returned
// instead, matching stop.js's catch-all exit(0). The raw stdin reader is
// unused: the Stop payload carries every field this handler needs.
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
//
// OU-316 HIGH regression fix: an earlier version of this function
// short-circuited the ENTIRE nudge/Block decision whenever the whole-turn
// persist found ANY block anywhere in the turn (foundAnyBlock). That
// silently suppressed a tier-2 self-claim nudge on the final message
// whenever an earlier, unrelated mid-turn block existed — reintroducing the
// very silent-loss class OU-316 was fixing. The whole-turn persist below is
// now a pure DATA side-action: it persists every non-sentinel block found
// anywhere in the turn, but the nudge/Block decision that follows is gated
// ONLY on the FINAL message's own kb block (exactly the pre-OU-316
// behavior) — mirroring the old persistKbBlock(message) short-circuit,
// which only ever looked at the final message.
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

	sessionShort := p.SessionID
	if sessionShort == "" {
		sessionShort = "main"
	}
	if len(sessionShort) > 8 {
		sessionShort = sessionShort[:8]
	}

	// Whole-turn persist (OU-316 data fix): scan EVERY main-context assistant
	// message of the current turn, not just the final one, and persist every
	// non-sentinel kb block found (mid-turn + final) — a well-formed kb
	// block emitted mid-turn must actually be persisted, not silently
	// dropped. This is a pure side-action: its result does NOT gate the
	// nudge decision below (see the OU-316 HIGH-fix doc comment above).
	_, warnings, _ := persistWholeTurnKbBlocks(p.TranscriptPath, db, "main", sessionShort, "stop", p.SessionID, project)

	message := readLastMainAssistantText(p.TranscriptPath)
	if utf8.RuneCountInString(message) < 80 {
		return withStopWarnings(hooks.Response{}, warnings)
	}
	message = truncateMessage(message, 5000)

	// The final message's own kb block (if any) was already persisted above
	// by persistWholeTurnKbBlocks — do not persist it again here. Old
	// behavior: a kb block in the FINAL message alone short-circuits the
	// nudge decision (persistKbBlock(message) used to do double duty as
	// both "persist" and "handled, no nudge"; the persist half now lives in
	// persistWholeTurnKbBlocks, this is only the "handled" half).
	if extractKbBlock(message) {
		return withStopWarnings(hooks.Response{}, warnings)
	}

	nudge, tier := checkNudgePatterns(message, "main", sessionShort, "stop", p.SessionID, project)
	if nudge != nil {
		// Tier-1 fires on decision language with no kb block in the FINAL
		// message — but the session may have already persisted earlier THIS
		// TURN (an ouroboros MCP write tool_use, or a prior ```kb block).
		// Suppress only the tier-1 nudge in that case; tier-2 (self-claim) is
		// NEVER suppressed since it already implies persistence was
		// referenced — restoring the exact pre-OU-316 guarantee.
		if tier == 1 && turnAlreadyPersisted(p.TranscriptPath) {
			logHookEvent(map[string]any{"hook": "stop", "kind": "suppressed", "session_id": p.SessionID, "project": project, "reason": "tier-1-persisted-this-turn"})
			return withStopWarnings(hooks.Response{}, warnings)
		}
		return withStopWarnings(hooks.Response{Block: nudge.Reason}, warnings)
	}

	// Default: exit silently (exploratory output).
	logHookEvent(map[string]any{"hook": "stop", "kind": "noop", "session_id": p.SessionID, "project": project})
	return withStopWarnings(hooks.Response{}, warnings)
}

// withStopWarnings sets resp.SystemMessage from warnings (if any non-empty),
// leaving every other field of resp untouched. This lets a malformed-block
// warning coexist with a firing Block nudge on the SAME Response — response
// .go's blockOutput JSON shape carries SystemMessage alongside the block
// Reason, so both surface to the model/user in one hook invocation.
func withStopWarnings(resp hooks.Response, warnings []string) hooks.Response {
	if len(warnings) > 0 {
		resp.SystemMessage = "[ouroboros] " + strings.Join(warnings, "; ") + " — persist reliably by calling the kb tool, then search to confirm."
	}
	return resp
}
