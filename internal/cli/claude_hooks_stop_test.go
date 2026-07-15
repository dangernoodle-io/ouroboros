package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dangernoodle-io/shesha/host/claudecode/hooks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/store"
)

// isolateHookLog redirects logHookEvent's ${HOME}/.ouroboros/hooks.log
// target at a throwaway temp dir so tests never touch the real user's log.
func isolateHookLog(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func writeTranscript(t *testing.T, lines []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644))
	return path
}

func assistantLine(text string) string {
	rec := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
		},
	}
	data, _ := json.Marshal(rec)
	return string(data)
}

func userLine(content any) string {
	rec := map[string]any{
		"type": "user",
		"message": map[string]any{
			"content": content,
		},
	}
	data, _ := json.Marshal(rec)
	return string(data)
}

func ouroborosToolUseAssistantLine() string {
	rec := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []map[string]any{{"type": "tool_use", "name": "mcp__ouroboros-mcp__kb", "input": map[string]any{}}},
		},
	}
	data, _ := json.Marshal(rec)
	return string(data)
}

// longEnough pads a message past the 80-char floor so it isn't dropped by
// the "too short to matter" guard.
func longEnough(s string) string {
	for len(s) < 90 {
		s += " padding"
	}
	return s
}

func gitProjectDir(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "acme-corp")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	return root
}

func TestRunHookStop_StopHookActive_NoOutput(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	p := hooks.StopPayload{
		Common:         hooks.Common{TranscriptPath: "/tmp/x", Cwd: "/tmp", SessionID: "abc"},
		StopHookActive: true,
	}
	assert.Equal(t, hooks.Response{}, runHookStop(p, db))
}

func TestRunHookStop_MissingTranscriptPath_NoOutput(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	p := hooks.StopPayload{Common: hooks.Common{Cwd: "/tmp", SessionID: "abc"}}
	assert.Equal(t, hooks.Response{}, runHookStop(p, db))
}

func TestRunHookStop_ShortMessage_NoOutput(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	transcript := writeTranscript(t, []string{assistantLine("too short")})
	p := hooks.StopPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: t.TempDir(), SessionID: "abc"}}
	assert.Equal(t, hooks.Response{}, runHookStop(p, db))
}

func TestRunHookStop_KbBlock_Persisted(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	message := longEnough("```kb\n{\"type\":\"decision\",\"category\":\"arch\",\"title\":\"Use PostgreSQL\",\"content\":\"performance\"}\n```\n")
	transcript := writeTranscript(t, []string{assistantLine(message)})
	p := hooks.StopPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: "sess1234"}}

	resp := runHookStop(p, db)
	assert.Equal(t, hooks.Response{}, resp, "kb block persisted, no nudge expected")

	summaries, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 10)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "Use PostgreSQL", summaries[0].Title)

	doc, err := store.GetDocument(db, summaries[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "hook:stop", doc.Metadata["source"])
	assert.Equal(t, "sess1234", doc.Metadata["session_id"])
}

func TestRunHookStop_KbBlockArray_AllPersisted(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	message := longEnough("```kb\n[{\"type\":\"fact\",\"title\":\"Fact A\",\"content\":\"a\"},{\"type\":\"fact\",\"title\":\"Fact B\",\"content\":\"b\"}]\n```\n")
	transcript := writeTranscript(t, []string{assistantLine(message)})
	p := hooks.StopPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: "sess5678"}}

	assert.Equal(t, hooks.Response{}, runHookStop(p, db))

	summaries, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 10)
	require.NoError(t, err)
	assert.Len(t, summaries, 2)
}

func TestRunHookStop_PersistSkillSentinel_Skipped(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	message := longEnough("```kb\n{\"type\":\"fact\",\"title\":\"Already Done\",\"content\":\"x\",\"_persisted_by\":\"persist-skill\"}\n```\n")
	transcript := writeTranscript(t, []string{assistantLine(message)})
	p := hooks.StopPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: "sess0001"}}

	assert.Equal(t, hooks.Response{}, runHookStop(p, db))

	summaries, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 10)
	require.NoError(t, err)
	assert.Empty(t, summaries)
}

func TestRunHookStop_MalformedKbJSON_HandledNoWrite(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	message := longEnough("```kb\nnot valid json at all here\n```\n")
	transcript := writeTranscript(t, []string{assistantLine(message)})
	p := hooks.StopPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: "sess0002"}}

	resp := runHookStop(p, db)
	assert.NotEmpty(t, resp.SystemMessage, "malformed block must warn via SystemMessage, not silent stderr-only")
	assert.Empty(t, resp.Block, "a malformed block must never Block")

	summaries, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 10)
	require.NoError(t, err)
	assert.Empty(t, summaries)
}

