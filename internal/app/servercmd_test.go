package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewServerCommand exercises NewServerCommand's full happy-path
// construction (buildServerV2 composition + cli.ServerCmd wiring) without
// ever calling RunE -- pure command-tree construction, no DB, no stdio,
// matching how internal/cli/root.go actually calls it at package-init
// time (building the cobra command tree must never have I/O side effects).
func TestNewServerCommand(t *testing.T) {
	cmd := NewServerCommand("v-test")
	require.NotNil(t, cmd)
	assert.Equal(t, "server", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
}

// TestServerOnStart_ConfigLoadError proves serverOnStart surfaces a real
// config.Load failure: OUROBOROS_CONFIG_DIR (xdgpath's app-specific
// override that internal/config.Load's mcpkitconfig.WithXDGFile resolves
// through) points at a directory containing a malformed bootstrap.json, so
// config.Load's loadFile -> json.Unmarshal genuinely fails -- a real
// misconfigured-bootstrap-file scenario (not a contrived fault hook). st.db
// must stay nil.
func TestServerOnStart_ConfigLoadError(t *testing.T) {
	configDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "bootstrap.json"), []byte("{not valid json"), 0o644))
	t.Setenv("OUROBOROS_CONFIG_DIR", configDir)

	st := &serverState{}
	err := serverOnStart(st)(context.Background())
	require.Error(t, err)
	assert.Nil(t, st.db)
}

// TestServerOnStart_InitDBError proves serverOnStart surfaces a real
// store.InitDB failure: PROJECT_KB_PATH points beneath a path component
// that is an ordinary FILE, not a directory, so InitDB's
// os.MkdirAll(parentDir, ...) genuinely fails -- a real misconfigured-path
// scenario (not a contrived fault hook). OUROBOROS_CONFIG_DIR points at an
// empty temp dir (no bootstrap.json) so config.Load itself succeeds and
// this isolates the InitDB failure. st.db must stay nil.
func TestServerOnStart_InitDBError(t *testing.T) {
	t.Setenv("OUROBOROS_CONFIG_DIR", t.TempDir())

	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	t.Setenv("PROJECT_KB_PATH", filepath.Join(blocker, "sub", "kb.db"))

	st := &serverState{}
	err := serverOnStart(st)(context.Background())
	require.Error(t, err)
	assert.Nil(t, st.db)
}

// TestServerOnStart_PopulatesDB exercises the real serverOnStart chain
// (config.Load -> store.InitDB) without touching mcpkit.App.Run or real
// stdio: PROJECT_KB_PATH (config.Load's primary env var, internal/config's
// config.go) points at a temp-dir SQLite path, so this opens a real (if
// throwaway) DB file and confirms st.db is live before handing it back to
// serverOnShutdown for cleanup.
func TestServerOnStart_PopulatesDB(t *testing.T) {
	t.Setenv("PROJECT_KB_PATH", filepath.Join(t.TempDir(), "kb.db"))

	st := &serverState{}
	require.NoError(t, serverOnStart(st)(context.Background()))
	require.NotNil(t, st.db)
	require.NoError(t, st.db.Ping())

	require.NoError(t, serverOnShutdown(st)(context.Background()))
}

// TestServerOnShutdown_NilDB_NoOp covers serverOnShutdown's nil-guard
// branch: a serverState whose db was never populated (e.g. OnStart never
// ran, or failed before assigning st.db) must shut down as a clean no-op,
// not panic on a nil *sql.DB.
func TestServerOnShutdown_NilDB_NoOp(t *testing.T) {
	st := &serverState{}
	require.NoError(t, serverOnShutdown(st)(context.Background()))
}
