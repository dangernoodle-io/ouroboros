package cli

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/dangernoodle-io/shesha/host/claudecode/hooks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/store"
)

// --- hub-suppression / prompt-mention resolution order ---------------------

// TestRunHookSubagentStart_MarketplaceHubCwd_NoMention_NoInjection is the
// hub-suppression regression test (OU-283/OU-263-consistency applied to
// SubagentStart): cwd resolves to the marketplace hub repo, and the
// subagent's prompt names no project. This MUST return a silent zero
// Response instead of injecting the hub's own KB. REQUIRED to fail if the
// `!isMarketplaceRepo` guard in runHookSubagentStart is removed (see the
// mutation-verify note at the bottom of this file).
func TestRunHookSubagentStart_MarketplaceHubCwd_NoMention_NoInjection(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	_, marketplaceDir, _ := upcWorkspace(t)
	subagentSeedFillerDocs(t, db)
	seedKbDoc(t, db, "dangernoodle-marketplace", "Marketplace secret decision")

	p := hooks.SubagentStartPayload{
		Common:    hooks.Common{Cwd: marketplaceDir, SessionID: "sub-sess1"},
		AgentType: "worker",
	}
	assert.Equal(t, hooks.Response{}, runHookSubagentStart(p, db))
}

func TestRunHookSubagentStart_PromptMention_WinsOverMarketplaceHubCwd(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	_, marketplaceDir, _ := upcWorkspace(t)
	subagentSeedFillerDocs(t, db)
	seedKbDoc(t, db, "ouroboros", "Use PostgreSQL for storage")
	_, err := backlog.CreateProject(db, "ouroboros", "OU")
	require.NoError(t, err)

	p := hooks.SubagentStartPayload{
		Common:    hooks.Common{Cwd: marketplaceDir, SessionID: "sub-sess2"},
		AgentType: "worker",
		Prompt:    "port the ouroboros hook and check how we use postgresql for its storage layer",
	}
	resp := runHookSubagentStart(p, db)
	assert.Contains(t, resp.AdditionalContext, "Use PostgreSQL")
}

func TestRunHookSubagentStart_WorkRepoCwd_NoMention_InjectsCwdProject(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	_, _, ouroborosDir := upcWorkspace(t)
	seedKbDoc(t, db, "ouroboros", "Use gRPC for internal APIs")

	p := hooks.SubagentStartPayload{
		Common:    hooks.Common{Cwd: ouroborosDir, SessionID: "sub-sess3"},
		AgentType: "worker",
	}
	resp := runHookSubagentStart(p, db)
	assert.Contains(t, resp.AdditionalContext, "Use gRPC")
}

// --- no project resolvable ---------------------------------------------

func TestRunHookSubagentStart_NoWorkspaceNoGit_NoInjection(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	cwd := t.TempDir() // no .claude ancestor, no .git ancestor
	p := hooks.SubagentStartPayload{
		Common:    hooks.Common{Cwd: cwd, SessionID: "sub-sess4"},
		AgentType: "worker",
	}
	assert.Equal(t, hooks.Response{}, runHookSubagentStart(p, db))
}

// --- skip-list -----------------------------------------------------------

func TestRunHookSubagentStart_SkippedAgentType_NoInjection(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	_, _, ouroborosDir := upcWorkspace(t)
	seedKbDoc(t, db, "ouroboros", "Use gRPC for internal APIs")

	p := hooks.SubagentStartPayload{
		Common:    hooks.Common{Cwd: ouroborosDir, SessionID: "sub-sess5"},
		AgentType: "Explore",
	}
	assert.Equal(t, hooks.Response{}, runHookSubagentStart(p, db))
}

func TestRunHookSubagentStart_SkippedAgentType_NamespaceQualified_NoInjection(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	_, _, ouroborosDir := upcWorkspace(t)
	seedKbDoc(t, db, "ouroboros", "Use gRPC for internal APIs")

	p := hooks.SubagentStartPayload{
		Common:    hooks.Common{Cwd: ouroborosDir, SessionID: "sub-sess5b"},
		AgentType: "plugin:knowledge-explorer",
	}
	assert.Equal(t, hooks.Response{}, runHookSubagentStart(p, db))
}

// --- nothing to inject ------------------------------------------------

func TestRunHookSubagentStart_NoKbNoBacklog_NoInjection(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	_, _, ouroborosDir := upcWorkspace(t)
	// No KB docs, no backlog items seeded at all.
	p := hooks.SubagentStartPayload{
		Common:    hooks.Common{Cwd: ouroborosDir, SessionID: "sub-sess6"},
		AgentType: "worker",
	}
	assert.Equal(t, hooks.Response{}, runHookSubagentStart(p, db))
}

// --- KB + backlog injection content -------------------------------------

