package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"regexp"

	"github.com/dangernoodle-io/shesha/host/claudecode/hooks"

	"dangernoodle.io/ouroboros/internal/store"
)

// preCompactDecisionThreshold is the number of decision-language turns (in
// the absence of any persisted kb-blocks) that flips the advisory log reason
// from "no_decisions" to "decisions_unpersisted_advisory". Faithful port of
// pre-compact.js's local `threshold = 3`.
const preCompactDecisionThreshold = 3

// preCompactPersistedQueryLimit mirrors pre-compact.js's queryKb 500-row cap
// (the JS never expects more than a handful of docs for one session, but
// caps generously to avoid truncating a real count).
const preCompactPersistedQueryLimit = 500

// preCompactDecisionPatterns are the broad decision-language signals used to
// flag a turn as advisory-worth-persisting. Faithful port of lib.js's
// decisionPatterns array (extractAllKbBlocks) — deliberately broader than
// tier1Patterns (hook_util.go), which is the Stop-hook's narrower nudge set.
var preCompactDecisionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bdecision\b`),
	regexp.MustCompile(`(?i)\bdecid`),
	regexp.MustCompile(`(?i)\bchose\b`),
	regexp.MustCompile(`(?i)\bchosen\b`),
	regexp.MustCompile(`(?i)\bchoose\b`),
	regexp.MustCompile(`(?i)\bgoing with\b`),
	regexp.MustCompile(`(?i)\breject`),
	regexp.MustCompile(`(?i)\bpick`),
	regexp.MustCompile(`(?i)\bselect`),
	regexp.MustCompile(`(?i)\bopting\b`),
}

// hasDecisionLanguage reports whether text matches any preCompactDecisionPatterns.
func hasDecisionLanguage(text string) bool {
	for _, p := range preCompactDecisionPatterns {
		if p.MatchString(text) {
			return true
		}
	}
	return false
}

// extractAllKbBlocks scans transcriptPath's tail (readTranscriptTail's same
// byte+line-count cap) for every main-context (non-sidechain) assistant
// turn, returning the count of turns containing a fenced ```kb block and the
// count of turns exhibiting decision language. Faithful port of lib.js's
// extractAllKbBlocks, reduced to the two counts pre-compact.js actually
// consumes (blocks.length and turns.filter(hasDecisionLanguage).length) —
// order does not matter for either count, so (unlike turnAlreadyPersisted)
// this scans forward and does not stop at a turn boundary; it considers the
// whole capped tail window. Fail-open: a missing/unreadable transcript
// yields (0, 0), same as the JS's try/catch returning {blocks:[],turns:[]}.
func extractAllKbBlocks(transcriptPath string) (blockCount, decisionTurnCount int) {
	lines := readTranscriptTail(transcriptPath)
	for _, line := range lines {
		var entry transcriptLine
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.Type != "assistant" || entry.IsSidechain {
			continue
		}
		if entry.Message == nil {
			continue
		}
		var blocks []transcriptContentBlock
		if err := json.Unmarshal(entry.Message.Content, &blocks); err != nil {
			continue
		}
		text := joinTextBlocks(blocks)
		if text == "" {
			continue
		}

		if extractKbBlock(text) {
			blockCount++
		}
		if hasDecisionLanguage(text) {
			decisionTurnCount++
		}
	}
	return blockCount, decisionTurnCount
}

// hookHandlePreCompact is the PreCompact-hook observability breadcrumb, a
// native port of plugin/scripts/pre-compact.js, wired as a
// hooks.Handler[hooks.PreCompactPayload]. IMPORTANT — behavior-faithful
// reading of pre-compact.js: the JS never persists anything (no kb write —
// it only calls a read-only queryKb) and never emits any user-visible
// output (no stdout, no stderr) on ANY branch; every path ends in
// logHookEvent (an internal ~/.ouroboros/hooks.log JSONL breadcrumb) and
// exit 0. This is a pure observability signal, not a persist-safety-net and
// not a warn/block hook — it must NEVER surface a Block or AdditionalContext
// and must NEVER call kb.WriteBatch. A withDB failure is likewise logged
// (not surfaced) and the zero hooks.Response ("silent allow") is always
// returned, matching pre-compact.js's catch-all exit(0).
func hookHandlePreCompact(_ context.Context, _ io.Reader, p hooks.PreCompactPayload) hooks.Response {
	if err := withDB(func(db *sql.DB) error {
		runHookPreCompact(p, db)
		return nil
	}); err != nil {
		logHookEvent(map[string]any{"hook": "pre_compact", "kind": "error", "session_id": p.SessionID, "detail": err.Error()})
	}
	return hooks.Response{}
}

