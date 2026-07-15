package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dangernoodle-io/shesha/host/claudecode/hooks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/store"
)

// readHookLogEntries reads every JSONL line from the isolated hooks.log
// (see isolateHookLog, defined in claude_hooks_stop_test.go, same package)
// under the given HOME dir, returning the decoded entries in file order.
func readHookLogEntries(t *testing.T, home string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".ouroboros", "hooks.log"))
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)

	var entries []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal(line, &entry))
		entries = append(entries, entry)
	}
	return entries
}

// lastHookLogEntry returns the last logged entry, or nil if none.
func lastHookLogEntry(t *testing.T, home string) map[string]any {
	t.Helper()
	entries := readHookLogEntries(t, home)
	if len(entries) == 0 {
		return nil
	}
	return entries[len(entries)-1]
}

func kbBlockMessage(title string) string {
	return "```kb\n{\"type\":\"fact\",\"title\":\"" + title + "\",\"content\":\"x\"}\n```\n"
}

func TestRunHookPreCompact_MissingTranscriptPath_SkipsNoTranscript(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	db := newTestDB(t)

	p := hooks.PreCompactPayload{Common: hooks.Common{Cwd: gitProjectDir(t), SessionID: "abc"}}
	resp := runHookPreCompact(p, db)

	assert.Equal(t, hooks.Response{}, resp)
	entry := lastHookLogEntry(t, home)
	require.NotNil(t, entry)
	assert.Equal(t, "skip", entry["kind"])
	assert.Equal(t, "no_transcript", entry["reason"])
}

func TestRunHookPreCompact_NoProject_SkipsNoProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	db := newTestDB(t)

	transcript := writeTranscript(t, []string{assistantLine("just a plain message")})
	// Not a git repo — projectFromPath resolves to "".
	p := hooks.PreCompactPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: t.TempDir(), SessionID: "abc"}}
	resp := runHookPreCompact(p, db)

	assert.Equal(t, hooks.Response{}, resp)
	entry := lastHookLogEntry(t, home)
	require.NotNil(t, entry)
	assert.Equal(t, "skip", entry["kind"])
	assert.Equal(t, "no_project", entry["reason"])
}

func TestRunHookPreCompact_NoKbBlocks_BelowThreshold_NoDecisions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	db := newTestDB(t)
	cwd := gitProjectDir(t)

	transcript := writeTranscript(t, []string{assistantLine("just an ordinary exploratory message")})
	p := hooks.PreCompactPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: ""}}
	resp := runHookPreCompact(p, db)

	assert.Equal(t, hooks.Response{}, resp)
	entry := lastHookLogEntry(t, home)
	require.NotNil(t, entry)
	assert.Equal(t, "allow", entry["kind"])
	assert.Equal(t, "no_decisions", entry["reason"])
	assert.Equal(t, "manual", entry["trigger"])
}

func TestRunHookPreCompact_NoKbBlocks_AtThreshold_DecisionsUnpersistedAdvisory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	db := newTestDB(t)
	cwd := gitProjectDir(t)

	transcript := writeTranscript(t, []string{
		assistantLine("we decided this"),
		assistantLine("chose this instead"),
		assistantLine("going with that"),
	})
	p := hooks.PreCompactPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: ""}, Trigger: "auto"}
	resp := runHookPreCompact(p, db)

	assert.Equal(t, hooks.Response{}, resp)
	entry := lastHookLogEntry(t, home)
	require.NotNil(t, entry)
	assert.Equal(t, "allow", entry["kind"])
	assert.Equal(t, "decisions_unpersisted_advisory", entry["reason"])
	assert.Equal(t, "auto", entry["trigger"])
	assert.EqualValues(t, 3, entry["decision_turns"])
}

func TestRunHookPreCompact_NoKbBlocks_SessionIDWithPersistedDocs_AllowsPersistedViaTool(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	project := filepath.Base(cwd)

	_, err := store.UpsertDocument(db, store.Document{
		Type: "fact", Project: project, Title: "Persisted Already", Content: "x", SessionID: "sess-1",
	})
	require.NoError(t, err)

	transcript := writeTranscript(t, []string{assistantLine("just an ordinary message")})
	p := hooks.PreCompactPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: "sess-1"}}
	resp := runHookPreCompact(p, db)

	assert.Equal(t, hooks.Response{}, resp)
	entry := lastHookLogEntry(t, home)
	require.NotNil(t, entry)
	assert.Equal(t, "allow", entry["kind"])
	assert.Equal(t, "persisted_via_tool", entry["reason"])
	assert.EqualValues(t, 1, entry["persisted_count"])
}

