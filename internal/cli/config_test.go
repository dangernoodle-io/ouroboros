package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"

	"dangernoodle.io/ouroboros/internal/backlog"
)

func TestRunConfigGet(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, backlog.SetConfig(db, "test_key", "test_value"))

	var buf bytes.Buffer
	err := runConfigGet(&buf, db, "test_key")
	require.NoError(t, err)
	assert.Equal(t, "test_value\n", buf.String())
}

func TestRunConfigGetNotFound(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runConfigGet(&buf, db, "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRunConfigSet(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runConfigSet(&buf, db, "new_key", "new_value")
	require.NoError(t, err)
	assert.Equal(t, "set new_key=new_value\n", buf.String())

	// Verify it was stored
	value, err := backlog.GetConfig(db, "new_key")
	require.NoError(t, err)
	assert.Equal(t, "new_value", value)
}

func TestRunConfigSetOverwrite(t *testing.T) {
	db := newTestDB(t)

	// Set initial value
	require.NoError(t, backlog.SetConfig(db, "key", "value1"))

	// Overwrite
	var buf bytes.Buffer
	err := runConfigSet(&buf, db, "key", "value2")
	require.NoError(t, err)
	assert.Equal(t, "set key=value2\n", buf.String())

	// Verify it was updated
	value, err := backlog.GetConfig(db, "key")
	require.NoError(t, err)
	assert.Equal(t, "value2", value)
}

func TestRunConfigList(t *testing.T) {
	db := newTestDB(t)

	// Set multiple config values
	require.NoError(t, backlog.SetConfig(db, "key1", "value1"))
	require.NoError(t, backlog.SetConfig(db, "key2", "value2"))
	require.NoError(t, backlog.SetConfig(db, "key3", "value3"))

	var buf bytes.Buffer
	err := runConfigList(&buf, db)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "key1=value1")
	assert.Contains(t, output, "key2=value2")
	assert.Contains(t, output, "key3=value3")
}

func TestRunConfigListEmpty(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runConfigList(&buf, db)
	require.NoError(t, err)
	assert.Equal(t, "", buf.String())
}

func TestRunConfigSetSpecialCharacters(t *testing.T) {
	db := newTestDB(t)

	testCases := []struct {
		name  string
		key   string
		value string
	}{
		{
			name:  "spaces",
			key:   "key_with_spaces",
			value: "value with spaces",
		},
		{
			name:  "special chars",
			key:   "key-special",
			value: "val@#$%^&*()_+-=[]{}|;:,.<>?",
		},
		{
			name:  "unicode",
			key:   "key_unicode",
			value: "🔑=value",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := runConfigSet(&buf, db, tc.key, tc.value)
			require.NoError(t, err)

			// Verify retrieval
			retrieved, err := backlog.GetConfig(db, tc.key)
			require.NoError(t, err)
			assert.Equal(t, tc.value, retrieved)
		})
	}
}

func TestRunConfigSetError(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Close())

	var buf bytes.Buffer
	err := runConfigSet(&buf, db, "k", "v")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config set")
}

func TestRunConfigListError(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Close())

	var buf bytes.Buffer
	err := runConfigList(&buf, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config list")
}

func TestRunConfigGetError(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Close())

	var buf bytes.Buffer
	err := runConfigGet(&buf, db, "k")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config get")
}

func TestRunConfigListMultipleValues(t *testing.T) {
	db := newTestDB(t)

	// Set values in a specific order to test output consistency
	keys := []string{"alpha", "beta", "gamma"}
	for i, key := range keys {
		require.NoError(t, backlog.SetConfig(db, key, "value"+string(rune('1'+i))))
	}

	var buf bytes.Buffer
	err := runConfigList(&buf, db)
	require.NoError(t, err)

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	assert.Len(t, lines, 3)

	// Verify all expected keys are present
	for _, key := range keys {
		assert.True(t, strings.Contains(output, key), "output should contain key %s", key)
	}
}

// TestVersionFlag_DevBuild proves rootCmd's cobra-native --version (wired
// via rootCmd.Version in root.go's init, not a hand-rolled RunE) prints the
// dev-build placeholder idiomatically ("<name> version <value>", cobra's
// defaultVersionTemplate) when built without ldflags.
func TestVersionFlag_DevBuild(t *testing.T) {
	origVersion := rootCmd.Version
	defer func() { rootCmd.Version = origVersion }()
	rootCmd.Version = "(development build)"

	output := execRootCaptured(t, "--version")

	assert.Contains(t, output, "version (development build)")
}

// TestVersionFlag_WithVersion proves the same idiomatic --version path
// prints a real ldflags-injected version string.
func TestVersionFlag_WithVersion(t *testing.T) {
	origVersion := rootCmd.Version
	defer func() { rootCmd.Version = origVersion }()
	rootCmd.Version = "v1.2.3-test"

	output := execRootCaptured(t, "--version")

	assert.Contains(t, output, "version v1.2.3-test")
}

// TestBareInvocation_ShowsHelp proves bare `ouroboros` (no args, no
// subcommand) is a pure help dispatcher: exits cleanly (no error) and
// prints usage, since rootCmd carries no Run/RunE (see root.go).
func TestBareInvocation_ShowsHelp(t *testing.T) {
	output := execRootCaptured(t)

	assert.Contains(t, output, "Usage:")
	assert.Contains(t, output, "ouroboros")
}

// TestServerSubcommand_Registered proves "server" (app.NewServerCommand,
// shesha's cli.ServerCmd) is mounted on rootCmd -- the only way the MCP
// server runs now that bare invocation no longer defaults to it.
func TestServerSubcommand_Registered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"server"})
	require.NoError(t, err)
	assert.Equal(t, "server", cmd.Name())
}

// execRootCaptured runs rootCmd with args, capturing both stdout and
// stderr into a single buffer via cobra's own SetOut/SetErr (rather than
// swapping os.Stdout), and restores rootCmd's output/args/flag state
// afterward so tests stay isolated from each other.
func execRootCaptured(t *testing.T, args ...string) string {
	t.Helper()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs(args)
	defer func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
		_ = rootCmd.Flags().Set("version", "false")
	}()

	require.NoError(t, rootCmd.Execute())
	return buf.String()
}
