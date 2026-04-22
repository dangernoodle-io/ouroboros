package backlog

import (
	"database/sql"
	"fmt"
	"time"
)

type Project struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Prefix  string `json:"prefix"`
	Created string `json:"created"`
}

func CreateProject(db *sql.DB, name, prefix string) (*Project, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := db.Exec("INSERT INTO projects (name, prefix, created) VALUES (?, ?, ?)", name, prefix, now)
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}
	id, _ := result.LastInsertId()
	return &Project{ID: id, Name: name, Prefix: prefix, Created: now}, nil
}

func ListProjects(db *sql.DB) ([]Project, error) {
	rows, err := db.Query("SELECT id, name, prefix, created FROM projects ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Prefix, &p.Created); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

func GetProjectByName(db *sql.DB, name string) (*Project, error) {
	var p Project
	err := db.QueryRow("SELECT id, name, prefix, created FROM projects WHERE name = ?", name).
		Scan(&p.ID, &p.Name, &p.Prefix, &p.Created)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("project not found: %s", name)
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func GetProjectByID(db *sql.DB, id int64) (*Project, error) {
	var p Project
	err := db.QueryRow("SELECT id, name, prefix, created FROM projects WHERE id = ?", id).
		Scan(&p.ID, &p.Name, &p.Prefix, &p.Created)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("project not found: %d", id)
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func RenameProject(db *sql.DB, oldName, newName string) (*Project, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("rename project: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Validate oldName exists
	var projectID int64
	err = tx.QueryRow("SELECT id FROM projects WHERE name = ?", oldName).Scan(&projectID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("project not found: %s", oldName)
	}
	if err != nil {
		return nil, fmt.Errorf("rename project: %w", err)
	}

	// Validate newName does NOT exist
	var existing int
	err = tx.QueryRow("SELECT 1 FROM projects WHERE name = ?", newName).Scan(&existing)
	if err == nil {
		return nil, fmt.Errorf("project already exists: %s", newName)
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("rename project: %w", err)
	}

	// Update projects table
	_, err = tx.Exec("UPDATE projects SET name = ? WHERE id = ?", newName, projectID)
	if err != nil {
		return nil, fmt.Errorf("rename project: %w", err)
	}

	// Update documents table (cascade)
	_, err = tx.Exec("UPDATE documents SET project = ? WHERE project = ?", newName, oldName)
	if err != nil {
		return nil, fmt.Errorf("rename project: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("rename project: %w", err)
	}

	// Fetch and return refreshed project
	return GetProjectByName(db, newName)
}
