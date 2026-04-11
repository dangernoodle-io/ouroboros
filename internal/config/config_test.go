package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PROJECT_KB_PATH", "")
	t.Setenv("QM_DB_PATH", "")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Contains(t, cfg.DBPath, ".local/share/ouroboros")
	assert.Equal(t, "none", cfg.BackupMode)
}

func TestLoadFromFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PROJECT_KB_PATH", "")
	t.Setenv("QM_DB_PATH", "")

	bootstrapDir := filepath.Join(tmpHome, ".config", "ouroboros")
	require.NoError(t, os.MkdirAll(bootstrapDir, 0o755))

	bootstrapFile := filepath.Join(bootstrapDir, "bootstrap.json")
	data := []byte(`{
  "db_path": "/custom/db.db",
  "backup": "dedicated"
}`)
	require.NoError(t, os.WriteFile(bootstrapFile, data, 0o644))

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "/custom/db.db", cfg.DBPath)
	assert.Equal(t, "dedicated", cfg.BackupMode)
}

func TestLoadEnvOverrides(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PROJECT_KB_PATH", "/env/db.db")
	t.Setenv("QM_DB_PATH", "/fallback/db.db")
	t.Setenv("QM_BACKUP_MODE", "shared")
	t.Setenv("QM_GIT_REPO", "/env/git")
	t.Setenv("QM_SPARSE_PATH", "/sparse")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "/env/db.db", cfg.DBPath)
	assert.Equal(t, "shared", cfg.BackupMode)
	assert.Equal(t, "/env/git", cfg.GitRepo)
	assert.Equal(t, "/sparse", cfg.SparseDir)
}

func TestLoadQMEnvFallback(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PROJECT_KB_PATH", "")
	t.Setenv("QM_DB_PATH", "/fallback/db.db")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "/fallback/db.db", cfg.DBPath)
}

func TestSaveAndLoad(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PROJECT_KB_PATH", "")
	t.Setenv("QM_DB_PATH", "")

	original := &Config{
		DBPath:     "/test/db.db",
		BackupMode: "dedicated",
		GitRepo:    "/test/git",
		SparseDir:  "sparse",
	}

	err := Save(original)
	require.NoError(t, err)

	loaded, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "/test/db.db", loaded.DBPath)
	assert.Equal(t, "dedicated", loaded.BackupMode)
	assert.Equal(t, "/test/git", loaded.GitRepo)
	assert.Equal(t, "sparse", loaded.SparseDir)
}

func TestExpandHome(t *testing.T) {
	home := "/home/testuser"
	tests := []struct {
		input    string
		expected string
	}{
		{"~/config", filepath.Join(home, "config")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}

	for _, tt := range tests {
		result := expandHome(tt.input)
		if tt.input == "~/config" {
			assert.True(t, filepath.IsAbs(result))
			assert.Contains(t, result, "config")
		} else {
			assert.Equal(t, tt.expected, result)
		}
	}
}

func TestBootstrapExists(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	assert.False(t, BootstrapExists())

	bootstrapDir := filepath.Join(tmpHome, ".config", "ouroboros")
	require.NoError(t, os.MkdirAll(bootstrapDir, 0o755))

	bootstrapFile := filepath.Join(bootstrapDir, "bootstrap.json")
	require.NoError(t, os.WriteFile(bootstrapFile, []byte("{}"), 0o644))

	assert.True(t, BootstrapExists())
}

func TestBootstrapPath(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	path := BootstrapPath()
	assert.Contains(t, path, "ouroboros")
	assert.Contains(t, path, "bootstrap.json")
}

func TestLoadFileOverridesDefaults(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PROJECT_KB_PATH", "")
	t.Setenv("QM_DB_PATH", "")

	bootstrapDir := filepath.Join(tmpHome, ".config", "ouroboros")
	require.NoError(t, os.MkdirAll(bootstrapDir, 0o755))

	bootstrapFile := filepath.Join(bootstrapDir, "bootstrap.json")
	data := []byte(`{"db_path": "/file/db.db"}`)
	require.NoError(t, os.WriteFile(bootstrapFile, data, 0o644))

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "/file/db.db", cfg.DBPath)
	assert.Equal(t, "none", cfg.BackupMode) // default value
}

func TestLoadPartialFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PROJECT_KB_PATH", "")
	t.Setenv("QM_DB_PATH", "")

	bootstrapDir := filepath.Join(tmpHome, ".config", "ouroboros")
	require.NoError(t, os.MkdirAll(bootstrapDir, 0o755))

	bootstrapFile := filepath.Join(bootstrapDir, "bootstrap.json")
	data := []byte(`{"backup": "shared"}`)
	require.NoError(t, os.WriteFile(bootstrapFile, data, 0o644))

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "shared", cfg.BackupMode)
	assert.Contains(t, cfg.DBPath, ".local/share/ouroboros") // default
}

func TestSaveCreatesDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cfg := &Config{
		DBPath:     "/test/db.db",
		BackupMode: "dedicated",
	}

	err := Save(cfg)
	require.NoError(t, err)

	bootstrapDir := filepath.Join(tmpHome, ".config", "ouroboros")
	info, err := os.Stat(bootstrapDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	bootstrapFile := filepath.Join(bootstrapDir, "bootstrap.json")
	data, err := os.ReadFile(bootstrapFile)
	require.NoError(t, err)

	var loaded Config
	err = json.Unmarshal(data, &loaded)
	require.NoError(t, err)
	assert.Equal(t, "/test/db.db", loaded.DBPath)
	assert.Equal(t, "dedicated", loaded.BackupMode)
}