func TestRunHookSubagentStart_NoPrompt_InjectsRecentKbAndBacklog(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	_, _, ouroborosDir := upcWorkspace(t)
	seedKbDoc(t, db, "ouroboros", "Some ouroboros decision")

	proj, err := backlog.CreateProject(db, "ouroboros", "OU")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Port the next hook", "", "", "", "")
	require.NoError(t, err)

	p := hooks.SubagentStartPayload{
		Common:    hooks.Common{Cwd: ouroborosDir, SessionID: "sub-sess7"},
		AgentType: "worker",
	}
	resp := runHookSubagentStart(p, db)
	assert.Contains(t, resp.AdditionalContext, "KB context (recent)")
	assert.Contains(t, resp.AdditionalContext, "Some ouroboros decision")
	assert.Contains(t, resp.AdditionalContext, "Open items (top 1)")
	assert.Contains(t, resp.AdditionalContext, "Port the next hook")
}

func TestRunHookSubagentStart_WithPrompt_SearchMatch_LabelsRelevant(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	_, _, ouroborosDir := upcWorkspace(t)
	subagentSeedFillerDocs(t, db)
	seedKbDoc(t, db, "ouroboros", "Use gRPC for internal APIs")

	p := hooks.SubagentStartPayload{
		Common:    hooks.Common{Cwd: ouroborosDir, SessionID: "sub-sess8"},
		AgentType: "worker",
		Prompt:    "please implement the gRPC internal APIs change",
	}
	resp := runHookSubagentStart(p, db)
	assert.Contains(t, resp.AdditionalContext, "KB context (relevant)")
	assert.Contains(t, resp.AdditionalContext, "Use gRPC for internal APIs")
}

func TestRunHookSubagentStart_WithPrompt_NoSearchMatch_FallsBackToRecent_StillLabelsRelevant(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	_, _, ouroborosDir := upcWorkspace(t)
	seedKbDoc(t, db, "ouroboros", "Totally unrelated filing cabinet reorganization notes")

	p := hooks.SubagentStartPayload{
		Common:    hooks.Common{Cwd: ouroborosDir, SessionID: "sub-sess9"},
		AgentType: "worker",
		Prompt:    "xyzzyqux plugh frobnicate nonexistent vocabulary zzzznomatch",
	}
	resp := runHookSubagentStart(p, db)
	// Faithful port: label depends only on prompt truthiness, not on
	// whether the search path actually found a match.
	assert.Contains(t, resp.AdditionalContext, "KB context (relevant)")
	assert.Contains(t, resp.AdditionalContext, "Totally unrelated filing cabinet reorganization notes")
}

// --- no cooldown -----------------------------------------------------------

// TestRunHookSubagentStart_NoCooldown_ConsecutiveSpawnsBothInject is the
// rule-compliance regression test (rules/plugin.md: "Do NOT cooldown
// SubagentStart — every subagent is a fresh context with no memory"): two
// spawns in a row, same session+project, must BOTH inject — no suppression
// on the second. subagent-start.js's 60s session+project cooldown was a bug
// and is deliberately not carried forward (see runHookSubagentStart's doc
// comment).
func TestRunHookSubagentStart_NoCooldown_ConsecutiveSpawnsBothInject(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	_, _, ouroborosDir := upcWorkspace(t)
	seedKbDoc(t, db, "ouroboros", "Use gRPC for internal APIs")

	p := hooks.SubagentStartPayload{
		Common:    hooks.Common{Cwd: ouroborosDir, SessionID: "sub-sess10"},
		AgentType: "worker",
	}
	first := runHookSubagentStart(p, db)
	require.NotEqual(t, hooks.Response{}, first, "first spawn should inject")

	second := runHookSubagentStart(p, db)
	assert.NotEqual(t, hooks.Response{}, second, "a second consecutive spawn (same session+project) must also inject — no cooldown")
	assert.Equal(t, first, second, "back-to-back spawns with unchanged state must produce identical injected content")
}

// --- withDB failure --------------------------------------------------------

