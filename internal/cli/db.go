package cli

import (
	"database/sql"
	"fmt"

	"dangernoodle.io/ouroboros/internal/store"
)

func withDB(fn func(*sql.DB) error) error {
	db, err := store.InitDB()
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close() //nolint:errcheck
	return fn(db)
}
