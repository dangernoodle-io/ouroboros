package backlog_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"dangernoodle.io/ouroboros/internal/backlog"
)

func TestParsePriority(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		wantN  int
		wantOK bool
	}{
		{"upperP", "P0", 0, true},
		{"lowerP", "p2", 2, true},
		{"multiDigit", "P10", 10, true},
		{"surroundingWhitespace", " P3 ", 3, true},
		{"empty", "", 0, false},
		{"tooShort", "P", 0, false},
		{"badPrefix", "X0", 0, false},
		{"nonNumericSuffix", "PX", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, ok := backlog.ParsePriority(tc.in)
			assert.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				assert.Equal(t, tc.wantN, n)
			}
		})
	}
}
