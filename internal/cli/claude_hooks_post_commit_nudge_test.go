package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dangernoodle-io/mcpkit/host/claudecode/hooks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func bashToolInput(t *testing.T, command string) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(map[string]string{"command": command})
	require.NoError(t, err)
	return data
}

// --- git commit: nudge emitted to stderr ------------------------------------

func TestRunPostCommitNudge_GitCommit_EmitsNudge(t *testing.T) {
	isolateHookLog(t)
	t.Setenv("HOME", t.TempDir())

	p := hooks.PostToolUsePayload{
		Common:    hooks.Common{SessionID: "sess1"},
		ToolName:  "Bash",
		ToolInput: bashToolInput(t, "git commit -m 'wip'"),
	}

	out := captureStderr(t, func() { runPostCommitNudge(p) })
	assert.Equal(t, "[ouroboros] /persist to save decisions\n", out)
}

func TestRunPostCommitNudge_GitCommit_CaseInsensitive(t *testing.T) {
	isolateHookLog(t)
	t.Setenv("HOME", t.TempDir())

	p := hooks.PostToolUsePayload{
		Common:    hooks.Common{SessionID: "sess1c"},
		ToolName:  "Bash",
		ToolInput: bashToolInput(t, "GIT COMMIT -m 'wip'"),
	}

	out := captureStderr(t, func() { runPostCommitNudge(p) })
	assert.Contains(t, out, "/persist to save decisions")
}

// --- non-commit Bash command: no-op ------------------------------------------

func TestRunPostCommitNudge_NonCommitCommand_Silent(t *testing.T) {
	isolateHookLog(t)
	t.Setenv("HOME", t.TempDir())

	p := hooks.PostToolUsePayload{
		Common:    hooks.Common{SessionID: "sess2"},
		ToolName:  "Bash",
		ToolInput: bashToolInput(t, "ls -la"),
	}

	out := captureStderr(t, func() { runPostCommitNudge(p) })
	assert.Empty(t, out)
}

// --- per-project cooldown -----------------------------------------------------

func TestRunPostCommitNudge_Cooldown_SuppressesRepeat(t *testing.T) {
	isolateHookLog(t)
	t.Setenv("HOME", t.TempDir())

	p := hooks.PostToolUsePayload{
		Common:    hooks.Common{SessionID: "sess3"},
		ToolName:  "Bash",
		ToolInput: bashToolInput(t, "git commit -m first"),
	}

	first := captureStderr(t, func() { runPostCommitNudge(p) })
	assert.Contains(t, first, "/persist to save decisions", "first commit within the window should fire")

	second := captureStderr(t, func() { runPostCommitNudge(p) })
	assert.Empty(t, second, "a repeat commit within the 5-minute cooldown must be suppressed")
}

func TestRunPostCommitNudge_Cooldown_KeyedByProject_DifferentProjectNotSuppressed(t *testing.T) {
	isolateHookLog(t)
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()

	repo1 := filepath.Join(root, "repo-one")
	require.NoError(t, os.MkdirAll(filepath.Join(repo1, ".git"), 0o755))
	repo2 := filepath.Join(root, "repo-two")
	require.NoError(t, os.MkdirAll(filepath.Join(repo2, ".git"), 0o755))

	p1 := hooks.PostToolUsePayload{
		Common:    hooks.Common{SessionID: "sess4a", Cwd: repo1},
		ToolName:  "Bash",
		ToolInput: bashToolInput(t, "git commit -m x"),
	}
	_ = captureStderr(t, func() { runPostCommitNudge(p1) })

	p2 := hooks.PostToolUsePayload{
		Common:    hooks.Common{SessionID: "sess4b", Cwd: repo2},
		ToolName:  "Bash",
		ToolInput: bashToolInput(t, "git commit -m y"),
	}
	out2 := captureStderr(t, func() { runPostCommitNudge(p2) })
	assert.Contains(t, out2, "/persist to save decisions", "a different project's cooldown key must be independent")
}

// --- malformed / empty tool_input: silent, no panic --------------------------

func TestRunPostCommitNudge_MalformedToolInput_Silent(t *testing.T) {
	isolateHookLog(t)
	t.Setenv("HOME", t.TempDir())

	p := hooks.PostToolUsePayload{
		Common:    hooks.Common{SessionID: "sess5"},
		ToolName:  "Bash",
		ToolInput: json.RawMessage(`not json`),
	}

	out := captureStderr(t, func() { runPostCommitNudge(p) })
	assert.Empty(t, out)
}

func TestRunPostCommitNudge_EmptyToolInput_Silent(t *testing.T) {
	isolateHookLog(t)
	t.Setenv("HOME", t.TempDir())

	p := hooks.PostToolUsePayload{
		Common:    hooks.Common{SessionID: "sess6"},
		ToolName:  "Bash",
		ToolInput: json.RawMessage(`{}`),
	}

	out := captureStderr(t, func() { runPostCommitNudge(p) })
	assert.Empty(t, out)
}

// --- no resolvable project: nudge still fires (bucketed under "unknown") ----

func TestRunPostCommitNudge_NoResolvableProject_StillNudges(t *testing.T) {
	isolateHookLog(t)
	t.Setenv("HOME", t.TempDir())

	p := hooks.PostToolUsePayload{
		Common:    hooks.Common{SessionID: "sess7", Cwd: t.TempDir()}, // no .git ancestor
		ToolName:  "Bash",
		ToolInput: bashToolInput(t, "git commit -m x"),
	}

	out := captureStderr(t, func() { runPostCommitNudge(p) })
	assert.Contains(t, out, "/persist to save decisions")
}

func TestPostCommitNudgeCooldownFile_EmptyProject_UsesUnknownBucket(t *testing.T) {
	got := postCommitNudgeCooldownFile("")
	assert.Contains(t, got, filepath.Join("post-commit-nudge", "unknown"))
}

func TestPostCommitNudgeCooldownFile_SanitizesProjectName(t *testing.T) {
	got := postCommitNudgeCooldownFile("weird/name with spaces")
	assert.NotContains(t, got[len(got)-len("weird/name with spaces"):], "/")
	assert.Contains(t, got, "weird-name-with-spaces")
}
