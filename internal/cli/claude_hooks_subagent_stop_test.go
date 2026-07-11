package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dangernoodle-io/mcpkit/host/claudecode/hooks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/store"
)

func TestRunHookSubagentStop_StopHookActive_StillPersists(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	message := longEnough("```kb\n{\"type\":\"decision\",\"category\":\"arch\",\"title\":\"Persist Even When StopHookActive\",\"content\":\"byte-faithful to js\"}\n```\n")
	p := hooks.SubagentStopPayload{
		Common:               hooks.Common{Cwd: cwd, SessionID: "sess9999"},
		AgentID:              "agent-stopactive01",
		AgentType:            "worker",
		LastAssistantMessage: message,
		StopHookActive:       true,
	}

	resp := runHookSubagentStop(p, db)
	assert.Equal(t, hooks.Response{}, resp, "StopHookActive field is ignored; kb block still persists")

	summaries, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 10)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "Persist Even When StopHookActive", summaries[0].Title)

	doc, err := store.GetDocument(db, summaries[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "hook:subagent_stop", doc.Metadata["source"])
	assert.Equal(t, "sess9999", doc.Metadata["session_id"])
	assert.Equal(t, "agent-stopactive01", doc.Metadata["agent_id"])
	assert.Equal(t, "worker", doc.Metadata["agent_type"])
}

func TestRunHookSubagentStop_SkippedAgentType_NoOutput(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	message := longEnough("```kb\n{\"type\":\"fact\",\"title\":\"Should Not Persist\",\"content\":\"x\"}\n```\n")
	p := hooks.SubagentStopPayload{
		Common:               hooks.Common{Cwd: cwd, SessionID: "abc"},
		AgentType:            "Explore",
		LastAssistantMessage: message,
	}
	assert.Equal(t, hooks.Response{}, runHookSubagentStop(p, db))

	summaries, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 10)
	require.NoError(t, err)
	assert.Empty(t, summaries)
}

func TestRunHookSubagentStop_SkippedAgentType_NamespacedTail(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	message := longEnough("```kb\n{\"type\":\"fact\",\"title\":\"Should Not Persist\",\"content\":\"x\"}\n```\n")
	p := hooks.SubagentStopPayload{
		Common:               hooks.Common{Cwd: cwd, SessionID: "abc"},
		AgentType:            "plugin:knowledge-explorer",
		LastAssistantMessage: message,
	}
	assert.Equal(t, hooks.Response{}, runHookSubagentStop(p, db))

	summaries, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 10)
	require.NoError(t, err)
	assert.Empty(t, summaries)
}

func TestRunHookSubagentStop_ShortMessage_NoOutput(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	p := hooks.SubagentStopPayload{
		Common:               hooks.Common{Cwd: "/tmp", SessionID: "abc"},
		LastAssistantMessage: "too short",
	}
	assert.Equal(t, hooks.Response{}, runHookSubagentStop(p, db))
}

func TestRunHookSubagentStop_KbBlock_Persisted(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	message := longEnough("```kb\n{\"type\":\"decision\",\"category\":\"arch\",\"title\":\"Use PostgreSQL\",\"content\":\"performance\"}\n```\n")
	p := hooks.SubagentStopPayload{
		Common:               hooks.Common{Cwd: cwd, SessionID: "sess1234"},
		AgentID:              "agent-abcdef123456",
		AgentType:            "worker",
		LastAssistantMessage: message,
	}

	resp := runHookSubagentStop(p, db)
	assert.Equal(t, hooks.Response{}, resp, "kb block persisted, silent allow")

	summaries, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 10)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "Use PostgreSQL", summaries[0].Title)

	doc, err := store.GetDocument(db, summaries[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "hook:subagent_stop", doc.Metadata["source"])
	assert.Equal(t, "sess1234", doc.Metadata["session_id"])
	assert.Equal(t, "agent-abcdef123456", doc.Metadata["agent_id"])
	assert.Equal(t, "worker", doc.Metadata["agent_type"])
}

func TestRunHookSubagentStop_KbBlockArray_AllPersisted(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	message := longEnough("```kb\n[{\"type\":\"fact\",\"title\":\"Fact A\",\"content\":\"a\"},{\"type\":\"fact\",\"title\":\"Fact B\",\"content\":\"b\"}]\n```\n")
	p := hooks.SubagentStopPayload{
		Common:               hooks.Common{Cwd: cwd, SessionID: "sess5678"},
		LastAssistantMessage: message,
	}

	assert.Equal(t, hooks.Response{}, runHookSubagentStop(p, db))

	summaries, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 10)
	require.NoError(t, err)
	assert.Len(t, summaries, 2)
}

