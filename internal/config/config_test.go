package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// neutralizeXDG clears the XDG/OUROBOROS override env vars xdgpath honors,
// so tests asserting a specific default/config path aren't sensitive to the
// ambient environment (a real XDG_DATA_HOME/XDG_CONFIG_HOME set outside the
// test process would otherwise redirect Load()'s resolved paths).
func neutralizeXDG(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("OUROBOROS_DATA_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OUROBOROS_CONFIG_DIR", "")
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PROJECT_KB_PATH", "")
	t.Setenv("QM_DB_PATH", "")
	neutralizeXDG(t)

	cfg, err := Load()
	require.NoError(t, err)
	assert.Contains(t, cfg.DBPath, filepath.Join(".local", "share", "ouroboros"))
}

func TestLoadFromFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PROJECT_KB_PATH", "")
	t.Setenv("QM_DB_PATH", "")
	neutralizeXDG(t)

	// OUROBOROS_CONFIG_DIR is used verbatim as the app config dir (highest
	// precedence in xdgpath's resolution), so the test controls exactly
	// where WithXDGFile looks, independent of $HOME/.config.
	bootstrapDir := t.TempDir()
	t.Setenv("OUROBOROS_CONFIG_DIR", bootstrapDir)

	bootstrapFile := filepath.Join(bootstrapDir, "bootstrap.json")
	data := []byte(`{
  "db_path": "/custom/db.db"
}`)
	require.NoError(t, os.WriteFile(bootstrapFile, data, 0o644))

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "/custom/db.db", cfg.DBPath)
}

func TestLoadEnvOverrides(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PROJECT_KB_PATH", "/env/db.db")
	t.Setenv("QM_DB_PATH", "/fallback/db.db")
	neutralizeXDG(t)

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "/env/db.db", cfg.DBPath)
}

func TestLoadQMEnvFallback(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PROJECT_KB_PATH", "")
	t.Setenv("QM_DB_PATH", "/fallback/db.db")
	neutralizeXDG(t)

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "/fallback/db.db", cfg.DBPath)
}

func TestLoadFileOverridesDefaults(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PROJECT_KB_PATH", "")
	t.Setenv("QM_DB_PATH", "")
	neutralizeXDG(t)

	bootstrapDir := t.TempDir()
	t.Setenv("OUROBOROS_CONFIG_DIR", bootstrapDir)

	bootstrapFile := filepath.Join(bootstrapDir, "bootstrap.json")
	data := []byte(`{"db_path": "/file/db.db"}`)
	require.NoError(t, os.WriteFile(bootstrapFile, data, 0o644))

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "/file/db.db", cfg.DBPath)
}

func TestLoadPartialFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PROJECT_KB_PATH", "")
	t.Setenv("QM_DB_PATH", "")
	neutralizeXDG(t)

	bootstrapDir := filepath.Join(tmpHome, ".config", "ouroboros")
	require.NoError(t, os.MkdirAll(bootstrapDir, 0o755))

	bootstrapFile := filepath.Join(bootstrapDir, "bootstrap.json")
	data := []byte(`{}`)
	require.NoError(t, os.WriteFile(bootstrapFile, data, 0o644))

	cfg, err := Load()
	require.NoError(t, err)
	assert.Contains(t, cfg.DBPath, filepath.Join(".local", "share", "ouroboros")) // default
}

func TestLoadTildeExpansion(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PROJECT_KB_PATH", "~/custom/db.db")
	t.Setenv("QM_DB_PATH", "")
	neutralizeXDG(t)

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmpHome, "custom", "db.db"), cfg.DBPath)
}

func TestLoadMalformedBootstrapErrors(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PROJECT_KB_PATH", "")
	t.Setenv("QM_DB_PATH", "")
	neutralizeXDG(t)

	bootstrapDir := t.TempDir()
	t.Setenv("OUROBOROS_CONFIG_DIR", bootstrapDir)

	bootstrapFile := filepath.Join(bootstrapDir, "bootstrap.json")
	require.NoError(t, os.WriteFile(bootstrapFile, []byte("not json"), 0o644))

	_, err := Load()
	require.Error(t, err)
}
