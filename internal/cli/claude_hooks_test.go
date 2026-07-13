package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClaudeHooksStop_WireDecodesStdinAndRuns exercises the full mcpkit
// seam end to end: build the provider's command tree, locate `hooks stop`,
// feed it real stdin JSON, and confirm it decodes and runs to a silent
// exit-0 (no transcript_path present, so runHookStop short-circuits).
func TestClaudeHooksStop_WireDecodesStdinAndRuns(t *testing.T) {
	isolateHookLog(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PROJECT_KB_PATH", filepath.Join(home, "kb.db"))
	t.Setenv("QM_DB_PATH", "")

	provider := claudeProvider()
	mounts := provider.Mounts()
	require.Len(t, mounts, 1, "claude namespace is a single top-level command")
	assert.Equal(t, "claude", mounts[0].Cmd.Use)

	stopCmd, _, err := mounts[0].Cmd.Find([]string{"hooks", "stop"})
	require.NoError(t, err)

	var out bytes.Buffer
	stopCmd.SetOut(&out)
	stopCmd.SetIn(strings.NewReader(`{"cwd":"/tmp","session_id":"abc"}`))

	require.NoError(t, stopCmd.RunE(stopCmd, nil))
	assert.Empty(t, out.String())
}

// TestOuroborosHookCommand_Removed pins that the old top-level `hook stop`
// command tree no longer exists on rootCmd — the migration target.
func TestOuroborosHookCommand_Removed(t *testing.T) {
	_, _, err := rootCmd.Find([]string{"hook", "stop"})
	assert.Error(t, err)
}

// TestRootCmd_ClaudeHooksStopMounted confirms `ouroboros claude hooks stop`
// is reachable from the real rootCmd (the wiring in root.go), not just from
// a freshly-built provider.
func TestRootCmd_ClaudeHooksStopMounted(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"claude", "hooks", "stop"})
	require.NoError(t, err)
	assert.Equal(t, "stop", cmd.Use)
}

// TestClaudeHooksSubagentStop_WireDecodesStdinAndRuns mirrors
// TestClaudeHooksStop_WireDecodesStdinAndRuns for the SubagentStop event:
// build the provider's command tree, locate `hooks subagent-stop`, feed it
// real stdin JSON, and confirm it decodes and runs to a silent exit-0 (too
// short a message, so runHookSubagentStop short-circuits).
func TestClaudeHooksSubagentStop_WireDecodesStdinAndRuns(t *testing.T) {
	isolateHookLog(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PROJECT_KB_PATH", filepath.Join(home, "kb.db"))
	t.Setenv("QM_DB_PATH", "")

	provider := claudeProvider()
	mounts := provider.Mounts()
	require.Len(t, mounts, 1, "claude namespace is a single top-level command")

	subagentStopCmd, _, err := mounts[0].Cmd.Find([]string{"hooks", "subagent-stop"})
	require.NoError(t, err)

	var out bytes.Buffer
	subagentStopCmd.SetOut(&out)
	subagentStopCmd.SetIn(strings.NewReader(`{"cwd":"/tmp","session_id":"abc","last_assistant_message":"too short"}`))

	require.NoError(t, subagentStopCmd.RunE(subagentStopCmd, nil))
	assert.Empty(t, out.String())
}

// TestRootCmd_ClaudeHooksSubagentStopMounted confirms
// `ouroboros claude hooks subagent-stop` is reachable from the real rootCmd
// (root.go wiring), not just from a freshly-built provider.
func TestRootCmd_ClaudeHooksSubagentStopMounted(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"claude", "hooks", "subagent-stop"})
	require.NoError(t, err)
	assert.Equal(t, "subagent-stop", cmd.Use)
}

// TestClaudeHooksUserPromptSubmit_WireDecodesStdinAndRuns mirrors
// TestClaudeHooksStop_WireDecodesStdinAndRuns for the UserPromptSubmit
// event: build the provider's command tree, locate
// `hooks user-prompt-submit`, feed it real stdin JSON, and confirm it
// decodes and runs to a silent exit-0 (no cwd/no reachable project, so
// runHookUserPromptSubmit short-circuits).
func TestClaudeHooksUserPromptSubmit_WireDecodesStdinAndRuns(t *testing.T) {
	isolateHookLog(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PROJECT_KB_PATH", filepath.Join(home, "kb.db"))
	t.Setenv("QM_DB_PATH", "")

	provider := claudeProvider()
	mounts := provider.Mounts()
	require.Len(t, mounts, 1, "claude namespace is a single top-level command")

	upsCmd, _, err := mounts[0].Cmd.Find([]string{"hooks", "user-prompt-submit"})
	require.NoError(t, err)

	var out bytes.Buffer
	upsCmd.SetOut(&out)
	upsCmd.SetIn(strings.NewReader(`{"cwd":"/tmp","session_id":"abc","prompt":"please help me understand this codebase in detail"}`))

	require.NoError(t, upsCmd.RunE(upsCmd, nil))
	assert.Empty(t, out.String())
}

// TestRootCmd_ClaudeHooksUserPromptSubmitMounted confirms
// `ouroboros claude hooks user-prompt-submit` is reachable from the real
// rootCmd (root.go wiring), not just from a freshly-built provider.
func TestRootCmd_ClaudeHooksUserPromptSubmitMounted(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"claude", "hooks", "user-prompt-submit"})
	require.NoError(t, err)
	assert.Equal(t, "user-prompt-submit", cmd.Use)
}
