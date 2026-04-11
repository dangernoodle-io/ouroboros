package backlog

import (
	"database/sql"
	"fmt"
)

func GetConfig(d *sql.DB, key string) (string, error) {
	var value string
	err := d.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("config key not found: %s", key)
	}
	return value, err
}

func SetConfig(d *sql.DB, key, value string) error {
	_, err := d.Exec("INSERT OR REPLACE INTO config (key, value) VALUES (?, ?)", key, value)
	return err
}

func GetAllConfig(d *sql.DB) (map[string]string, error) {
	rows, err := d.Query("SELECT key, value FROM config ORDER BY key")
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	config := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		config[k] = v
	}
	return config, rows.Err()
}