func TestRunHookPreCompact_NoKbBlocks_SessionIDNoPersistedDocs_FallsToHeuristic(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	db := newTestDB(t)
	cwd := gitProjectDir(t)

	transcript := writeTranscript(t, []string{assistantLine("just an ordinary message")})
	p := hooks.PreCompactPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: "sess-none"}}
	resp := runHookPreCompact(p, db)

	assert.Equal(t, hooks.Response{}, resp)
	entry := lastHookLogEntry(t, home)
	require.NotNil(t, entry)
	assert.Equal(t, "allow", entry["kind"])
	assert.Equal(t, "no_decisions", entry["reason"])
}

func TestRunHookPreCompact_KbBlocks_NoSessionID_AllowsNoSessionID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	db := newTestDB(t)
	cwd := gitProjectDir(t)

	transcript := writeTranscript(t, []string{assistantLine(kbBlockMessage("A"))})
	p := hooks.PreCompactPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: ""}}
	resp := runHookPreCompact(p, db)

	assert.Equal(t, hooks.Response{}, resp)
	entry := lastHookLogEntry(t, home)
	require.NotNil(t, entry)
	assert.Equal(t, "allow", entry["kind"])
	assert.Equal(t, "no_session_id", entry["reason"])
	assert.EqualValues(t, 1, entry["block_count"])
}

func TestRunHookPreCompact_KbBlocks_AllPersisted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	project := filepath.Base(cwd)

	_, err := store.UpsertDocument(db, store.Document{
		Type: "fact", Project: project, Title: "Already Persisted", Content: "x", SessionID: "sess-2",
	})
	require.NoError(t, err)

	transcript := writeTranscript(t, []string{assistantLine(kbBlockMessage("A"))})
	p := hooks.PreCompactPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: "sess-2"}}
	resp := runHookPreCompact(p, db)

	assert.Equal(t, hooks.Response{}, resp)
	entry := lastHookLogEntry(t, home)
	require.NotNil(t, entry)
	assert.Equal(t, "allow", entry["kind"])
	assert.Equal(t, "all_persisted", entry["reason"])
	assert.EqualValues(t, 1, entry["block_count"])
	assert.EqualValues(t, 1, entry["persisted_count"])
}

func TestRunHookPreCompact_KbBlocks_SomeUnpersisted_UnpersistedAdvisory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	db := newTestDB(t)
	cwd := gitProjectDir(t)

	// Two turns each with a kb block, but no matching persisted doc in the DB
	// for this session_id — the whole batch is unpersisted.
	transcript := writeTranscript(t, []string{
		assistantLine(kbBlockMessage("A")),
		assistantLine(kbBlockMessage("B")),
	})
	p := hooks.PreCompactPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: "sess-3"}}
	resp := runHookPreCompact(p, db)

	assert.Equal(t, hooks.Response{}, resp)
	entry := lastHookLogEntry(t, home)
	require.NotNil(t, entry)
	assert.Equal(t, "unpersisted_advisory", entry["kind"])
	assert.Equal(t, "unpersisted_blocks", entry["reason"])
	assert.EqualValues(t, 2, entry["block_count"])
	assert.EqualValues(t, 0, entry["persisted_count"])
	assert.EqualValues(t, 2, entry["unpersisted_count"])
}

// TestRunHookPreCompact_NeverWritesDocuments is a positive belt-and-suspenders
// guarantee (on top of the "unpersisted_advisory" reason assertion above)
// that this hook never persists: the documents table row count must be
// identical before and after runHookPreCompact runs, even when unpersisted
// kb-blocks are detected.
func TestRunHookPreCompact_NeverWritesDocuments(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	db := newTestDB(t)
	cwd := gitProjectDir(t)

	transcript := writeTranscript(t, []string{
		assistantLine(kbBlockMessage("A")),
		assistantLine(kbBlockMessage("B")),
	})
	p := hooks.PreCompactPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: "sess-3"}}

	var before int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM documents").Scan(&before))

	runHookPreCompact(p, db)

	var after int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM documents").Scan(&after))
	assert.Equal(t, before, after, "pre-compact hook must never write to documents")
}

