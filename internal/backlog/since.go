package backlog

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// daysSuffixRe matches a bare "Nd" duration shorthand (e.g. "7d"), which
// time.ParseDuration doesn't support natively (no day unit).
var daysSuffixRe = regexp.MustCompile(`^(\d+)d$`)

// ParseSinceCutoff resolves a "since" filter value (CLI --since / MCP since
// arg) to an RFC3339 (UTC) cutoff timestamp, comparable against Item.Created
// via string >=. Accepts a duration (Go's time.ParseDuration units, or an
// "Nd" days shorthand) computed relative to now, an RFC3339 timestamp, or a
// bare "2006-01-02" date. An unrecognized value is a clear error, not a
// silent no-op filter — shared by the CLI and MCP get/search domain=backlog
// so both parse identically.
func ParseSinceCutoff(s string) (string, error) {
	now := time.Now().UTC()
	if m := daysSuffixRe.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		return now.Add(-time.Duration(n) * 24 * time.Hour).UTC().Format(time.RFC3339), nil
	}
	if d, err := time.ParseDuration(s); err == nil {
		return now.Add(-d).UTC().Format(time.RFC3339), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	return "", fmt.Errorf("invalid since value %q: expected a duration (e.g. 24h, 7d), a date (2006-01-02), or an RFC3339 timestamp", s)
}