// TestRunHookStop_MidTurnKbBlock_Persisted is the regression test for the
// OU-316 silent-loss bug: a well-formed ```kb block emitted in a NON-FINAL
// assistant message of the turn (the WiFi-FSM scenario) must actually be
// persisted, not silently dropped while turnAlreadyPersisted correctly
// detects it and would otherwise suppress the tier-1 nudge with no other
// signal.
func TestRunHookStop_MidTurnKbBlock_Persisted(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	midTurnBlock := "```kb\n{\"type\":\"decision\",\"category\":\"arch\",\"title\":\"WiFi FSM Retry Backoff\",\"content\":\"exponential backoff\"}\n```"
	finalMessage := longEnough("Summary of the work done this turn, no kb block here.")
	transcript := writeTranscript(t, []string{
		userLine("kick off the turn"),
		assistantLine(midTurnBlock),
		assistantLine(finalMessage),
	})
	p := hooks.StopPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: "sess0010"}}

	resp := runHookStop(p, db)
	assert.Equal(t, hooks.Response{}, resp, "mid-turn block persisted cleanly, no nudge/warning expected")

	summaries, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 10)
	require.NoError(t, err)
	require.Len(t, summaries, 1, "the mid-turn kb block must be persisted, not silently dropped")
	assert.Equal(t, "WiFi FSM Retry Backoff", summaries[0].Title)
}

// TestRunHookStop_MidTurnBlockPlusFinalTier2SelfClaim_NeverSuppressed is the
// OU-316 HIGH-regression test: the whole-turn kb persist (a DATA fix) must
// NOT short-circuit the nudge decision on an UNRELATED final message. A
// mid-turn kb block persists cleanly, and the FINAL message (with no block
// of its own) contains a tier-2 self-claim — the self-claim nudge must
// STILL fire, exactly as it would if the mid-turn block had never existed.
func TestRunHookStop_MidTurnBlockPlusFinalTier2SelfClaim_NeverSuppressed(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	midTurnBlock := "```kb\n{\"type\":\"decision\",\"category\":\"arch\",\"title\":\"Mid Turn Decision\",\"content\":\"x\"}\n```"
	finalMessage := longEnough("[ouroboros] main abcd1234: persisted 1 entries to acme-corp [ids: 1]")
	transcript := writeTranscript(t, []string{
		userLine("kick off the turn"),
		assistantLine(midTurnBlock),
		assistantLine(finalMessage),
	})
	p := hooks.StopPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: "sess0013"}}

	resp := runHookStop(p, db)
	assert.NotEmpty(t, resp.Block, "tier-2 self-claim must fire even though an unrelated mid-turn block was already persisted")
	assert.Contains(t, resp.Block, "tier-2")

	summaries, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 10)
	require.NoError(t, err)
	require.Len(t, summaries, 1, "the mid-turn kb block must still be persisted — the data fix stays intact")
	assert.Equal(t, "Mid Turn Decision", summaries[0].Title)
}

// TestRunHookStop_MidTurnBlockPlusFinalTier1Decision_Suppressed pins the
// companion case: a mid-turn kb block plus a FINAL message with tier-1
// decision language (no self-claim, no block) — turnAlreadyPersisted still
// detects the mid-turn block as an already-persisted signal and suppresses
// ONLY the tier-1 nudge, exactly like the pre-OU-316 behavior.
func TestRunHookStop_MidTurnBlockPlusFinalTier1Decision_Suppressed(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	midTurnBlock := "```kb\n{\"type\":\"decision\",\"category\":\"arch\",\"title\":\"Mid Turn Decision Two\",\"content\":\"x\"}\n```"
	finalMessage := longEnough("We decided to use PostgreSQL for the new service.")
	transcript := writeTranscript(t, []string{
		userLine("kick off the turn"),
		assistantLine(midTurnBlock),
		assistantLine(finalMessage),
	})
	p := hooks.StopPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: "sess0014"}}

	resp := runHookStop(p, db)
	assert.Equal(t, hooks.Response{}, resp, "tier-1 nudge suppressed: turnAlreadyPersisted detects the mid-turn block")

	summaries, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 10)
	require.NoError(t, err)
	require.Len(t, summaries, 1, "the mid-turn kb block must still be persisted")
	assert.Equal(t, "Mid Turn Decision Two", summaries[0].Title)
}