func TestRunHookPreCompact_KbBlocks_PartiallyPersisted_UnpersistedAdvisory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	project := filepath.Base(cwd)

	_, err := store.UpsertDocument(db, store.Document{
		Type: "fact", Project: project, Title: "One Persisted", Content: "x", SessionID: "sess-4",
	})
	require.NoError(t, err)

	transcript := writeTranscript(t, []string{
		assistantLine(kbBlockMessage("A")),
		assistantLine(kbBlockMessage("B")),
	})
	p := hooks.PreCompactPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: "sess-4"}}
	resp := runHookPreCompact(p, db)

	assert.Equal(t, hooks.Response{}, resp)
	entry := lastHookLogEntry(t, home)
	require.NotNil(t, entry)
	assert.Equal(t, "unpersisted_advisory", entry["kind"])
	assert.EqualValues(t, 2, entry["block_count"])
	assert.EqualValues(t, 1, entry["persisted_count"])
	assert.EqualValues(t, 1, entry["unpersisted_count"])
}

func TestRunHookPreCompact_MalformedTranscript_SilentNoDecisions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	db := newTestDB(t)
	cwd := gitProjectDir(t)

	transcript := writeTranscript(t, []string{"not json at all", "{{{broken"})
	p := hooks.PreCompactPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: ""}}
	resp := runHookPreCompact(p, db)

	assert.Equal(t, hooks.Response{}, resp, "malformed transcript must never block; must resolve to the zero Response")
	entry := lastHookLogEntry(t, home)
	require.NotNil(t, entry)
	assert.Equal(t, "allow", entry["kind"])
	assert.Equal(t, "no_decisions", entry["reason"])
}

func TestRunHookPreCompact_MissingTranscriptFile_SilentNoDecisions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	db := newTestDB(t)
	cwd := gitProjectDir(t)

	p := hooks.PreCompactPayload{Common: hooks.Common{TranscriptPath: filepath.Join(t.TempDir(), "nope.jsonl"), Cwd: cwd, SessionID: ""}}
	resp := runHookPreCompact(p, db)

	assert.Equal(t, hooks.Response{}, resp)
	entry := lastHookLogEntry(t, home)
	require.NotNil(t, entry)
	assert.Equal(t, "allow", entry["kind"])
	assert.Equal(t, "no_decisions", entry["reason"])
}

func TestHookHandlePreCompact_Success_SilentAllow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PROJECT_KB_PATH", filepath.Join(home, "kb.db"))
	t.Setenv("QM_DB_PATH", "")

	transcript := writeTranscript(t, []string{assistantLine("just an ordinary message")})
	p := hooks.PreCompactPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: gitProjectDir(t), SessionID: "abc"}}
	resp := hookHandlePreCompact(t.Context(), strings.NewReader(""), p)

	assert.Equal(t, hooks.Response{}, resp)
	entry := lastHookLogEntry(t, home)
	require.NotNil(t, entry)
	assert.Equal(t, "allow", entry["kind"])
}

func TestHookHandlePreCompact_DBOpenFailure_SilentAllow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Make the DB's parent directory creation fail: point PROJECT_KB_PATH at
	// a path whose parent is a plain file, so store.InitDB's MkdirAll errors
	// and withDB returns a non-nil error.
	blocker := filepath.Join(home, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	t.Setenv("PROJECT_KB_PATH", filepath.Join(blocker, "sub", "kb.db"))
	t.Setenv("QM_DB_PATH", "")

	p := hooks.PreCompactPayload{Common: hooks.Common{Cwd: "/tmp", SessionID: "abc"}}
	resp := hookHandlePreCompact(t.Context(), strings.NewReader(""), p)

	assert.Equal(t, hooks.Response{}, resp, "a withDB failure must silently allow, never block")
	entry := lastHookLogEntry(t, home)
	require.NotNil(t, entry)
	assert.Equal(t, "error", entry["kind"])
}

