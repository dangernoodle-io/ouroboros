package backlog

import (
	"strconv"
	"strings"
)

// ParsePriority parses a "P<n>" priority string (e.g. "P0", case-insensitive,
// surrounding whitespace tolerated) into n. Ok is false for an
// empty/malformed value.
func ParsePriority(priority string) (int, bool) {
	p := strings.TrimSpace(priority)
	if len(p) < 2 || (p[0] != 'P' && p[0] != 'p') {
		return 0, false
	}
	n, err := strconv.Atoi(p[1:])
	if err != nil {
		return 0, false
	}
	return n, true
}
