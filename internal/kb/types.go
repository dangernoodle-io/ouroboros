package kb

// Entry represents a knowledge base entry for batch operations.
type Entry struct {
	Type     string            `json:"type"`
	Project  string            `json:"project,omitempty"`
	Category string            `json:"category,omitempty"`
	Title    string            `json:"title"`
	Content  string            `json:"content"`
	Notes    string            `json:"notes,omitempty"`
	Tags     []string          `json:"tags,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// EntryUpdate is a partial, id-addressed KB document update. Pointer fields
// distinguish "not provided" (nil, leave unchanged) from an explicit value —
// this is what lets a title-only update retitle a document in place instead
// of creating a duplicate under the natural-key upsert.
// AppendNotes, when true, appends Notes to the existing document's notes
// (\n\n-separated) instead of replacing them; a nil or empty Notes is a
// no-op in append mode (nothing to append). Ignored unless Notes is
// non-nil and non-empty; has no effect on any other field.
type EntryUpdate struct {
	ID          int64
	Type        *string
	Project     *string
	Category    *string
	Title       *string
	Content     *string
	Notes       *string
	AppendNotes bool
	Tags        *[]string
	Metadata    *map[string]string
}

// PutResult represents the result of a put operation.
type PutResult struct {
	ID              int64  `json:"id"`
	Action          string `json:"action"`
	Title           string `json:"title"`
	Conflict        bool   `json:"conflict,omitempty"`
	PreviousExcerpt string `json:"previous_excerpt,omitempty"`
}
