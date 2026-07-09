package backlog

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var projectNameRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{2,}$`)
var prefixShapedRE = regexp.MustCompile(`^[A-Z]{1,4}$`)
var validPrefixRE = regexp.MustCompile(`^[A-Z][A-Z0-9]{0,3}$`)

func ValidatePrefix(prefix string) error {
	if prefix == "" {
		return errors.New("prefix is required")
	}
	if !validPrefixRE.MatchString(prefix) {
		return fmt.Errorf("invalid prefix %q: must be 1-4 chars, start with a letter, contain only letters and digits", prefix)
	}
	return nil
}

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
	now := nowRFC3339()
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
	if _, err := tx.Exec("UPDATE edges SET project_id = ? WHERE project_id = ?", toID, fromID); err != nil {
		return fmt.Errorf("reassign edges: %w", err)
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

		// Cascade-delete every edge whose endpoint belongs to this project,
		// BEFORE deleting the items/documents themselves so the subqueries
		// still resolve. The edges.project_id FK's ON DELETE CASCADE alone
		// is insufficient: an edge created via CLI `link` or a [[Title]]
		// autolink always stores project_id NULL (never set), so it would
		// never be caught by that FK and would dangle over now-deleted
		// endpoints. This bulk delete-by-endpoint is the real cascade; the
		// FK cascade is defensive belt-and-suspenders for edges that do
		// carry a project_id. Documents are matched by the same
		// case-insensitive `project` name column the delete below uses
		// (documents.project_id is never populated on write — see
		// upsertDocumentExec — so matching on it here would silently match
		// nothing).
		if _, err := tx.Exec(`
			DELETE FROM edges
			WHERE (source_type = 'item' AND source_id IN (SELECT id FROM items WHERE project_id = ?))
			   OR (target_type = 'item' AND target_id IN (SELECT id FROM items WHERE project_id = ?))
			   OR (source_type = 'kb'   AND source_id IN (SELECT CAST(id AS TEXT) FROM documents WHERE LOWER(project) = LOWER(?)))
			   OR (target_type = 'kb'   AND target_id IN (SELECT CAST(id AS TEXT) FROM documents WHERE LOWER(project) = LOWER(?)))
		`, src.ID, src.ID, src.Name, src.Name); err != nil {
			return fmt.Errorf("delete project edges: %w", err)
		}
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

func RenameProject(db *sql.DB, oldName, newName, newPrefix string) (*Project, error) {
	if newName == "" && newPrefix == "" {
		return nil, errors.New("at least one of new_name or new_prefix is required")
	}

	project, err := GetProjectByName(db, oldName)
	if err != nil {
		return nil, fmt.Errorf("project not found: %s", oldName)
	}

	if newName != "" {
		if err := ValidateProjectName(newName); err != nil {
			return nil, err
		}
	}
	if newPrefix != "" {
		if err := ValidatePrefix(newPrefix); err != nil {
			return nil, err
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("rename project: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if newName != "" {
		// Case-insensitive collision check, excluding self
		var collision int
		err = tx.QueryRow("SELECT 1 FROM projects WHERE LOWER(name) = LOWER(?) AND id != ?", newName, project.ID).Scan(&collision)
		if err == nil {
			return nil, fmt.Errorf("project already exists: %s", newName)
		}
		if err != sql.ErrNoRows {
			return nil, fmt.Errorf("rename project: %w", err)
		}

		if _, err = tx.Exec("UPDATE projects SET name = ? WHERE id = ?", newName, project.ID); err != nil {
			return nil, fmt.Errorf("rename project: %w", err)
		}
		// Case-insensitive doc cascade
		if _, err = tx.Exec("UPDATE documents SET project = ? WHERE LOWER(project) = LOWER(?)", newName, oldName); err != nil {
			return nil, fmt.Errorf("rename project: %w", err)
		}
	}

	if newPrefix != "" {
		renamedAt := nowRFC3339()
		if err := renamePrefixInTx(tx, project.ID, newPrefix, renamedAt); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("rename project: %w", err)
	}

	fetchName := newName
	if fetchName == "" {
		fetchName = oldName
	}
	return GetProjectByName(db, fetchName)
}

func renamePrefixInTx(tx *sql.Tx, projectID int64, newPrefix, renamedAt string) error {
	// Defer FK enforcement to commit so we can rewrite items.id and plans.item_id
	// in any order within this tx — both reference each other, so any sequential
	// order would violate FK mid-tx. Resets automatically at commit.
	if _, err := tx.Exec("PRAGMA defer_foreign_keys = ON"); err != nil {
		return fmt.Errorf("rename prefix: %w", err)
	}

	// Collision check
	var collision int
	err := tx.QueryRow("SELECT 1 FROM projects WHERE prefix = ? AND id != ?", newPrefix, projectID).Scan(&collision)
	if err == nil {
		return fmt.Errorf("prefix already in use: %s", newPrefix)
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("rename prefix: %w", err)
	}

	// Collect all items for this project
	rows, err := tx.Query("SELECT id, seq FROM items WHERE project_id = ? ORDER BY seq", projectID)
	if err != nil {
		return fmt.Errorf("rename prefix: %w", err)
	}
	type itemRef struct {
		oldID string
		seq   int64
	}
	var items []itemRef
	for rows.Next() {
		var ref itemRef
		if err := rows.Scan(&ref.oldID, &ref.seq); err != nil {
			rows.Close()
			return fmt.Errorf("rename prefix: %w", err)
		}
		items = append(items, ref)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rename prefix: %w", err)
	}

	// Per-row order matters under FK enforcement:
	//   1. UPDATE plans (both old & new IDs absent from items briefly is OK; plans points at oldID still valid)
	//   2. UPDATE items (triggers ON UPDATE CASCADE on item_id_aliases.new_id — any prior alias pointing at oldID auto-follows)
	//   3. INSERT new alias (oldID → newID) — newID now exists in items so FK satisfied
	for _, ref := range items {
		newID := fmt.Sprintf("%s-%d", newPrefix, ref.seq)

		if _, err = tx.Exec("UPDATE plans SET item_id = ? WHERE item_id = ?", newID, ref.oldID); err != nil {
			return fmt.Errorf("rename prefix plans %s: %w", ref.oldID, err)
		}

		if _, err = tx.Exec("UPDATE items SET id = ? WHERE id = ?", newID, ref.oldID); err != nil {
			return fmt.Errorf("rename prefix item %s: %w", ref.oldID, err)
		}

		// Cascade-update any edge referencing the old item id (source or
		// target) — keeps edges.source_id/target_id canonical rather than
		// relying solely on read-time item_id_aliases resolution. OR IGNORE
		// drops the update on the rare unique-constraint collision (both
		// old and new id already held an identical edge) instead of
		// aborting the rename; the DELETE that follows then removes the
		// now-redundant old-id row left behind by that collision so no
		// stale, unreachable edge survives (the new-id equivalent already
		// exists).
		if _, err = tx.Exec("UPDATE OR IGNORE edges SET source_id = ? WHERE source_type = 'item' AND source_id = ?", newID, ref.oldID); err != nil {
			return fmt.Errorf("rename prefix edges source %s: %w", ref.oldID, err)
		}
		if _, err = tx.Exec("DELETE FROM edges WHERE source_type = 'item' AND source_id = ?", ref.oldID); err != nil {
			return fmt.Errorf("rename prefix edges source cleanup %s: %w", ref.oldID, err)
		}
		if _, err = tx.Exec("UPDATE OR IGNORE edges SET target_id = ? WHERE target_type = 'item' AND target_id = ?", newID, ref.oldID); err != nil {
			return fmt.Errorf("rename prefix edges target %s: %w", ref.oldID, err)
		}
		if _, err = tx.Exec("DELETE FROM edges WHERE target_type = 'item' AND target_id = ?", ref.oldID); err != nil {
			return fmt.Errorf("rename prefix edges target cleanup %s: %w", ref.oldID, err)
		}

		_, err = tx.Exec(
			`INSERT INTO item_id_aliases (old_id, new_id, renamed_at) VALUES (?, ?, ?)
             ON CONFLICT(old_id) DO UPDATE SET new_id=excluded.new_id, renamed_at=excluded.renamed_at`,
			ref.oldID, newID, renamedAt,
		)
		if err != nil {
			return fmt.Errorf("rename prefix alias %s: %w", ref.oldID, err)
		}
	}

	// Update project prefix
	if _, err = tx.Exec("UPDATE projects SET prefix = ? WHERE id = ?", newPrefix, projectID); err != nil {
		return fmt.Errorf("rename prefix: %w", err)
	}

	return nil
}