// --- extractAllKbBlocks / hasDecisionLanguage -----------------------------

func TestExtractAllKbBlocks_CountsBlocksAndDecisionTurnsSeparately(t *testing.T) {
	transcript := writeTranscript(t, []string{
		assistantLine(kbBlockMessage("A")),
		assistantLine("we decided to do this"),
		assistantLine("nothing interesting here"),
	})
	blockCount, decisionTurns := extractAllKbBlocks(transcript)
	assert.Equal(t, 1, blockCount)
	assert.Equal(t, 1, decisionTurns)
}

func TestExtractAllKbBlocks_SkipsSidechainAndNonAssistant(t *testing.T) {
	sidechain := `{"type":"assistant","isSidechain":true,"message":{"content":[{"type":"text","text":"decision language here"}]}}`
	transcript := writeTranscript(t, []string{
		sidechain,
		userLine("a user message mentioning decision too"),
	})
	blockCount, decisionTurns := extractAllKbBlocks(transcript)
	assert.Equal(t, 0, blockCount)
	assert.Equal(t, 0, decisionTurns)
}

func TestExtractAllKbBlocks_MissingFile(t *testing.T) {
	blockCount, decisionTurns := extractAllKbBlocks(filepath.Join(t.TempDir(), "nope.jsonl"))
	assert.Equal(t, 0, blockCount)
	assert.Equal(t, 0, decisionTurns)
}

func TestHasDecisionLanguage(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{"decision word", "this was a hard decision", true},
		{"decided", "we decided", true},
		{"chose", "she chose wisely", true},
		{"chosen", "the chosen path", true},
		{"choose", "let's choose carefully", true},
		{"going with", "going with option A", true},
		{"reject", "we reject this approach", true},
		{"pick", "pick the fastest one", true},
		{"select", "select the best candidate", true},
		{"opting", "opting for simplicity", true},
		{"none", "just an ordinary sentence", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, hasDecisionLanguage(tc.text))
		})
	}
}

// --- queryPersistedCount ---------------------------------------------------

func TestQueryPersistedCount_DBError(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Close())

	count, err := queryPersistedCount(db, "acme-corp", "sess-x")
	require.Error(t, err)
	assert.Equal(t, 0, count)
}

func TestRunHookPreCompact_KbBlocks_QueryError_AllowsQueryError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	db := newTestDB(t)
	require.NoError(t, db.Close())
	cwd := gitProjectDir(t)

	transcript := writeTranscript(t, []string{assistantLine(kbBlockMessage("A"))})
	p := hooks.PreCompactPayload{Common: hooks.Common{TranscriptPath: transcript, Cwd: cwd, SessionID: "sess-err"}}
	resp := runHookPreCompact(p, db)

	assert.Equal(t, hooks.Response{}, resp)
	entry := lastHookLogEntry(t, home)
	require.NotNil(t, entry)
	assert.Equal(t, "allow", entry["kind"])
	assert.Equal(t, "query_error", entry["reason"])
}

func TestExtractAllKbBlocks_SkipsAssistantWithNilMessage(t *testing.T) {
	transcript := writeTranscript(t, []string{
		`{"type":"assistant"}`,
		assistantLine(kbBlockMessage("A")),
	})
	blockCount, decisionTurns := extractAllKbBlocks(transcript)
	assert.Equal(t, 1, blockCount)
	assert.Equal(t, 0, decisionTurns)
}

func TestQueryPersistedCount_NoMatches(t *testing.T) {
	db := newTestDB(t)
	count, err := queryPersistedCount(db, "nonexistent-project", "sess-x")
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestQueryPersistedCount_MatchesBySessionAndProject(t *testing.T) {
	db := newTestDB(t)
	_, err := store.UpsertDocument(db, store.Document{
		Type: "fact", Project: "acme-corp", Title: "Doc A", Content: "x", SessionID: "sess-y",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type: "fact", Project: "acme-corp", Title: "Doc B", Content: "x", SessionID: "sess-z",
	})
	require.NoError(t, err)

	count, err := queryPersistedCount(db, "acme-corp", "sess-y")
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}
