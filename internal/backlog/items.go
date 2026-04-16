package backlog

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Item struct {
	ID          string `json:"id"`
	ProjectID   int64  `json:"project_id"`
	Priority    string `json:"priority"`
	Title       string `json:"title"`
	Component   string `json:"component"`
	Description string `json:"description"`
	Notes       string `json:"notes,omitempty"`
	Status      string `json:"status"`
	Created     string `json:"created"`
	Updated     string `json:"updated"`
}

func AddItem(d *sql.DB, projectID int64, prefix, priority, title, description, notes, component string) (*Item, error) {
	tx, err := d.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	var seq int64
	err = tx.QueryRow("SELECT COALESCE(MAX(seq), 0) + 1 FROM items WHERE project_id = ?", projectID).Scan(&seq)
	if err != nil {
		return nil, err
	}

	id := fmt.Sprintf("%s-%d", prefix, seq)
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = tx.Exec(
		"INSERT INTO items (id, project_id, seq, priority, title, description, notes, component, status, created, updated) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'open', ?, ?)",
		id, projectID, seq, priority, title, description, notes, component, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("add item: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &Item{
		ID: id, ProjectID: projectID, Priority: priority,
		Title: title, Component: component, Description: description, Notes: notes, Status: "open",
		Created: now, Updated: now,
	}, nil
}

func GetItem(d *sql.DB, id string) (*Item, error) {
	var item Item
	err := d.QueryRow(
		"SELECT id, project_id, priority, title, component, description, notes, status, created, updated FROM items WHERE id = ?", id,
	).Scan(&item.ID, &item.ProjectID, &item.Priority, &item.Title, &item.Component, &item.Description, &item.Notes, &item.Status, &item.Created, &item.Updated)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("item not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func UpdateItem(d *sql.DB, id string, fields map[string]string) (*Item, error) {
	allowed := map[string]bool{"priority": true, "title": true, "description": true, "notes": true, "status": true, "component": true}

	var sets []string
	var args []interface{}
	for k, v := range fields {
		if !allowed[k] {
			return nil, fmt.Errorf("invalid field: %s", k)
		}
		sets = append(sets, k+" = ?")
		args = append(args, v)
	}
	if len(sets) == 0 {
		return GetItem(d, id)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	sets = append(sets, "updated = ?")
	args = append(args, now)
	args = append(args, id)

	_, err := d.Exec("UPDATE items SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	if err != nil {
		return nil, fmt.Errorf("update item: %w", err)
	}
	return GetItem(d, id)
}

func MarkDone(d *sql.DB, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := d.Exec("UPDATE items SET status = 'done', updated = ? WHERE id = ?", now, id)
	if err != nil {
		return fmt.Errorf("mark done: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("item not found: %s", id)
	}
	return nil
}

func DeleteItems(d *sql.DB, ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	result, err := d.Exec("DELETE FROM items WHERE id IN ("+strings.Join(placeholders, ",")+") ", args...)
	if err != nil {
		return 0, fmt.Errorf("delete items: %w", err)
	}
	return result.RowsAffected()
}

type ItemFilter struct {
	ProjectID   *int64
	PriorityMin *int
	PriorityMax *int
	Status      *string
	Component   *string
}

func ListItems(d *sql.DB, f ItemFilter) ([]Item, error) {
	query := "SELECT id, project_id, priority, title, component, description, status, created, updated FROM items WHERE 1=1"
	var args []interface{}

	if f.ProjectID != nil {
		query += " AND project_id = ?"
		args = append(args, *f.ProjectID)
	}
	if f.PriorityMin != nil {
		query += " AND CAST(SUBSTR(priority, 2) AS INTEGER) >= ?"
		args = append(args, *f.PriorityMin)
	}
	if f.PriorityMax != nil {
		query += " AND CAST(SUBSTR(priority, 2) AS INTEGER) <= ?"
		args = append(args, *f.PriorityMax)
	}
	if f.Status != nil {
		query += " AND status = ?"
		args = append(args, *f.Status)
	}
	if f.Component != nil {
		query += " AND component = ?"
		args = append(args, *f.Component)
	}

	query += " ORDER BY CAST(SUBSTR(priority, 2) AS INTEGER), id"

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var items []Item
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ID, &item.ProjectID, &item.Priority, &item.Title, &item.Component, &item.Description, &item.Status, &item.Created, &item.Updated); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