// TestRunHookStop_MultipleKbBlocksAcrossTurn_AllPersisted pins that every
// fenced kb block across multiple assistant messages of the same turn is
// persisted, not just the first or the final one.
func TestRunHookStop_MultipleKbBlocksAcrossTurn_AllPersisted(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	first := "```kb\n{\"type\":\"fact\",\"title\":\"Fact One\",\"content\":\"a\"}\n```"
	second := "```kb\n{\"type\":\"fact\",\"title\":\"Fact Two\",\"content\":\"b\"}\n```"
	third := longEnough("```kb\n{\"type\":\"fact\",\"title\":\"Fact Three\",\"content\":\"c\"}\n```\n")
	transcript := writeTranscript(t, []string{
		userLine("kick off the turn"),
		assistantLine(first),
		assistantLine(second),
		assistantLine(third),
	})
	p := hooks.StopPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: "sess0011"}}

	resp := runHookStop(p, db)
	assert.Equal(t, hooks.Response{}, resp)

	summaries, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 10)
	require.NoError(t, err)
	assert.Len(t, summaries, 3)
}

// TestRunHookStop_SanctionedFlow_ToolUseMidTurnPlusFinalSentinel pins the
// sanctioned persist-skill flow: the kb tool is called mid-turn and the
// final message emits only the anti-double-write sentinel block. No double
// write, and no spurious nudge (the sentinel block still counts as
// foundAnyBlock, so the Stop hook never falls through to checkNudgePatterns
// on this message).
func TestRunHookStop_SanctionedFlow_ToolUseMidTurnPlusFinalSentinel(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	finalMessage := longEnough("```kb\n{\"type\":\"fact\",\"title\":\"Already Done\",\"content\":\"x\",\"_persisted_by\":\"persist-skill\"}\n```\nWe decided to use PostgreSQL for the new service.\n")
	transcript := writeTranscript(t, []string{
		userLine("kick off the turn"),
		ouroborosToolUseAssistantLine(),
		assistantLine(finalMessage),
	})
	p := hooks.StopPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: "sess0012"}}

	resp := runHookStop(p, db)
	assert.Equal(t, hooks.Response{}, resp, "sentinel-only block: no warning, no nudge fallthrough")

	summaries, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 10)
	require.NoError(t, err)
	assert.Empty(t, summaries, "sentinel must skip persistence — no double-write")
}

func TestRunHookStop_Tier1Nudge_NotSuppressed(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	message := longEnough("We decided to use PostgreSQL for the new service.")
	transcript := writeTranscript(t, []string{
		userLine("kick off the turn"),
		assistantLine(message),
	})
	p := hooks.StopPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: "sess0003"}}

	resp := runHookStop(p, db)
	assert.Contains(t, resp.Block, "tier-1")
}

func TestRunHookStop_Tier1Nudge_SuppressedWhenAlreadyPersistedThisTurn(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	message := longEnough("We decided to use PostgreSQL for the new service.")
	transcript := writeTranscript(t, []string{
		userLine("kick off the turn"),
		ouroborosToolUseAssistantLine(),
		assistantLine(message),
	})
	p := hooks.StopPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: "sess0004"}}

	resp := runHookStop(p, db)
	assert.Equal(t, hooks.Response{}, resp, "tier-1 nudge should be suppressed when already persisted this turn")
}

func TestRunHookStop_Tier2Nudge_NeverSuppressed(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	message := longEnough("[ouroboros] main abcd1234: persisted 2 entries to acme-corp [ids: 1,2]")
	transcript := writeTranscript(t, []string{
		userLine("kick off the turn"),
		ouroborosToolUseAssistantLine(),
		assistantLine(message),
	})
	p := hooks.StopPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: "sess0005"}}

	resp := runHookStop(p, db)
	assert.Contains(t, resp.Block, "tier-2")
}

func TestRunHookStop_ExploratoryMessage_NoOutput(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	message := longEnough("Here is a summary of the files I looked at while exploring the codebase.")
	transcript := writeTranscript(t, []string{assistantLine(message)})
	p := hooks.StopPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: "sess0006"}}

	assert.Equal(t, hooks.Response{}, runHookStop(p, db))
}