func TestHookHandleSubagentStart_DBOpenFailure_SilentAllow(t *testing.T) {
	isolateHookLog(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	blocker := filepath.Join(home, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	t.Setenv("PROJECT_KB_PATH", filepath.Join(blocker, "sub", "kb.db"))
	t.Setenv("QM_DB_PATH", "")

	p := hooks.SubagentStartPayload{Common: hooks.Common{Cwd: "/tmp", SessionID: "abc"}, AgentType: "worker"}
	resp := hookHandleSubagentStart(t.Context(), nil, p)
	assert.Equal(t, hooks.Response{}, resp, "a withDB failure must silently allow, never block")
}

// --- subagentStartBacklogItems ------------------------------------------

func TestSubagentStartBacklogItems_UnknownProject_ReturnsNil(t *testing.T) {
	db := newTestDB(t)
	assert.Nil(t, subagentStartBacklogItems(db, "no-such-project"))
}

func TestSubagentStartBacklogItems_NoOpenItems_ReturnsNil(t *testing.T) {
	db := newTestDB(t)
	_, err := backlog.CreateProject(db, "empty-project", "EP")
	require.NoError(t, err)
	assert.Nil(t, subagentStartBacklogItems(db, "empty-project"))
}

// subagentSeedFillerDocs seeds unrelated filler documents in another project
// so BM25's idf statistics (global across the FTS index) are meaningful,
// rather than collapsing to ~0 in a single-document corpus (see
// upcSeedFillerDocs's doc comment for the full rationale — the same
// principle applies here).
func subagentSeedFillerDocs(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := store.UpsertDocument(db, store.Document{
		Type: "note", Project: "filler", Title: "Rotate compost bin weekly", Content: "gardening chore schedule reminder",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type: "note", Project: "filler", Title: "Order more printer toner", Content: "office supplies inventory restock",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type: "note", Project: "filler", Title: "Plan team lunch venue", Content: "restaurant booking logistics",
	})
	require.NoError(t, err)
}

// --- OU-309: mention resolution uses ouroboros's registered projects ------

// TestRunHookSubagentStart_MentionOfUnregisteredSubdir_NoSpuriousMatch is the
// OU-309 regression for the spurious-subdirectory-match bug: with a
// NON-EMPTY, realistic candidate list (two REGISTERED projects, neither
// named "internal"), the subagent's prompt mentions "internal/cli" (a path
// fragment, never a registered project) and no registered project name.
// Mention resolution must NOT resolve to a project named "internal" —
// proving the unregistered name is correctly rejected from a populated
// candidate list, not merely absent because no candidates existed.
// Resolution falls through to the cwd tier, injecting the cwd repo's
// ("ouroboros") own KB.
func TestRunHookSubagentStart_MentionOfUnregisteredSubdir_NoSpuriousMatch(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	_, _, ouroborosDir := upcWorkspace(t)
	_, err := backlog.CreateProject(db, "ouroboros", "OU")
	require.NoError(t, err)
	_, err = backlog.CreateProject(db, "breadboard", "BB")
	require.NoError(t, err)
	subagentSeedFillerDocs(t, db)
	seedKbDoc(t, db, "ouroboros", "Use gRPC for internal APIs")

	p := hooks.SubagentStartPayload{
		Common:    hooks.Common{Cwd: ouroborosDir, SessionID: "sub-ou309-subdir"},
		AgentType: "worker",
		Prompt:    "please look at internal/cli and explain how we use gRPC for internal APIs here",
	}
	resp := runHookSubagentStart(p, db)
	assert.Contains(t, resp.AdditionalContext, "Use gRPC for internal APIs", "falls through to the cwd tier despite a non-empty registered-project candidate list")
	assert.NotContains(t, resp.AdditionalContext, "[ouroboros] internal", "the unregistered \"internal\" subdir-shaped mention must never resolve to a project label")
}

// TestRunHookSubagentStart_LongerRegisteredProjectNameWins is the OU-309
// longest-match regression: two registered projects both appear in the
// prompt ("ouroboros" is a substring-word of "ouroboros-plugin"); the
// longer, more-specific registered name must win.
func TestRunHookSubagentStart_LongerRegisteredProjectNameWins(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	_, marketplaceDir, _ := upcWorkspace(t)
	_, err := backlog.CreateProject(db, "ouroboros", "OU")
	require.NoError(t, err)
	_, err = backlog.CreateProject(db, "ouroboros-plugin", "OP")
	require.NoError(t, err)
	subagentSeedFillerDocs(t, db)
	seedKbDoc(t, db, "ouroboros", "Core KB decision")
	seedKbDoc(t, db, "ouroboros-plugin", "Plugin hook decision")

	p := hooks.SubagentStartPayload{
		Common:    hooks.Common{Cwd: marketplaceDir, SessionID: "sub-ou309-longest"},
		AgentType: "worker",
		Prompt:    "let's discuss the ouroboros-plugin hook decision in detail",
	}
	resp := runHookSubagentStart(p, db)
	assert.Contains(t, resp.AdditionalContext, "Plugin hook decision")
}

// --- mutation-verify (documented, not part of the permanent suite) --------
//
// Verified by hand during implementation: temporarily changing
//
//	if gitRoot != "" && !isMarketplaceRepo(gitRoot) {
//
// to
//
//	if gitRoot != "" {
//
// in runHookSubagentStart (dropping the marketplace-hub guard) makes
// TestRunHookSubagentStart_MarketplaceHubCwd_NoMention_NoInjection FAIL (the
// marketplace hub's KB entry gets injected). The guard was then reverted.
