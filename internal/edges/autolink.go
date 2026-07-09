package edges

import (
	"database/sql"
	"regexp"
	"strconv"
	"strings"
)

// linkPattern matches [[Title]] wiki-style references in KB content.
var linkPattern = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)

// itemIDPattern matches a backlog item id shape, e.g. "BB-9" or "TM-40".
var itemIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*-\d+$`)

// AutolinkKB parses [[Title]] references out of a KB document's content and
// creates "explains" edges from the document to each resolved target: an
// item-id-shaped title resolves against backlog items; otherwise it
// resolves against a same-project KB document title. An unresolved title is
// silently skipped (no error — autolinking is best-effort, never a write
// failure). Idempotent: deletes the document's prior autolinked
// (source=doc, label=explains) edges before recreating, so a re-write drops
// stale links and adds new ones without duplicating. Returns the count of
// edges created.
func AutolinkKB(ex Executor, docID int64, project, content string) (int, error) {
	sourceID := strconv.FormatInt(docID, 10)
	if _, err := DeleteAutolinks(ex, TypeKB, sourceID, "explains"); err != nil {
		return 0, err
	}

	matches := linkPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return 0, nil
	}

	seen := make(map[string]bool, len(matches))
	count := 0
	for _, m := range matches {
		title := strings.TrimSpace(m[1])
		if title == "" || seen[title] {
			continue
		}
		seen[title] = true

		if itemIDPattern.MatchString(title) && itemExists(ex, title) {
			if _, err := Link(ex, TypeKB, sourceID, "explains", TypeItem, title, 0); err != nil {
				return count, err
			}
			count++
			continue
		}

		targetID, ok := resolveKBTitle(ex, project, title, docID)
		if !ok {
			continue
		}
		if _, err := Link(ex, TypeKB, sourceID, "explains", TypeKB, targetID, 0); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func itemExists(ex Executor, id string) bool {
	var one int
	err := ex.QueryRow("SELECT 1 FROM items WHERE id = ?", id).Scan(&one)
	return err == nil
}

// resolveKBTitle looks up a KB document by (project, title), excluding the
// autolinking document itself so a title matching its own title never
// self-links.
func resolveKBTitle(ex Executor, project, title string, excludeID int64) (string, bool) {
	var id int64
	err := ex.QueryRow("SELECT id FROM documents WHERE project = ? AND title = ? AND id != ? LIMIT 1", project, title, excludeID).Scan(&id)
	if err == sql.ErrNoRows || err != nil {
		return "", false
	}
	return strconv.FormatInt(id, 10), true
}
