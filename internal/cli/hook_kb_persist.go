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

// persistKbBlock extracts a fenced ```kb block from message and, if found,
// persists it to the KB via a direct kb.WriteBatch call. Returns true if a
// kb block was matched (whether persist succeeded or failed) — a matched
// block is "handled" and the caller must not fall through to any subsequent
// nudge check. Returns false if no kb block was found. extraMeta carries
// caller-specific metadata (e.g. subagent_stop's agent_id/agent_type) merged
// onto each entry's metadata alongside source/session_id; pass nil when the
// caller has none (e.g. the Stop hook).
func persistKbBlock(message string, db *sql.DB, label, idShort, hookName, sessionID, project string, extraMeta map[string]string) bool {
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
		for k, v := range extraMeta {
			entries[i].Metadata[k] = v
		}
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
