// Package dashboard implements the data-capture layer for the ouroboros
// dashboard: config-declared segment producers each emit a strict NDJSON
// fragment contract on stdout; one core parser consumes fragments fail-open.
// The whole layer is gated by a global master toggle (dashboard.enabled,
// default off).
package dashboard

// Context is the stdin schema every segment producer receives.
type Context struct {
	Schema         int    `json:"schema"`
	Project        string `json:"project,omitempty"`
	Repo           string `json:"repo,omitempty"`
	Cwd            string `json:"cwd,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
	SessionStart   string `json:"session_start,omitempty"`
	Now            string `json:"now"`
	SinceLast      string `json:"since_last,omitempty"`
	TranscriptPath string `json:"transcript_path,omitempty"`
}