func TestHookHandleStop_DBOpenFailure_SilentAllow(t *testing.T) {
	isolateHookLog(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Make the DB's parent directory creation fail: point PROJECT_KB_PATH at
	// a path whose parent is a plain file, so store.InitDB's MkdirAll errors
	// and withDB returns a non-nil error.
	blocker := filepath.Join(home, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	t.Setenv("PROJECT_KB_PATH", filepath.Join(blocker, "sub", "kb.db"))
	t.Setenv("QM_DB_PATH", "")

	p := hooks.StopPayload{Common: hooks.Common{Cwd: "/tmp", SessionID: "abc"}}
	resp := hookHandleStop(t.Context(), strings.NewReader(""), p)
	assert.Equal(t, hooks.Response{}, resp, "a withDB failure must silently allow, never block")
}

// --- helper unit tests ---------------------------------------------------

func TestProjectFromPath_FindsGitRootNLevelsUp(t *testing.T) {
	root := filepath.Join(t.TempDir(), "myproject")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	nested := filepath.Join(root, "a", "b", "c")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	assert.Equal(t, "myproject", projectFromPath(nested))
}

func TestProjectFromPath_NoGitRoot(t *testing.T) {
	dir := t.TempDir()
	assert.Equal(t, "", projectFromPath(filepath.Join(dir, "a", "b")))
}

func TestProjectFromPath_EmptyStartPath(t *testing.T) {
	assert.Equal(t, "", projectFromPath(""))
}

func TestReadLastMainAssistantText_SkipsSidechainAndPicksNewest(t *testing.T) {
	// The scan runs backwards, so the sidechain entry to be skipped must be
	// the MOST RECENT (last) line, with the expected match earlier.
	sidechain := `{"type":"assistant","isSidechain":true,"message":{"content":[{"type":"text","text":"sidechain text"}]}}`
	transcript := writeTranscript(t, []string{
		assistantLine("first main message"),
		assistantLine("second main message"),
		sidechain,
	})
	assert.Equal(t, "second main message", readLastMainAssistantText(transcript))
}

func TestReadLastMainAssistantText_MissingFile(t *testing.T) {
	assert.Equal(t, "", readLastMainAssistantText(filepath.Join(t.TempDir(), "nope.jsonl")))
}

func TestTurnAlreadyPersisted_TrueOnOuroborosToolUse(t *testing.T) {
	transcript := writeTranscript(t, []string{
		userLine("start turn"),
		ouroborosToolUseAssistantLine(),
	})
	assert.True(t, turnAlreadyPersisted(transcript))
}

func TestTurnAlreadyPersisted_TrueOnPriorKbFence(t *testing.T) {
	transcript := writeTranscript(t, []string{
		userLine("start turn"),
		assistantLine("```kb\n{\"type\":\"fact\",\"title\":\"x\",\"content\":\"y\"}\n```"),
	})
	assert.True(t, turnAlreadyPersisted(transcript))
}

func TestTurnAlreadyPersisted_FalseWhenNoSignalThisTurn(t *testing.T) {
	transcript := writeTranscript(t, []string{
		userLine("start turn"),
		assistantLine("just a plain message"),
	})
	assert.False(t, turnAlreadyPersisted(transcript))
}

func TestTurnAlreadyPersisted_MissingFile(t *testing.T) {
	assert.False(t, turnAlreadyPersisted(filepath.Join(t.TempDir(), "nope.jsonl")))
}

func TestTurnAlreadyPersisted_StopsAtGenuineUserBoundary(t *testing.T) {
	// The tool_use is from a PRIOR turn (before the genuine user boundary),
	// so it must not count for the current turn.
	transcript := writeTranscript(t, []string{
		ouroborosToolUseAssistantLine(),
		userLine("a new genuine user prompt"),
		assistantLine("just a plain message"),
	})
	assert.False(t, turnAlreadyPersisted(transcript))
}

func TestTurnAlreadyPersisted_ToolResultDoesNotBoundary(t *testing.T) {
	// A tool_result "user" record is a synthetic pseudo-turn, not a genuine
	// boundary — scanning must continue past it into the same turn.
	transcript := writeTranscript(t, []string{
		userLine("a genuine user prompt"),
		userLine([]map[string]any{{"type": "tool_result", "content": "ok"}}),
		ouroborosToolUseAssistantLine(),
	})
	assert.True(t, turnAlreadyPersisted(transcript))
}

func TestTurnAlreadyPersisted_LargeFileUsesTailCap(t *testing.T) {
	lines := []string{userLine("start turn")}
	padding := strings.Repeat("x", 1024)
	for i := 0; i < 2200; i++ {
		lines = append(lines, assistantLine(padding))
	}
	lines = append(lines, ouroborosToolUseAssistantLine())
	transcript := writeTranscript(t, lines)

	info, err := os.Stat(transcript)
	require.NoError(t, err)
	require.Greater(t, info.Size(), int64(maxHookTailBytes))

	assert.True(t, turnAlreadyPersisted(transcript))
}

func TestTurnAlreadyPersisted_FalseWhenOnlySignalIsSidechainToolUse(t *testing.T) {
	// A sidechain (subagent) tool_use is not a main-context signal — it must
	// not suppress the main turn's nudge; persisting it is SubagentStop's job.
	sidechainToolUse := `{"type":"assistant","isSidechain":true,"message":{"content":[{"type":"tool_use","name":"mcp__ouroboros-mcp__kb","input":{}}]}}`
	transcript := writeTranscript(t, []string{
		userLine("start turn"),
		sidechainToolUse,
	})
	assert.False(t, turnAlreadyPersisted(transcript))
}

func TestTurnAlreadyPersisted_FalseWhenOnlySignalIsSidechainKbFence(t *testing.T) {
	rec := map[string]any{
		"type":        "assistant",
		"isSidechain": true,
		"message": map[string]any{
			"content": []map[string]any{{
				"type": "text",
				"text": "```kb\n{\"type\":\"fact\",\"title\":\"x\",\"content\":\"y\"}\n```",
			}},
		},
	}
	data, err := json.Marshal(rec)
	require.NoError(t, err)
	transcript := writeTranscript(t, []string{
		userLine("start turn"),
		string(data),
	})
	assert.False(t, turnAlreadyPersisted(transcript))
}

// --- isGenuineUserTurnBoundary ------------------------------------------

func TestIsGenuineUserTurnBoundary(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{"empty", ``, true},
		{"string content", `"a plain string"`, true},
		{"non-array object", `{"foo":"bar"}`, true},
		{"empty array", `[]`, true},
		{"malformed array", `[not valid`, true},
		{"all tool_result", `[{"type":"tool_result","content":"ok"}]`, false},
		{"mixed blocks", `[{"type":"tool_result","content":"ok"},{"type":"text","text":"hi"}]`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isGenuineUserTurnBoundary(json.RawMessage(tc.content)))
		})
	}
}

