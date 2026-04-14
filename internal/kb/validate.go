package kb

import (
	"fmt"

	"dangernoodle.io/ouroboros/internal/store"
)

const ContentMaxLen = 500

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
