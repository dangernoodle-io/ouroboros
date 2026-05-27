package cli

import (
	"database/sql"
	"fmt"

	"dangernoodle.io/ouroboros/internal/config"
	"dangernoodle.io/ouroboros/internal/store"
)

func withDB(fn func(*sql.DB) error) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	db, err := store.InitDB(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close() //nolint:errcheck
	return fn(db)
}