// --- isOuroborosWriteTool -------------------------------------------------

func TestIsOuroborosWriteTool(t *testing.T) {
	cases := []struct {
		name string
		tool string
		want bool
	}{
		{"empty", "", false},
		{"no ouroboros marker", "mcp__github__create_issue", false},
		{"ouroboros but wrong suffix", "mcp__ouroboros-mcp__unrelated", false},
		{"ouroboros kb tool", "mcp__ouroboros-mcp__kb", true},
		{"ouroboros backlog tool", "mcp__ouroboros-mcp__backlog", true},
		{"ouroboros legacy put", "mcp__ouroboros-mcp__put", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isOuroborosWriteTool(tc.tool))
		})
	}
}

// --- truncateMessage -------------------------------------------------------

func TestTruncateMessage_ShortUnchanged(t *testing.T) {
	assert.Equal(t, "hello", truncateMessage("hello", 5000))
}

func TestTruncateMessage_TruncatesAtByteBoundary(t *testing.T) {
	s := strings.Repeat("a", 6000)
	out := truncateMessage(s, 5000)
	assert.Len(t, out, 5000)
}

func TestTruncateMessage_BacksOffPartialRune(t *testing.T) {
	// "é" is 2 bytes (0xC3 0xA9); place one straddling the cut point so the
	// naive byte slice would split it.
	s := strings.Repeat("a", 4999) + "é" + strings.Repeat("b", 10)
	out := truncateMessage(s, 5000)
	assert.True(t, utf8.ValidString(out))
	assert.LessOrEqual(t, len(out), 5000)
}

// --- readLastMainAssistantText edge cases ---------------------------------

func TestReadLastMainAssistantText_NoMessageField(t *testing.T) {
	transcript := writeTranscript(t, []string{`{"type":"assistant"}`})
	assert.Equal(t, "", readLastMainAssistantText(transcript))
}

func TestReadLastMainAssistantText_NonArrayContentSkipped(t *testing.T) {
	transcript := writeTranscript(t, []string{
		assistantLine("the real message here"),
		`{"type":"assistant","message":{"content":"not-an-array"}}`,
	})
	assert.Equal(t, "the real message here", readLastMainAssistantText(transcript))
}

func TestReadLastMainAssistantText_MalformedLineSkipped(t *testing.T) {
	// Malformed line must be the MOST RECENT (last) one — the scan runs
	// backwards, so a valid line placed last would return before the parse
	// error branch is ever reached.
	transcript := writeTranscript(t, []string{
		assistantLine("the real message"),
		"not json at all",
	})
	assert.Equal(t, "the real message", readLastMainAssistantText(transcript))
}

func TestReadLastMainAssistantText_LargeFileUsesTailCap(t *testing.T) {
	// The target message sits at end-of-file, so it's found whether or not
	// the cap math is right; this only pins that a >cap file is handled
	// without error and the tail message is still found (position-invariant
	// — see TestReadLastMainAssistantText_PartialLeadingLineDropped below for
	// a test that actually pins the drop-the-partial-line behavior).
	lines := []string{userLine("start turn")}
	padding := strings.Repeat("x", 1024)
	for i := 0; i < 2200; i++ {
		lines = append(lines, assistantLine(padding))
	}
	lines = append(lines, assistantLine("the real message"))
	transcript := writeTranscript(t, lines)

	info, err := os.Stat(transcript)
	require.NoError(t, err)
	require.Greater(t, info.Size(), int64(maxHookTailBytes))

	assert.Equal(t, "the real message", readLastMainAssistantText(transcript))
}

