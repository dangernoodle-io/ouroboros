package config

import (
	"os"

	sheshaconfig "github.com/dangernoodle-io/shesha/config"
	"github.com/dangernoodle-io/shesha/xdgpath"
)

type Config struct {
	DBPath string `json:"db_path"`
}

// Load builds the ouroboros config from defaults, the bootstrap.json
// overlay, and env vars, in that precedence. DBPath defaults to
// ~/.local/share/ouroboros/kb.db (honors XDG_DATA_HOME/OUROBOROS_DATA_DIR),
// is overlaid by bootstrap.json's db_path if present, then by
// PROJECT_KB_PATH (primary) or QM_DB_PATH (alias) if set. A leading "~/" in
// the final DBPath is expanded. bootstrap.json's location is
// ~/.config/ouroboros/bootstrap.json by default, overridable via
// XDG_CONFIG_HOME/OUROBOROS_CONFIG_DIR.
func Load() (*Config, error) {
	defaults := Config{DBPath: xdgpath.DataFile("ouroboros", "kb.db")}

	cfg, err := sheshaconfig.Load(defaults, sheshaconfig.WithXDGFile("ouroboros", "bootstrap.json"))
	if err != nil {
		return nil, err
	}

	// PROJECT_KB_PATH takes priority, then QM_DB_PATH as alias. shesha's
	// WithEnv field-maps by struct field name and doesn't know about this
	// alias, so it's applied by hand.
	if v := os.Getenv("PROJECT_KB_PATH"); v != "" {
		cfg.DBPath = v
	} else if v := os.Getenv("QM_DB_PATH"); v != "" {
		cfg.DBPath = v
	}

	cfg.DBPath = sheshaconfig.ExpandHome(cfg.DBPath)

	return &cfg, nil
}
