package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dangernoodle-io/mcpkit/host/claudecode/hooks"

	"dangernoodle.io/ouroboros/internal/kb"
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

	if persistKbBlock(message, db, "main", sessionShort, "stop", p.SessionID, project) {
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

// hookKbEntry decodes a single fenced ```kb block entry, capturing the
// persist-skill sentinel alongside the standard kb.Entry fields.
type hookKbEntry struct {
	kb.Entry
	PersistedBy string `json:"_persisted_by,omitempty"`
}

// parseKbBlockEntries decodes a fenced kb block's JSON body, which may be a
// single object or an array of objects, returning the normalized entries and
// the first entry's _persisted_by sentinel (if any).
func parseKbBlockEntries(raw []byte) ([]kb.Entry, string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var hooked []hookKbEntry
		if err := json.Unmarshal(raw, &hooked); err != nil {
			return nil, "", err
		}
		entries := make([]kb.Entry, len(hooked))
		persistedBy := ""
		for i, h := range hooked {
			entries[i] = h.Entry
			if i == 0 {
				persistedBy = h.PersistedBy
			}
		}
		return entries, persistedBy, nil
	}

	var h hookKbEntry
	if err := json.Unmarshal(raw, &h); err != nil {
		return nil, "", err
	}
	return []kb.Entry{h.Entry}, h.PersistedBy, nil
}

// persistKbBlock extracts a fenced ```kb block from message and, if found,
// persists it to the KB via a direct kb.WriteBatch call. Returns true if a
// kb block was matched (whether persist succeeded or failed) — a matched
// block is "handled" and the caller must not fall through to the nudge
// check. Returns false if no kb block was found.
func persistKbBlock(message string, db *sql.DB, label, idShort, hookName, sessionID, project string) bool {
	matched, body := extractKbBlock(message)
	if !matched {
		return false
	}

	entries, persistedBy, err := parseKbBlockEntries([]byte(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ouroboros] %s %s: kb block JSON parse error: %s\n", label, idShort, err)
		return true
	}

	if persistedBy == "persist-skill" {
		logHookEvent(map[string]any{"hook": hookName, "kind": "skip", "reason": "persist-skill-sentinel", "session_id": sessionID, "project": project})
		return true
	}

	if project == "" {
		fmt.Fprintf(os.Stderr, "[ouroboros] %s %s: kb block found but no project (run inside a git repo)\n", label, idShort)
		return true
	}

	for i := range entries {
		if entries[i].Metadata == nil {
			entries[i].Metadata = map[string]string{}
		}
		entries[i].Metadata["source"] = "hook:" + hookName
		entries[i].Metadata["session_id"] = sessionID
	}

	results, err := kb.WriteBatch(db, entries, project)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ouroboros] %s %s: kb failed: %s\n", label, idShort, err)
		logHookEvent(map[string]any{"hook": hookName, "kind": "error", "detail": err.Error(), "session_id": sessionID, "project": project})
		return true
	}

	ids := make([]string, 0, len(results))
	for _, r := range results {
		ids = append(ids, strconv.FormatInt(r.ID, 10))
	}
	fmt.Fprintf(os.Stderr, "[ouroboros] %s %s: persisted %d entries to %s [ids: %s]\n", label, idShort, len(results), project, strings.Join(ids, ","))
	logHookEvent(map[string]any{"hook": hookName, "kind": "persist", "session_id": sessionID, "project": project, "entries": len(results), "ids": ids})
	return true
}