func TestReadLastMainAssistantText_PartialLeadingLineDropped(t *testing.T) {
	// Pin that the leading-partial-line drop in readTranscriptTail is
	// load-bearing, not just crash-safe. The fixture is built so the ONLY
	// main-context assistant record the backward scan could ever match is
	// the truncated leading fragment itself:
	//
	//   file = junkPrefix + wrongJSON + "\n" + garbageTail + "\n"
	//
	// junkPrefix's length is chosen (arithmetically, not by trial) so the
	// tail-cap window starts EXACTLY at the byte where wrongJSON begins.
	// garbageTail's length is chosen so that window ends exactly at EOF.
	// That means the captured tail is precisely: wrongJSON + "\n" +
	// garbageTail + "\n" — garbageTail never parses as JSON, so it's always
	// skipped.
	//
	// - With the drop present: the tail's leading line (wrongJSON) is
	//   stripped as a presumed-partial fragment, leaving only garbageTail —
	//   no assistant record survives, so the function returns "".
	// - Without the drop (mutation-tested below): wrongJSON is kept and,
	//   since junkPrefix was stripped away by the cap itself, happens to be
	//   valid standalone JSON — a fully-formed (wrong) assistant record that
	//   the backward scan would match and return.
	wrongJSON := assistantLine("WRONG: leaked partial-line fragment")
	innerLen := len(wrongJSON)
	require.Less(t, innerLen+2, maxHookTailBytes, "fixture assumes wrongJSON is small relative to the cap")

	junkPrefix := strings.Repeat("j", 4096)
	afterLen := maxHookTailBytes - innerLen - 2
	garbageTail := strings.Repeat("z", afterLen)

	transcript := writeTranscript(t, []string{junkPrefix + wrongJSON, garbageTail})

	info, err := os.Stat(transcript)
	require.NoError(t, err)
	require.Greater(t, info.Size(), int64(maxHookTailBytes))
	require.EqualValues(t, len(junkPrefix), info.Size()-int64(maxHookTailBytes),
		"fixture arithmetic must land the tail-cap boundary exactly at wrongJSON's start")

	assert.Equal(t, "", readLastMainAssistantText(transcript),
		"the partial leading fragment must be dropped, never mis-parsed into a wrong result")
}

// --- readTranscriptTail error paths -----------------------------------------

func TestReadTranscriptTail_SubCapReadFileErrorReturnsNil(t *testing.T) {
	// A directory stat-succeeds (size < cap) but os.ReadFile on it fails
	// (EISDIR) — exercises the small-file os.ReadFile error branch.
	dir := t.TempDir()
	assert.Nil(t, readTranscriptTail(dir))
}

func TestReadTranscriptTail_NoNewlineInCappedTailKeepsWholeTail(t *testing.T) {
	// A file larger than the cap whose capped tail window contains no
	// newline at all: the idx<0 branch must keep the whole tail rather
	// than drop it as a presumed-partial leading line.
	body := strings.Repeat("y", maxHookTailBytes+4096)
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Greater(t, info.Size(), int64(maxHookTailBytes))

	lines := readTranscriptTail(path)
	require.Len(t, lines, 1)
	assert.Equal(t, strings.Repeat("y", maxHookTailBytes), string(lines[0]))
}

func TestReadTranscriptTail_OverCapOpenErrorReturnsNil(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: chmod 0o000 does not deny read access")
	}
	// A file larger than the cap that os.Stat can see but os.Open cannot
	// read (permission denied) — exercises the over-cap os.Open error
	// branch.
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	body := strings.Repeat("y", maxHookTailBytes+4096)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	require.NoError(t, os.Chmod(path, 0o000))
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	assert.Nil(t, readTranscriptTail(path))
}

// --- findGitRoot / projectFromPath -----------------------------------------

func TestFindGitRoot_StartPathIsFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "myproject")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	file := filepath.Join(root, "some-file.txt")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))

	assert.Equal(t, root, findGitRoot(file))
}

func TestFindGitRoot_NonexistentStartPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "myproject")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	missing := filepath.Join(root, "does", "not", "exist.txt")

	assert.Equal(t, root, findGitRoot(missing))
}

// --- logHookEvent ------------------------------------------------------

