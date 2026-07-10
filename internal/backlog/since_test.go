package backlog_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
)

func TestParseSinceCutoffDuration(t *testing.T) {
	before := time.Now().UTC()
	cutoff, err := backlog.ParseSinceCutoff("24h")
	require.NoError(t, err)

	got, err := time.Parse(time.RFC3339, cutoff)
	require.NoError(t, err)
	assert.WithinDuration(t, before.Add(-24*time.Hour), got, 5*time.Second)
}

func TestParseSinceCutoffDaysShorthand(t *testing.T) {
	before := time.Now().UTC()
	cutoff, err := backlog.ParseSinceCutoff("7d")
	require.NoError(t, err)

	got, err := time.Parse(time.RFC3339, cutoff)
	require.NoError(t, err)
	assert.WithinDuration(t, before.Add(-7*24*time.Hour), got, 5*time.Second)
}

func TestParseSinceCutoffDate(t *testing.T) {
	cutoff, err := backlog.ParseSinceCutoff("2026-01-01")
	require.NoError(t, err)
	assert.Equal(t, "2026-01-01T00:00:00Z", cutoff)
}

func TestParseSinceCutoffRFC3339(t *testing.T) {
	cutoff, err := backlog.ParseSinceCutoff("2026-01-01T12:30:00Z")
	require.NoError(t, err)
	assert.Equal(t, "2026-01-01T12:30:00Z", cutoff)
}

func TestParseSinceCutoffInvalid_Errors(t *testing.T) {
	_, err := backlog.ParseSinceCutoff("not-a-date")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid since value "not-a-date"`)
}
