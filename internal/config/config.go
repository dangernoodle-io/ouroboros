package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	DBPath     string `json:"db_path"`
	BackupMode string `json:"backup"`
	GitRepo    string `json:"git_repo,omitempty"`
	SparseDir  string `json:"sparse_path,omitempty"`
}

func defaultConfig() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		DBPath:     filepath.Join(home, ".local", "share", "ouroboros", "kb.db"),
		BackupMode: "none",
	}
}

func bootstrapPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ouroboros", "bootstrap.json")
}

func BootstrapPath() string {
	return bootstrapPath()
}

func BootstrapExists() bool {
	_, err := os.Stat(bootstrapPath())
	return err == nil
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

func Load() (*Config, error) {
	cfg := defaultConfig()

	data, err := os.ReadFile(bootstrapPath())
	if err == nil {
		var file Config
		if err := json.Unmarshal(data, &file); err == nil {
			if file.DBPath != "" {
				cfg.DBPath = file.DBPath
			}
			if file.BackupMode != "" {
				cfg.BackupMode = file.BackupMode
			}
			if file.GitRepo != "" {
				cfg.GitRepo = file.GitRepo
			}
			if file.SparseDir != "" {
				cfg.SparseDir = file.SparseDir
			}
		}
	}

	// PROJECT_KB_PATH takes priority, then QM_DB_PATH as alias
	if v := os.Getenv("PROJECT_KB_PATH"); v != "" {
		cfg.DBPath = v
	} else if v := os.Getenv("QM_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("QM_BACKUP_MODE"); v != "" {
		cfg.BackupMode = v
	}
	if v := os.Getenv("QM_GIT_REPO"); v != "" {
		cfg.GitRepo = v
	}
	if v := os.Getenv("QM_SPARSE_PATH"); v != "" {
		cfg.SparseDir = v
	}

	cfg.DBPath = expandHome(cfg.DBPath)
	cfg.GitRepo = expandHome(cfg.GitRepo)

	return cfg, nil
}

func Save(cfg *Config) error {
	path := bootstrapPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