func TestLogHookEvent_WritesJSONLToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	logHookEvent(map[string]any{"hook": "stop", "kind": "noop"})

	data, err := os.ReadFile(filepath.Join(home, ".ouroboros", "hooks.log"))
	require.NoError(t, err)
	var entry map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(data), &entry))
	assert.Equal(t, "stop", entry["hook"])
	assert.Equal(t, "noop", entry["kind"])
	assert.Contains(t, entry, "ts")
}

func TestLogHookEvent_GatedOff(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OUROBOROS_HOOK_LOG", "off")

	logHookEvent(map[string]any{"hook": "stop", "kind": "noop"})

	_, err := os.Stat(filepath.Join(home, ".ouroboros", "hooks.log"))
	assert.True(t, os.IsNotExist(err))
}

func TestRunHookStop_KbBlock_NoProject_NotPersisted(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	// Not a git repo — projectFromPath resolves to "".
	cwd := t.TempDir()
	message := longEnough("```kb\n{\"type\":\"fact\",\"title\":\"Orphan\",\"content\":\"x\"}\n```\n")
	transcript := writeTranscript(t, []string{assistantLine(message)})
	p := hooks.StopPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: "sess0007"}}

	resp := runHookStop(p, db)
	assert.NotEmpty(t, resp.SystemMessage, "a kb block with no resolvable project must warn, not silently drop")
	assert.Empty(t, resp.Block)

	summaries, err := store.QueryDocuments(db, nil, nil, nil, "", nil, 10)
	require.NoError(t, err)
	assert.Empty(t, summaries)
}

func TestRunHookStop_KbBlock_WriteBatchError_HandledNoNudge(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	message := longEnough("```kb\n{\"type\":\"not-a-real-type\",\"title\":\"Bad\",\"content\":\"x\"}\n```\n")
	transcript := writeTranscript(t, []string{assistantLine(message)})
	p := hooks.StopPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: "sess0008"}}

	resp := runHookStop(p, db)
	assert.NotEmpty(t, resp.SystemMessage, "a kb.WriteBatch failure must warn, not silently drop")
	assert.Empty(t, resp.Block, "write failure never falls through to the decision-language nudge")

	summaries, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 10)
	require.NoError(t, err)
	assert.Empty(t, summaries)
}

func TestTurnAlreadyPersisted_MalformedLineSkipped(t *testing.T) {
	// Malformed line must be the MOST RECENT (last) one — the scan runs
	// backwards, so a signal placed last would return true before the parse
	// error branch is ever reached.
	transcript := writeTranscript(t, []string{
		userLine("start turn"),
		ouroborosToolUseAssistantLine(),
		"not json at all",
	})
	assert.True(t, turnAlreadyPersisted(transcript))
}

func TestReadLastMainAssistantText_NullMessage(t *testing.T) {
	transcript := writeTranscript(t, []string{
		assistantLine("the real message"),
		`{"type":"assistant","message":null}`,
	})
	assert.Equal(t, "the real message", readLastMainAssistantText(transcript))
}

func TestLogHookEvent_NoHomeDir(t *testing.T) {
	t.Setenv("HOME", "")
	// Must not panic and must swallow the UserHomeDir error silently.
	logHookEvent(map[string]any{"hook": "stop", "kind": "noop"})
}

func TestLogHookEvent_MkdirAllError(t *testing.T) {
	home := t.TempDir()
	// Pre-create ~/.ouroboros as a plain FILE so MkdirAll fails.
	require.NoError(t, os.WriteFile(filepath.Join(home, ".ouroboros"), []byte("x"), 0o644))
	t.Setenv("HOME", home)

	// Must not panic; MkdirAll error is swallowed.
	logHookEvent(map[string]any{"hook": "stop", "kind": "noop"})
}

func TestLogHookEvent_OpenFileError(t *testing.T) {
	home := t.TempDir()
	// Pre-create the log path itself as a DIRECTORY so OpenFile fails.
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".ouroboros", "hooks.log"), 0o755))
	t.Setenv("HOME", home)

	// Must not panic; OpenFile error is swallowed.
	logHookEvent(map[string]any{"hook": "stop", "kind": "noop"})
}

func TestLogHookEvent_MarshalError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// A channel value cannot be JSON-marshaled; must not panic.
	logHookEvent(map[string]any{"bad": make(chan int)})

	_, err := os.Stat(filepath.Join(home, ".ouroboros", "hooks.log"))
	assert.True(t, os.IsNotExist(err), "marshal failure must not produce a log line")
}

// --- joinTextBlocks --------------------------------------------------------

func TestJoinTextBlocks_NoTextBlocks(t *testing.T) {
	assert.Equal(t, "", joinTextBlocks([]transcriptContentBlock{{Type: "tool_use", Name: "x"}}))
}

