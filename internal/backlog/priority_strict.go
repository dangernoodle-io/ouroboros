package backlog

import (
	"fmt"
	"strconv"
)

// ParsePriorityStrict parses a "P0"-"P6" priority string, returning an error
// (not a bool) for an invalid value. Unlike ParsePriority (case-insensitive,
// unbounded, used by lenient CLI filters), this requires an exact two-byte
// "P<digit>" form: uppercase "P" only, a single digit 0-6. Shared by the
// backlog write path (internal/app) and the backlog read/query path
// (internal/query), which previously carried byte-identical private copies
// of this validator (OU-329).
func ParsePriorityStrict(s string) (int, error) {
	if len(s) != 2 || s[0] != 'P' {
		return 0, fmt.Errorf("invalid priority: %s (expected P0-P6)", s)
	}
	n, err := strconv.Atoi(string(s[1]))
	if err != nil || n < 0 || n > 6 {
		return 0, fmt.Errorf("invalid priority: %s (expected P0-P6)", s)
	}
	return n, nil
}
