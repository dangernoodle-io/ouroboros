package kb

import (
	"fmt"

	"dangernoodle.io/ouroboros/internal/store"
)

const ContentMaxLen = 500

// roadmap is intentionally excluded: the roadmap singleton doc is written
// exclusively via internal/roadmap.Save (and the transactional Mutate), which
// enforce roadmap-specific invariants (singleton row, structured metadata,
// content-cap truncation) that the generic kb write path does not.
var validTypes = map[string]bool{
	"decision": true,
	"fact":     true,
	"note":     true,
	"plan":     true,
	"relation": true,
}

// ValidateDocument enforces the put-tool contract: required fields,
// type enum, content cap. Returns nil if valid.
func ValidateDocument(doc store.Document) error {
	if doc.Type == "" {
		return fmt.Errorf("type is required")
	}
	if !validTypes[doc.Type] {
		return fmt.Errorf("invalid type %q (must be one of: decision, fact, note, plan, relation)", doc.Type)
	}
	if doc.Project == "" {
		return fmt.Errorf("project is required")
	}
	if doc.Title == "" {
		return fmt.Errorf("title is required")
	}
	if doc.Content == "" {
		return fmt.Errorf("content is required")
	}
	if len(doc.Content) > ContentMaxLen {
		return fmt.Errorf("content exceeds %d char hard cap (got %d) - move narrative into notes field", ContentMaxLen, len(doc.Content))
	}
	return nil
}

// ValidateEntryUpdate enforces the update-by-id contract: only provided
// (non-nil) fields are checked, but a provided value must still satisfy the
// same invariants as create — a required field can be repointed but not
// blanked (e.g. title="" is rejected, not a silent no-op).
func ValidateEntryUpdate(u EntryUpdate) error {
	if u.Type != nil {
		if *u.Type == "" {
			return fmt.Errorf("type cannot be empty")
		}
		if !validTypes[*u.Type] {
			return fmt.Errorf("invalid type %q (must be one of: decision, fact, note, plan, relation)", *u.Type)
		}
	}
	if u.Project != nil && *u.Project == "" {
		return fmt.Errorf("project cannot be empty")
	}
	if u.Title != nil && *u.Title == "" {
		return fmt.Errorf("title cannot be empty")
	}
	if u.Content != nil {
		if *u.Content == "" {
			return fmt.Errorf("content cannot be empty")
		}
		if len(*u.Content) > ContentMaxLen {
			return fmt.Errorf("content exceeds %d char hard cap (got %d) - move narrative into notes field", ContentMaxLen, len(*u.Content))
		}
	}
	return nil
}