func TestRunHookSubagentStop_PersistSkillSentinel_Skipped(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	message := longEnough("```kb\n{\"type\":\"fact\",\"title\":\"Already Done\",\"content\":\"x\",\"_persisted_by\":\"persist-skill\"}\n```\n")
	p := hooks.SubagentStopPayload{
		Common:               hooks.Common{Cwd: cwd, SessionID: "sess0001"},
		LastAssistantMessage: message,
	}

	assert.Equal(t, hooks.Response{}, runHookSubagentStop(p, db))

	summaries, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 10)
	require.NoError(t, err)
	assert.Empty(t, summaries)
}

func TestRunHookSubagentStop_MalformedKbJSON_HandledNoWrite(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	message := longEnough("```kb\nnot valid json at all here\n```\n")
	p := hooks.SubagentStopPayload{
		Common:               hooks.Common{Cwd: cwd, SessionID: "sess0002"},
		LastAssistantMessage: message,
	}

	assert.Equal(t, hooks.Response{}, runHookSubagentStop(p, db))

	summaries, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 10)
	require.NoError(t, err)
	assert.Empty(t, summaries)
}

func TestRunHookSubagentStop_KbBlock_NoProject_NotPersisted(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	// Not a git repo — projectFromPath resolves to "".
	cwd := t.TempDir()
	message := longEnough("```kb\n{\"type\":\"fact\",\"title\":\"Orphan\",\"content\":\"x\"}\n```\n")
	p := hooks.SubagentStopPayload{
		Common:               hooks.Common{Cwd: cwd, SessionID: "sess0003"},
		LastAssistantMessage: message,
	}

	assert.Equal(t, hooks.Response{}, runHookSubagentStop(p, db))

	summaries, err := store.QueryDocuments(db, nil, nil, nil, "", nil, 10)
	require.NoError(t, err)
	assert.Empty(t, summaries)
}

func TestRunHookSubagentStop_NoKbBlock_DecisionLanguage_NeverNudges(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	// Decision-language would trigger a Stop-hook tier-1 nudge; the
	// SubagentStop hook must never nudge (OU-222/OU-254).
	message := longEnough("We decided to use PostgreSQL for the new service.")
	p := hooks.SubagentStopPayload{
		Common:               hooks.Common{Cwd: cwd, SessionID: "sess0004"},
		LastAssistantMessage: message,
	}

	assert.Equal(t, hooks.Response{}, runHookSubagentStop(p, db), "no nudge, ever, for subagent-stop")
}

func TestRunHookSubagentStop_LongAgentID_Truncated(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := gitProjectDir(t)
	message := longEnough("```kb\n{\"type\":\"fact\",\"title\":\"Trunc\",\"content\":\"x\"}\n```\n")
	p := hooks.SubagentStopPayload{
		Common:               hooks.Common{Cwd: cwd, SessionID: "sess0005"},
		AgentID:              "abcdefghijklmnop",
		LastAssistantMessage: message,
	}

	assert.Equal(t, hooks.Response{}, runHookSubagentStop(p, db))

	summaries, err := store.QueryDocuments(db, nil, []string{"acme-corp"}, nil, "", nil, 10)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	doc, err := store.GetDocument(db, summaries[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "abcdefghijklmnop", doc.Metadata["agent_id"], "full agent_id is stored in metadata (only the log excerpt id is truncated)")
}

func TestHookHandleSubagentStop_DBOpenFailure_SilentAllow(t *testing.T) {
	isolateHookLog(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	blocker := filepath.Join(home, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	t.Setenv("PROJECT_KB_PATH", filepath.Join(blocker, "sub", "kb.db"))
	t.Setenv("QM_DB_PATH", "")

	p := hooks.SubagentStopPayload{Common: hooks.Common{Cwd: "/tmp", SessionID: "abc"}}
	resp := hookHandleSubagentStop(t.Context(), strings.NewReader(""), p)
	assert.Equal(t, hooks.Response{}, resp, "a withDB failure must silently allow, never block")
}

// --- isSkippedAgentType ------------------------------------------------------

func TestIsSkippedAgentType(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", false},
		{"exact match", "Explore", true},
		{"exact match knowledge-explorer", "knowledge-explorer", true},
		{"exact match backlog-manager", "backlog-manager", true},
		{"not skipped", "worker", false},
		{"namespaced tail matches", "plugin:Explore", true},
		{"namespaced tail no match", "plugin:worker", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isSkippedAgentType(tc.in))
		})
	}
}

// --- excerptMessage -----------------------------------------------------------

func TestExcerptMessage_ShortUnchanged(t *testing.T) {
	assert.Equal(t, "hello", excerptMessage("hello", 120))
}

func TestExcerptMessage_TruncatesAndStripsNewlines(t *testing.T) {
	msg := strings.Repeat("a", 130) + "\nmore"
	out := excerptMessage(msg, 120)
	assert.Len(t, []rune(out), 120)
	assert.NotContains(t, out, "\n")
}
