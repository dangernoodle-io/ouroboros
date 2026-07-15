package backlog_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
)

func TestParsePriorityStrict(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{"P0", "P0", 0, false},
		{"P1", "P1", 1, false},
		{"P6", "P6", 6, false},
		{"invalid format (out of range)", "P7", 0, true},
		{"invalid prefix", "X1", 0, true},
		{"invalid value (negative)", "P-1", 0, true},
		{"no prefix", "1", 0, true},
		{"empty", "", 0, true},
		{"lowercase p rejected", "p0", 0, true},
		{"multi-digit rejected", "P10", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := backlog.ParsePriorityStrict(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
