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
	cmds := provider.Commands()
	require.Len(t, cmds, 1, "claude namespace is a single top-level command")
	assert.Equal(t, "claude", cmds[0].Use)

	stopCmd, _, err := cmds[0].Find([]string{"hooks", "stop"})
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