func TestJoinTextBlocks_MultipleTextBlocksJoined(t *testing.T) {
	blocks := []transcriptContentBlock{
		{Type: "text", Text: "first"},
		{Type: "tool_use", Name: "x"},
		{Type: "text", Text: "second"},
	}
	assert.Equal(t, "first\nsecond", joinTextBlocks(blocks))
}

func TestReadLastMainAssistantText_SkipsToolUseOnlyTurn(t *testing.T) {
	transcript := writeTranscript(t, []string{
		assistantLine("the real message"),
		ouroborosToolUseAssistantLine(),
	})
	assert.Equal(t, "the real message", readLastMainAssistantText(transcript))
}

// --- turnAlreadyPersisted additional branches ------------------------------

func TestTurnAlreadyPersisted_ContinuesPastToolResultToEarlierSignal(t *testing.T) {
	transcript := writeTranscript(t, []string{
		ouroborosToolUseAssistantLine(),
		userLine([]map[string]any{{"type": "tool_result", "content": "ok"}}),
		assistantLine("just a plain message"),
	})
	assert.True(t, turnAlreadyPersisted(transcript))
}

func TestTurnAlreadyPersisted_SkipsNonAssistantNonUserLine(t *testing.T) {
	// The scan runs backwards, so the noise line to be skipped must be the
	// MOST RECENT (last) line, with the signal earlier.
	transcript := writeTranscript(t, []string{
		userLine("start turn"),
		ouroborosToolUseAssistantLine(),
		`{"type":"system","message":{"content":"noise"}}`,
	})
	assert.True(t, turnAlreadyPersisted(transcript))
}

func TestTurnAlreadyPersisted_SkipsAssistantWithNilMessage(t *testing.T) {
	transcript := writeTranscript(t, []string{
		userLine("start turn"),
		ouroborosToolUseAssistantLine(),
		`{"type":"assistant"}`,
	})
	assert.True(t, turnAlreadyPersisted(transcript))
}

func TestTurnAlreadyPersisted_SkipsMalformedAssistantContent(t *testing.T) {
	transcript := writeTranscript(t, []string{
		userLine("start turn"),
		ouroborosToolUseAssistantLine(),
		`{"type":"assistant","message":{"content":"not-an-array"}}`,
	})
	assert.True(t, turnAlreadyPersisted(transcript))
}

func TestTurnAlreadyPersisted_ManyLinesExceedsScanCap(t *testing.T) {
	lines := []string{ouroborosToolUseAssistantLine()}
	for i := 0; i < maxHookScanLines+500; i++ {
		lines = append(lines, `{"type":"assistant","message":{"content":[{"type":"text","text":"noise"}]}}`)
	}
	transcript := writeTranscript(t, lines)

	info, err := os.Stat(transcript)
	require.NoError(t, err)
	require.Less(t, info.Size(), int64(maxHookTailBytes), "must stay under the tail-byte cap to isolate the line cap")

	// The signal line is older than the last maxHookScanLines lines, so it
	// must NOT be found — this exercises the line-count cap truncation.
	assert.False(t, turnAlreadyPersisted(transcript))
}

// --- runHookStop session-short formatting ----------------------------------

func TestRunHookStop_EmptySessionID_DefaultsToMain(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	message := longEnough("We decided to use PostgreSQL for the new service.")
	transcript := writeTranscript(t, []string{
		userLine("kick off the turn"),
		assistantLine(message),
	})
	p := hooks.StopPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd}}

	resp := runHookStop(p, db)
	assert.Contains(t, resp.Block, "main main:")
}

func TestRunHookStop_LongSessionID_Truncated(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	message := longEnough("We decided to use PostgreSQL for the new service.")
	transcript := writeTranscript(t, []string{
		userLine("kick off the turn"),
		assistantLine(message),
	})
	p := hooks.StopPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: "abcdefghijklmnop"}}

	resp := runHookStop(p, db)
	assert.Contains(t, resp.Block, "main abcdefgh:")
}

// --- parseKbBlockEntries malformed array ------------------------------------

func TestRunHookStop_MalformedKbArrayJSON_HandledNoWrite(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	message := longEnough("```kb\n[not valid json array\n```\n")
	transcript := writeTranscript(t, []string{assistantLine(message)})
	p := hooks.StopPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: "sess0009"}}

	resp := runHookStop(p, db)
	assert.NotEmpty(t, resp.SystemMessage, "malformed block must warn via SystemMessage, not silent stderr-only")
	assert.Empty(t, resp.Block, "a malformed block must never Block")

	summaries, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 10)
	require.NoError(t, err)
	assert.Empty(t, summaries)
}