// runHookPreCompact implements the PreCompact-hook flow. Every branch ends
// in a logHookEvent call and no branch ever returns anything but the zero
// hooks.Response — see hookHandlePreCompact's doc comment for why. The
// return value exists only for parity with the other hook handlers'
// testable-inner-function shape; it is always hooks.Response{}.
func runHookPreCompact(p hooks.PreCompactPayload, db *sql.DB) hooks.Response { //nolint:unparam // always the zero Response by design (pre-compact.js is a pure log-only observability breadcrumb: never blocks, never emits stdout/stderr, never persists)
	// Log fire event at entry, before any early-exit logic (including
	// project resolution) — mirrors pre-compact.js:9-11's explicit
	// ordering comment (log fire before any early-exit logic; enables
	// grepping hooks.log for fire/allow/error sequences). The JS logs fire
	// before it knows the project, so this breadcrumb omits it too.
	logHookEvent(map[string]any{"hook": "pre_compact", "kind": "fire"})

	// Project resolution is hub-aware (OU-301): resolveHookProject prefers
	// CLAUDE_PROJECT_DIR else cwd, excluding the marketplace hub, matching
	// UserPromptSubmit/SubagentStart/Stop/SubagentStop. This hook never
	// writes to the KB — the resolved project only affects which project
	// name this handler's log breadcrumbs carry.
	project := resolveHookProject(p.ProjectDir, p.Cwd)

	if p.TranscriptPath == "" {
		logHookEvent(map[string]any{"hook": "pre_compact", "kind": "skip", "reason": "no_transcript"})
		return hooks.Response{}
	}

	if project == "" {
		logHookEvent(map[string]any{"hook": "pre_compact", "kind": "skip", "reason": "no_project"})
		return hooks.Response{}
	}

	blockCount, decisionTurnCount := extractAllKbBlocks(p.TranscriptPath)

	if blockCount == 0 {
		runPreCompactNoBlocks(p, db, project, decisionTurnCount)
		return hooks.Response{}
	}

	runPreCompactWithBlocks(p, db, project, blockCount)
	return hooks.Response{}
}

// runPreCompactNoBlocks implements the "no kb-blocks in transcript" branch:
// check persisted docs via session_id first (persisted via the tool path),
// else fall back to the decision-language heuristic (advisory-only,
// compaction is never blocked on it).
func runPreCompactNoBlocks(p hooks.PreCompactPayload, db *sql.DB, project string, decisionTurnCount int) {
	if p.SessionID != "" {
		persistedCount, err := queryPersistedCount(db, project, p.SessionID)
		if err == nil && persistedCount > 0 {
			logHookEvent(map[string]any{
				"hook": "pre_compact", "kind": "allow", "project": project,
				"reason": "persisted_via_tool", "persisted_count": persistedCount, "session_id": p.SessionID,
			})
			return
		}
		// Query failed or no docs found: fall through to heuristic.
	}

	reason := "no_decisions"
	if decisionTurnCount >= preCompactDecisionThreshold {
		reason = "decisions_unpersisted_advisory"
	}
	trigger := p.Trigger
	if trigger == "" {
		trigger = "manual"
	}
	logHookEvent(map[string]any{
		"hook": "pre_compact", "kind": "allow", "project": project,
		"reason": reason, "trigger": trigger, "decision_turns": decisionTurnCount, "threshold": preCompactDecisionThreshold,
	})
}

// runPreCompactWithBlocks implements the "kb-blocks present" branch: precise
// session_id diffing against the persisted count.
func runPreCompactWithBlocks(p hooks.PreCompactPayload, db *sql.DB, project string, blockCount int) {
	if p.SessionID == "" {
		logHookEvent(map[string]any{
			"hook": "pre_compact", "kind": "allow", "project": project,
			"reason": "no_session_id", "block_count": blockCount,
		})
		return
	}

	persistedCount, err := queryPersistedCount(db, project, p.SessionID)
	if err != nil {
		logHookEvent(map[string]any{
			"hook": "pre_compact", "kind": "allow", "project": project,
			"reason": "query_error", "block_count": blockCount,
		})
		return
	}

	if persistedCount >= blockCount {
		logHookEvent(map[string]any{
			"hook": "pre_compact", "kind": "allow", "project": project,
			"reason": "all_persisted", "block_count": blockCount, "persisted_count": persistedCount, "session_id": p.SessionID,
		})
		return
	}

	unpersisted := blockCount - persistedCount
	logHookEvent(map[string]any{
		"hook": "pre_compact", "kind": "unpersisted_advisory", "project": project,
		"reason": "unpersisted_blocks", "block_count": blockCount, "persisted_count": persistedCount,
		"unpersisted_count": unpersisted, "session_id": p.SessionID,
	})
}

// queryPersistedCount returns the number of documents persisted for project
// and sessionID, via a direct store.QueryDocuments call (no shell-out,
// replacing pre-compact.js's queryKb CLI round-trip). Faithful port of
// pre-compact.js's queryPersistedCount, fail-open: an error here means the
// caller must treat the query as failed and fall through, exactly like the
// JS's null-on-failure contract.
func queryPersistedCount(db *sql.DB, project, sessionID string) (int, error) {
	summaries, err := store.QueryDocuments(db, nil, []string{project}, nil, "", nil, preCompactPersistedQueryLimit, sessionID)
	if err != nil {
		return 0, err
	}
	return len(summaries), nil
}
