package dashboard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultSegments(t *testing.T) {
	segs := DefaultSegments()
	require.Len(t, segs, 2)
	assert.Equal(t, "git", segs[0].ID)
	assert.Equal(t, "git", segs[0].Builtin)
	assert.Equal(t, "roadmap", segs[1].ID)
	assert.Equal(t, "roadmap", segs[1].Builtin)
}

func TestParseSegments(t *testing.T) {
	specs, err := ParseSegments(`[{"id":"git","builtin":"git"},{"id":"custom","exec":["echo","hi"]}]`)
	require.NoError(t, err)
	require.Len(t, specs, 2)
	assert.Equal(t, "git", specs[0].Builtin)
	assert.Equal(t, []string{"echo", "hi"}, specs[1].Exec)
}

func TestParseSegments_Invalid(t *testing.T) {
	_, err := ParseSegments(`not json`)
	require.Error(t, err)
}

func TestSegmentSpec_IsEnabled(t *testing.T) {
	trueVal := true
	falseVal := false

	assert.True(t, SegmentSpec{}.IsEnabled())
	assert.True(t, SegmentSpec{Enabled: &trueVal}.IsEnabled())
	assert.False(t, SegmentSpec{Enabled: &falseVal}.IsEnabled())
}

func TestParseCooldown(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want time.Duration
	}{
		{"empty defaults to 60s", "", 60 * time.Second},
		{"invalid defaults to 60s", "not-a-duration", 60 * time.Second},
		{"floors below minimum", "5s", MinCooldown},
		{"passes through valid value above floor", "2m", 2 * time.Minute},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ParseCooldown(tc.in))
		})
	}
}
