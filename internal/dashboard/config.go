package dashboard

import (
	"encoding/json"
	"time"
)

// Config keys under which the dashboard layer stores its settings in the
// backlog config table.
const (
	KeyEnabled     = "dashboard.enabled"
	KeySegments    = "dashboard.segments"
	KeyOutput      = "dashboard.output"
	KeyCooldown    = "dashboard.cooldown"
	KeyLastRefresh = "dashboard.last_refresh"
)

// MinCooldown floors any configured refresh cooldown.
const MinCooldown = 30 * time.Second

// defaultCooldown is used when dashboard.cooldown is unset or invalid.
const defaultCooldown = 60 * time.Second

// SegmentSpec declares one segment producer: either a built-in (Builtin) or
// an external command (Exec/Shell — not yet executed in this slice).
type SegmentSpec struct {
	ID string `json:"id"`
	// Every/Timeout are reserved for the lifecycle slice (per-segment
	// cadence + exec/shell producer timeout); not enforced yet — only the
	// global dashboard.cooldown gate applies in this slice.
	Every   string   `json:"every,omitempty"`
	Enabled *bool    `json:"enabled,omitempty"`
	Timeout string   `json:"timeout,omitempty"`
	Builtin string   `json:"builtin,omitempty"`
	Exec    []string `json:"exec,omitempty"`
	Shell   string   `json:"shell,omitempty"`
}

// IsEnabled reports whether the segment is enabled; a nil Enabled defaults
// to true.
func (s SegmentSpec) IsEnabled() bool {
	if s.Enabled == nil {
		return true
	}
	return *s.Enabled
}

// DefaultSegments is the segment list used when dashboard.segments is unset.
func DefaultSegments() []SegmentSpec {
	return []SegmentSpec{
		{ID: "git", Builtin: "git"},
		{ID: "roadmap", Builtin: "roadmap"},
	}
}

// ParseSegments unmarshals a dashboard.segments config value.
func ParseSegments(jsonStr string) ([]SegmentSpec, error) {
	var specs []SegmentSpec
	if err := json.Unmarshal([]byte(jsonStr), &specs); err != nil {
		return nil, err
	}
	return specs, nil
}

// ParseCooldown parses a dashboard.cooldown duration string, defaulting to
// 60s when empty or invalid, and flooring the result at MinCooldown.
func ParseCooldown(s string) time.Duration {
	d := defaultCooldown
	if s != "" {
		if parsed, err := time.ParseDuration(s); err == nil {
			d = parsed
		}
	}
	if d < MinCooldown {
		d = MinCooldown
	}
	return d
}
