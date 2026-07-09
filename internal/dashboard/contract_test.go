package dashboard

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmitAndParseRoundTrip(t *testing.T) {
	maxVal := 100.0

	tests := []struct {
		name string
		frag Fragment
		kind string
		sec  string
	}{
		{
			name: "tile",
			frag: Tile{V: 1, Type: "tile", Section: "git", Label: "branch", Value: "main"},
			kind: "tile",
			sec:  "git",
		},
		{
			name: "bar",
			frag: Bar{V: 1, Type: "bar", Section: "kb", Label: "progress", Value: 42, Max: &maxVal},
			kind: "bar",
			sec:  "kb",
		},
		{
			name: "group",
			frag: Group{V: 1, Type: "group", Section: "backlog", Title: "Open items", Cards: []Card{{Title: "fix bug"}}},
			kind: "group",
			sec:  "backlog",
		},
		{
			name: "note",
			frag: Note{V: 1, Type: "note", Section: "misc", Text: "hello"},
			kind: "note",
			sec:  "misc",
		},
		{
			name: "html",
			frag: HTML{V: 1, Type: "html", Section: "custom", HTML: "<b>x</b>"},
			kind: "html",
			sec:  "custom",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			require.NoError(t, Emit(&buf, []Fragment{tc.frag}))

			result, err := Parse(&buf)
			require.NoError(t, err)
			require.Empty(t, result.Dropped)
			require.Len(t, result.Fragments, 1)
			assert.Equal(t, tc.sec, Section(result.Fragments[0]))
		})
	}
}

func TestEmitMultipleFragments(t *testing.T) {
	var buf bytes.Buffer
	frags := []Fragment{
		NewTile("git", "branch", "main"),
		NewTile("git", "uncommitted", "0"),
	}
	require.NoError(t, Emit(&buf, frags))

	lines := bytes.Count(buf.Bytes(), []byte("\n"))
	assert.Equal(t, 2, lines)

	result, err := Parse(&buf)
	require.NoError(t, err)
	assert.Empty(t, result.Dropped)
	assert.Len(t, result.Fragments, 2)
}

// errWriter always fails on Write, to exercise Emit's write-error path.
type errWriter struct{}

func (errWriter) Write(_ []byte) (int, error) {
	return 0, assert.AnError
}

func TestEmit_WriteError(t *testing.T) {
	err := Emit(errWriter{}, []Fragment{NewTile("git", "branch", "main")})
	require.Error(t, err)
}

func TestFragmentKind(t *testing.T) {
	assert.Equal(t, "tile", Tile{Type: "tile"}.kind())
	assert.Equal(t, "bar", Bar{Type: "bar"}.kind())
	assert.Equal(t, "group", Group{Type: "group"}.kind())
	assert.Equal(t, "note", Note{Type: "note"}.kind())
	assert.Equal(t, "html", HTML{Type: "html"}.kind())
}

func TestNewTile(t *testing.T) {
	tile := NewTile("git", "branch", "main")
	assert.Equal(t, 1, tile.V)
	assert.Equal(t, "tile", tile.Type)
	assert.Equal(t, "git", tile.Section)
	assert.Equal(t, "branch", tile.Label)
	assert.Equal(t, "main", tile.Value)
}
