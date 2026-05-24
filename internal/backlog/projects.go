package backlog

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var projectNameRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{2,}$`)
var prefixShapedRE = regexp.MustCompile(`^[A-Z]{1,4}$`)

func ValidateProjectName(name string) error {
	if name == "" {
		return errors.New("project name is required")
	}
	if !projectNameRE.MatchString(name) {
		return fmt.Errorf("invalid project name %q: must start with a letter and contain only letters, digits, underscore, or hyphen (min 3 chars)", name)
	}
	if prefixShapedRE.MatchString(name) {
		return fmt.Errorf("invalid project name %q: looks like a prefix (1-4 uppercase letters); use a descriptive name", name)
	}
	return nil
}

type Project struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Prefix  string `json:"prefix"`
	Created string `json:"created"`
}

func CreateProject(db *sql.DB, name, prefix string) (*Project, error) {
	if err := ValidateProjectName(name); err != nil {
		return nil, err
	}
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
	err := db.QueryRow("SELECT id, name, prefix, created FROM projects WHERE LOWER(name) = LOWER(?)", name).
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

// ReassignProjectChildren moves items, plans, and documents from one project to another within a transaction.
// Item and plan IDs encode the source project's prefix (e.g. "OU-12") and are NOT rewritten —
// reassigned children keep their original prefix-encoded IDs to preserve history.
func ReassignProjectChildren(tx *sql.Tx, fromID int64, fromName string, toID int64, toName string) error {
	if _, err := tx.Exec("UPDATE items SET project_id = ? WHERE project_id = ?", toID, fromID); err != nil {
		return fmt.Errorf("reassign items: %w", err)
	}
	if _, err := tx.Exec("UPDATE plans SET project_id = ? WHERE project_id = ?", toID, fromID); err != nil {
		return fmt.Errorf("reassign plans: %w", err)
	}
	if _, err := tx.Exec("UPDATE documents SET project = ? WHERE LOWER(project) = LOWER(?)", toName, fromName); err != nil {
		return fmt.Errorf("reassign documents: %w", err)
	}
	return nil
}

// DeleteProject deletes a project by name.
// Mutual exclusion: force and reassignTo cannot both be set.
// If reassignTo is set, children are moved to the target project before deletion.
// If force is set, all children (items, plans, documents) are cascade-deleted.
// If neither is set and children exist, returns a blocking error with counts.
func DeleteProject(db *sql.DB, name string, force bool, reassignTo string) error {
	if force && reassignTo != "" {
		return errors.New("force and reassign_to are mutually exclusive")
	}

	src, err := GetProjectByName(db, name)
	if err != nil {
		return fmt.Errorf("project not found: %q", name)
	}

	if reassignTo != "" {
		dst, err := GetProjectByName(db, reassignTo)
		if err != nil {
			return fmt.Errorf("reassign target not found: %q", reassignTo)
		}
		if dst.ID == src.ID {
			return fmt.Errorf("cannot reassign to self: %q", name)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("delete project: %w", err)
		}
		defer tx.Rollback() //nolint:errcheck

		if err := ReassignProjectChildren(tx, src.ID, src.Name, dst.ID, dst.Name); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM projects WHERE id = ?", src.ID); err != nil {
			return fmt.Errorf("delete project: %w", err)
		}
		return tx.Commit()
	}

	if force {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("delete project: %w", err)
		}
		defer tx.Rollback() //nolint:errcheck

		if _, err := tx.Exec("DELETE FROM documents WHERE LOWER(project) = LOWER(?)", src.Name); err != nil {
			return fmt.Errorf("delete project documents: %w", err)
		}
		if _, err := tx.Exec("DELETE FROM items WHERE project_id = ?", src.ID); err != nil {
			return fmt.Errorf("delete project items: %w", err)
		}
		if _, err := tx.Exec("DELETE FROM plans WHERE project_id = ?", src.ID); err != nil {
			return fmt.Errorf("delete project plans: %w", err)
		}
		if _, err := tx.Exec("DELETE FROM projects WHERE id = ?", src.ID); err != nil {
			return fmt.Errorf("delete project: %w", err)
		}
		return tx.Commit()
	}

	// Default path: block if children exist.
	var itemCount, planCount, docCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM items WHERE project_id = ?", src.ID).Scan(&itemCount); err != nil {
		return fmt.Errorf("count items: %w", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM plans WHERE project_id = ?", src.ID).Scan(&planCount); err != nil {
		return fmt.Errorf("count plans: %w", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM documents WHERE LOWER(project) = LOWER(?)", src.Name).Scan(&docCount); err != nil {
		return fmt.Errorf("count documents: %w", err)
	}
	if itemCount > 0 || planCount > 0 || docCount > 0 {
		return fmt.Errorf("cannot delete project %q: has %d items, %d plans, %d documents — pass force=true to cascade or reassign_to=<name> to move", name, itemCount, planCount, docCount)
	}

	if _, err := db.Exec("DELETE FROM projects WHERE id = ?", src.ID); err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	return nil
}

// DerivePrefix computes a unique 2-character uppercase prefix for the given project name.
// It uses the first two letters of the uppercased name, falling back to <letter>1–<letter>9
// if the base candidate is already taken.
func DerivePrefix(db *sql.DB, name string) (string, error) {
	base := strings.ToUpper(name)
	if len(base) < 2 {
		base = base + "X"
	}
	prefix := base[:2]

	projects, err := ListProjects(db)
	if err != nil {
		return "", err
	}

	existing := make(map[string]bool)
	for _, p := range projects {
		existing[p.Prefix] = true
	}

	if !existing[prefix] {
		return prefix, nil
	}

	for i := 1; i <= 9; i++ {
		candidate := fmt.Sprintf("%c%d", prefix[0], i)
		if !existing[candidate] {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("cannot derive unique prefix for: %s", name)
}

func RenameProject(db *sql.DB, oldName, newName string) (*Project, error) {
	if err := ValidateProjectName(newName); err != nil {
		return nil, err
	}
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

	// Validate newName does NOT exist (case-insensitive, exclude self)
	var existing int
	err = tx.QueryRow("SELECT 1 FROM projects WHERE LOWER(name) = LOWER(?) AND id != ?", newName, projectID).Scan(&existing)
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
