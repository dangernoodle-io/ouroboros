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

func TestStampProject_SetsProjectOnEveryFragmentType(t *testing.T) {
	maxVal := 100.0
	frags := []Fragment{
		Tile{V: 1, Type: "tile", Label: "l", Value: "v"},
		Bar{V: 1, Type: "bar", Label: "l", Value: 1, Max: &maxVal},
		Group{V: 1, Type: "group", Cards: []Card{{Title: "t"}}},
		Note{V: 1, Type: "note", Text: "t"},
		HTML{V: 1, Type: "html", HTML: "<b>x</b>"},
	}

	stamped := StampProject(frags, "breadboard")
	require.Len(t, stamped, 5)

	tile, ok := stamped[0].(Tile)
	require.True(t, ok)
	assert.Equal(t, "breadboard", tile.Project)

	bar, ok := stamped[1].(Bar)
	require.True(t, ok)
	assert.Equal(t, "breadboard", bar.Project)

	group, ok := stamped[2].(Group)
	require.True(t, ok)
	assert.Equal(t, "breadboard", group.Project)

	note, ok := stamped[3].(Note)
	require.True(t, ok)
	assert.Equal(t, "breadboard", note.Project)

	html, ok := stamped[4].(HTML)
	require.True(t, ok)
	assert.Equal(t, "breadboard", html.Project)

	// Original slice is untouched.
	origTile, ok := frags[0].(Tile)
	require.True(t, ok)
	assert.Empty(t, origTile.Project)
}

func TestProjectField_SurvivesEmitParseRoundTrip(t *testing.T) {
	maxVal := 100.0

	tests := []struct {
		name string
		frag Fragment
	}{
		{"tile", Tile{V: 1, Type: "tile", Label: "l", Value: "v", Project: "breadboard"}},
		{"bar", Bar{V: 1, Type: "bar", Label: "l", Value: 1, Max: &maxVal, Project: "breadboard"}},
		{"group", Group{V: 1, Type: "group", Cards: []Card{{Title: "t"}}, Project: "breadboard"}},
		{"note", Note{V: 1, Type: "note", Text: "t", Project: "breadboard"}},
		{"html", HTML{V: 1, Type: "html", HTML: "<b>x</b>", Project: "breadboard"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			require.NoError(t, Emit(&buf, []Fragment{tc.frag}))

			result, err := Parse(&buf)
			require.NoError(t, err)
			require.Empty(t, result.Dropped)
			require.Len(t, result.Fragments, 1)

			switch f := result.Fragments[0].(type) {
			case Tile:
				assert.Equal(t, "breadboard", f.Project)
			case Bar:
				assert.Equal(t, "breadboard", f.Project)
			case Group:
				assert.Equal(t, "breadboard", f.Project)
			case Note:
				assert.Equal(t, "breadboard", f.Project)
			case HTML:
				assert.Equal(t, "breadboard", f.Project)
			default:
				t.Fatalf("unexpected fragment type %T", f)
			}
		})
	}
}

func TestNewTile(t *testing.T) {
	tile := NewTile("git", "branch", "main")
	assert.Equal(t, 1, tile.V)
	assert.Equal(t, "tile", tile.Type)
	assert.Equal(t, "git", tile.Section)
	assert.Equal(t, "branch", tile.Label)
	assert.Equal(t, "main", tile.Value)
}
