package backlog

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Plan struct {
	ID        int64   `json:"id"`
	ProjectID *int64  `json:"project_id,omitempty"`
	ItemID    *string `json:"item_id,omitempty"`
	Title     string  `json:"title"`
	Content   string  `json:"content"`
	Status    string  `json:"status"`
	Created   string  `json:"created"`
	Updated   string  `json:"updated"`
}

func CreatePlan(d *sql.DB, title, content string, projectID *int64, itemID *string) (*Plan, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := d.Exec(
		"INSERT INTO plans (project_id, item_id, title, content, status, created, updated) VALUES (?, ?, ?, ?, 'draft', ?, ?)",
		projectID, itemID, title, content, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create plan: %w", err)
	}
	id, _ := result.LastInsertId()
	return &Plan{
		ID: id, ProjectID: projectID, ItemID: itemID,
		Title: title, Content: content, Status: "draft",
		Created: now, Updated: now,
	}, nil
}

func GetPlan(d *sql.DB, id int64) (*Plan, error) {
	var p Plan
	err := d.QueryRow(
		"SELECT id, project_id, item_id, title, content, status, created, updated FROM plans WHERE id = ?", id,
	).Scan(&p.ID, &p.ProjectID, &p.ItemID, &p.Title, &p.Content, &p.Status, &p.Created, &p.Updated)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("plan not found: %d", id)
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func UpdatePlan(d *sql.DB, id int64, fields map[string]string) (*Plan, error) {
	allowed := map[string]bool{"title": true, "content": true, "status": true}

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
		return GetPlan(d, id)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	sets = append(sets, "updated = ?")
	args = append(args, now)
	args = append(args, id)

	_, err := d.Exec("UPDATE plans SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	if err != nil {
		return nil, fmt.Errorf("update plan: %w", err)
	}
	return GetPlan(d, id)
}

type PlanFilter struct {
	ProjectID *int64
	Status    *string
}

func ListPlans(d *sql.DB, f PlanFilter) ([]Plan, error) {
	query := "SELECT id, project_id, item_id, title, content, status, created, updated FROM plans WHERE 1=1"
	var args []interface{}

	if f.ProjectID != nil {
		query += " AND project_id = ?"
		args = append(args, *f.ProjectID)
	}
	if f.Status != nil {
		query += " AND status = ?"
		args = append(args, *f.Status)
	}

	query += " ORDER BY id"

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var plans []Plan
	for rows.Next() {
		var p Plan
		if err := rows.Scan(&p.ID, &p.ProjectID, &p.ItemID, &p.Title, &p.Content, &p.Status, &p.Created, &p.Updated); err != nil {
			return nil, err
		}
		plans = append(plans, p)
	}
	return plans, rows.Err()
}
