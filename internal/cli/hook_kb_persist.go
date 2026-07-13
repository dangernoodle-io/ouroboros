package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"dangernoodle.io/ouroboros/internal/kb"
)

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

// persistOneKbBlock parses and persists a single fenced ```kb block's raw
// JSON body (one call per block — a message or turn may contain several).
// Returns persisted=true if entries were written. Returns a non-empty
// warning describing any problem — malformed JSON, a missing project, or a
// kb.WriteBatch failure — that the caller must surface to the user via
// hooks.Response.SystemMessage. This is load-bearing: stderr (fmt.Fprintf
// below) is ALSO written for local debugging, but a hook's stderr is
// invisible to both the user and the model — SystemMessage is the only
// channel that actually surfaces a malformed/failed block, so every warning
// path here returns its message rather than only logging it (OU-316).
//
// The persist-skill sentinel (_persisted_by:"persist-skill") is a
// deliberate no-warning skip: it means the persist skill already wrote this
// entry via the kb tool and emitted the block only as an anti-double-write
// marker, not a request to persist again.
func persistOneKbBlock(body string, db *sql.DB, label, idShort, hookName, sessionID, project string, extraMeta map[string]string) (persisted bool, warning string) {
	entries, persistedBy, err := parseKbBlockEntries([]byte(body))
	if err != nil {
		msg := fmt.Sprintf("%s %s: kb block JSON parse error: %s", label, idShort, err)
		fmt.Fprintf(os.Stderr, "[ouroboros] %s\n", msg)
		return false, msg
	}

	if persistedBy == "persist-skill" {
		logHookEvent(map[string]any{"hook": hookName, "kind": "skip", "reason": "persist-skill-sentinel", "session_id": sessionID, "project": project})
		return false, ""
	}

	if project == "" {
		msg := fmt.Sprintf("%s %s: kb block found but no project (run inside a git repo)", label, idShort)
		fmt.Fprintf(os.Stderr, "[ouroboros] %s\n", msg)
		return false, msg
	}

	for i := range entries {
		if entries[i].Metadata == nil {
			entries[i].Metadata = map[string]string{}
		}
		entries[i].Metadata["source"] = "hook:" + hookName
		entries[i].Metadata["session_id"] = sessionID
		for k, v := range extraMeta {
			entries[i].Metadata[k] = v
		}
	}

	results, err := kb.WriteBatch(db, entries, project)
	if err != nil {
		msg := fmt.Sprintf("%s %s: kb failed: %s", label, idShort, err)
		fmt.Fprintf(os.Stderr, "[ouroboros] %s\n", msg)
		logHookEvent(map[string]any{"hook": hookName, "kind": "error", "detail": err.Error(), "session_id": sessionID, "project": project})
		return false, msg
	}

	ids := make([]string, 0, len(results))
	for _, r := range results {
		ids = append(ids, strconv.FormatInt(r.ID, 10))
	}
	fmt.Fprintf(os.Stderr, "[ouroboros] %s %s: persisted %d entries to %s [ids: %s]\n", label, idShort, len(results), project, strings.Join(ids, ","))
	logHookEvent(map[string]any{"hook": hookName, "kind": "persist", "session_id": sessionID, "project": project, "entries": len(results), "ids": ids})
	return true, ""
}

// persistAllKbBlocksInMessage extracts every fenced ```kb block from a
// single message (extractAllKbBlockBodies) and persists each via
// persistOneKbBlock, aggregating the persisted count and any warnings.
// foundAny reports whether at least one kb block was present in message at
// all — the caller uses this to decide whether to fall through to any
// subsequent nudge/decision-language check. Used directly by SubagentStop
// (whose only signal is p.LastAssistantMessage) and, per assistant text, by
// persistWholeTurnKbBlocks (the Stop hook's whole-turn scan).
func persistAllKbBlocksInMessage(message string, db *sql.DB, label, idShort, hookName, sessionID, project string, extraMeta map[string]string) (persistedCount int, warnings []string, foundAny bool) {
	bodies := extractAllKbBlockBodies(message)
	if len(bodies) == 0 {
		return 0, nil, false
	}

	for _, body := range bodies {
		ok, warn := persistOneKbBlock(body, db, label, idShort, hookName, sessionID, project, extraMeta)
		if ok {
			persistedCount++
		}
		if warn != "" {
			warnings = append(warnings, warn)
		}
	}
	return persistedCount, warnings, true
}

// persistWholeTurnKbBlocks scans EVERY main-context assistant message of the
// CURRENT turn (scanTurnAssistantTexts — the same turn boundary
// turnAlreadyPersisted uses), not just the final message, and persists every
// fenced ```kb block found across all of them via persistAllKbBlocksInMessage.
// This closes the Stop hook's silent mid-turn loss (OU-316): a well-formed kb
// block emitted mid-turn was previously never persisted, yet
// turnAlreadyPersisted correctly detected it and suppressed the tier-1 nudge
// — silently dropping the entry with no signal to the user at all.
func persistWholeTurnKbBlocks(transcriptPath string, db *sql.DB, label, idShort, hookName, sessionID, project string) (persistedCount int, warnings []string, foundAny bool) {
	for _, text := range scanTurnAssistantTexts(transcriptPath) {
		count, warns, found := persistAllKbBlocksInMessage(text, db, label, idShort, hookName, sessionID, project, nil)
		if found {
			foundAny = true
		}
		persistedCount += count
		warnings = append(warnings, warns...)
	}
	return persistedCount, warnings, foundAny
}
