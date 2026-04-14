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

// PutResult represents the result of a put operation.
type PutResult struct {
	ID     int64  `json:"id"`
	Action string `json:"action"`
	Title  string `json:"title"`
}
