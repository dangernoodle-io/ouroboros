package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"dangernoodle.io/ouroboros/internal/kb"
)

// hookStopCmd is the Stop-hook persist-nudge, a native port of
// plugin/scripts/stop.js: reads the Stop event payload from stdin, either
// auto-persists a fenced ```kb block from the last assistant turn or nudges
// the user to persist decision language, writing at most one JSON line
// ({"decision":"block","reason":...}) to stdout. This hook is advisory and
// must never exit non-zero.
var hookStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop-hook persist-nudge (Claude Code plugin integration)",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Never propagate a non-nil error to cobra: this hook is advisory and
		// must always exit 0 (a DB-open failure here — unwritable path, disk
		// full, migration mismatch — must not fail-closed like the Node
		// original never could). Log to stderr only; stdout is reserved for
		// the exit-0 JSON decision protocol.
		if err := withDB(func(db *sql.DB) error {
			return runHookStop(cmd.OutOrStdout(), cmd.InOrStdin(), db)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "[ouroboros] hook stop: %s\n", err)
		}
		return nil
	},
}

func init() {
	hookCmd.AddCommand(hookStopCmd)
}

// hookStopInput is the subset of the Stop hook's stdin payload this command
// consumes.
type hookStopInput struct {
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	SessionID      string `json:"session_id"`
	StopHookActive bool   `json:"stop_hook_active"`
}

// runHookStop implements the Stop-hook flow. It never returns a non-nil
// error on the advisory hook path — any failure is swallowed (matching
// stop.js's catch-all exit(0)) so the caller's RunE always signals success;
// stdout presence/absence carries the exit-0 decision protocol instead.
func runHookStop(out io.Writer, in io.Reader, db *sql.DB) error {
	data, err := io.ReadAll(in)
	if err != nil {
		return nil //nolint:nilerr
	}

	var payload hookStopInput
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil //nolint:nilerr
	}

	// CRITICAL: avoid infinite loop when this hook causes the next turn.
	if payload.StopHookActive {
		return nil
	}
	if payload.TranscriptPath == "" {
		return nil
	}

	project := ""
	if payload.Cwd != "" {
		project = projectFromPath(payload.Cwd)
	}

	logHookEvent(map[string]any{"hook": "stop", "kind": "fire", "session_id": payload.SessionID, "project": project})

	message := readLastMainAssistantText(payload.TranscriptPath)
	if utf8.RuneCountInString(message) < 80 {
		return nil
	}
	message = truncateMessage(message, 5000)

	sessionShort := payload.SessionID
	if sessionShort == "" {
		sessionShort = "main"
	}
	if len(sessionShort) > 8 {
		sessionShort = sessionShort[:8]
	}

	if persistKbBlock(message, db, "main", sessionShort, "stop", payload.SessionID, project) {
		return nil
	}

	nudge, tier := checkNudgePatterns(message, "main", sessionShort, "stop", payload.SessionID, project)
	if nudge != nil {
		// Tier-1 fires on decision language with no kb block in the FINAL
		// message — but the session may have already persisted earlier THIS
		// TURN (an ouroboros MCP write tool_use, or a prior ```kb block).
		// Suppress only the tier-1 nudge in that case; tier-2 (self-claim) is
		// unaffected since it already implies persistence was referenced.
		if tier == 1 && turnAlreadyPersisted(payload.TranscriptPath) {
			logHookEvent(map[string]any{"hook": "stop", "kind": "suppressed", "session_id": payload.SessionID, "project": project, "reason": "tier-1-persisted-this-turn"})
			return nil
		}
		data, err := json.Marshal(nudge)
		if err != nil {
			return nil //nolint:nilerr
		}
		fmt.Fprintf(out, "%s\n", data)
		return nil
	}

	// Default: exit silently (exploratory output).
	logHookEvent(map[string]any{"hook": "stop", "kind": "noop", "session_id": payload.SessionID, "project": project})
	return nil
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
