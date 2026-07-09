package dashboard

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEmptyInput(t *testing.T) {
	result, err := Parse(strings.NewReader(""))
	require.NoError(t, err)
	assert.Empty(t, result.Fragments)
	assert.Empty(t, result.Dropped)
	assert.NotNil(t, result.Fragments)
	assert.NotNil(t, result.Dropped)
}

func TestParseBlankLinesSkipped(t *testing.T) {
	input := "\n   \n" + `{"v":1,"type":"tile","label":"branch","value":"main"}` + "\n\n"
	result, err := Parse(strings.NewReader(input))
	require.NoError(t, err)
	assert.Len(t, result.Fragments, 1)
	assert.Empty(t, result.Dropped)
}

func TestParseDropsBadLinesButKeepsGood(t *testing.T) {
	goodLine := `{"v":1,"type":"tile","label":"branch","value":"main"}`

	tests := []struct {
		name    string
		badLine string
		reason  string
	}{
		{
			name:    "malformed json",
			badLine: `{not json`,
			reason:  "malformed json",
		},
		{
			name:    "unsupported version",
			badLine: `{"v":2,"type":"tile","label":"x","value":"y"}`,
			reason:  "unsupported version",
		},
		{
			name:    "tile missing value",
			badLine: `{"v":1,"type":"tile","label":"x"}`,
			reason:  "tile",
		},
		{
			name:    "tile missing label",
			badLine: `{"v":1,"type":"tile","value":"y"}`,
			reason:  "tile",
		},
		{
			name:    "bar negative value",
			badLine: `{"v":1,"type":"bar","label":"x","value":-1}`,
			reason:  "bar",
		},
		{
			name:    "bar max not positive",
			badLine: `{"v":1,"type":"bar","label":"x","value":1,"max":0}`,
			reason:  "bar",
		},
		{
			name:    "bar missing label",
			badLine: `{"v":1,"type":"bar","value":1}`,
			reason:  "bar",
		},
		{
			name:    "group no cards",
			badLine: `{"v":1,"type":"group","cards":[]}`,
			reason:  "group",
		},
		{
			name:    "group card missing title",
			badLine: `{"v":1,"type":"group","cards":[{"desc":"x"}]}`,
			reason:  "group",
		},
		{
			name:    "note empty text",
			badLine: `{"v":1,"type":"note","text":""}`,
			reason:  "note",
		},
		{
			name:    "html empty",
			badLine: `{"v":1,"type":"html","html":""}`,
			reason:  "html",
		},
		{
			name:    "unknown type",
			badLine: `{"v":1,"type":"widget"}`,
			reason:  "unknown fragment type",
		},
		{
			name:    "tile type mismatch on full unmarshal",
			badLine: `{"v":1,"type":"tile","label":"x","value":123}`,
			reason:  "malformed json",
		},
		{
			name:    "bar type mismatch on full unmarshal",
			badLine: `{"v":1,"type":"bar","label":"x","value":"not-a-number"}`,
			reason:  "malformed json",
		},
		{
			name:    "group type mismatch on full unmarshal",
			badLine: `{"v":1,"type":"group","cards":"not-an-array"}`,
			reason:  "malformed json",
		},
		{
			name:    "note type mismatch on full unmarshal",
			badLine: `{"v":1,"type":"note","text":123}`,
			reason:  "malformed json",
		},
		{
			name:    "html type mismatch on full unmarshal",
			badLine: `{"v":1,"type":"html","html":123}`,
			reason:  "malformed json",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.badLine + "\n" + goodLine + "\n"
			result, err := Parse(strings.NewReader(input))
			require.NoError(t, err)

			require.Len(t, result.Dropped, 1)
			assert.Equal(t, 1, result.Dropped[0].Line)
			assert.Equal(t, tc.badLine, result.Dropped[0].Raw)
			assert.Contains(t, result.Dropped[0].Err, tc.reason)

			require.Len(t, result.Fragments, 1)
			tile, ok := result.Fragments[0].(Tile)
			require.True(t, ok)
			assert.Equal(t, "main", tile.Value)
		})
	}
}

func TestParseDropLineNumbers(t *testing.T) {
	input := strings.Join([]string{
		`{"v":1,"type":"tile","label":"a","value":"1"}`,
		`{bad`,
		`{"v":1,"type":"tile","label":"b","value":"2"}`,
		`{"v":2,"type":"tile","label":"c","value":"3"}`,
	}, "\n") + "\n"

	result, err := Parse(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Fragments, 2)
	require.Len(t, result.Dropped, 2)
	assert.Equal(t, 2, result.Dropped[0].Line)
	assert.Equal(t, 4, result.Dropped[1].Line)
}

func TestParseOversizedLineDroppedNotAborted(t *testing.T) {
	goodFirst := `{"v":1,"type":"tile","label":"a","value":"1"}`
	goodLast := `{"v":1,"type":"tile","label":"b","value":"2"}`

	// A line whose JSON payload alone exceeds maxLineBytes; still valid
	// JSON shape (a big note text), so the only reason it's dropped is
	// its size, exercising the oversized-line path specifically.
	huge := `{"v":1,"type":"note","text":"` + strings.Repeat("x", maxLineBytes+100) + `"}`

	input := strings.Join([]string{goodFirst, huge, goodLast}, "\n") + "\n"

	result, err := Parse(strings.NewReader(input))
	require.NoError(t, err)

	require.Len(t, result.Fragments, 2)
	tile0, ok := result.Fragments[0].(Tile)
	require.True(t, ok)
	assert.Equal(t, "1", tile0.Value)
	tile1, ok := result.Fragments[1].(Tile)
	require.True(t, ok)
	assert.Equal(t, "2", tile1.Value)

	require.Len(t, result.Dropped, 1)
	assert.Equal(t, 2, result.Dropped[0].Line)
	assert.Equal(t, "line too long", result.Dropped[0].Err)
	assert.LessOrEqual(t, len(result.Dropped[0].Raw), maxDropRawBytes+len("…"))
}

func TestParseOversizedLineNoTrailingNewline(t *testing.T) {
	// Oversized line is also the final line with no trailing "\n" -- exercises
	// the oversized+EOF break path.
	huge := `{"v":1,"type":"note","text":"` + strings.Repeat("x", maxLineBytes+100) + `"}`

	result, err := Parse(strings.NewReader(huge))
	require.NoError(t, err)
	assert.Empty(t, result.Fragments)
	require.Len(t, result.Dropped, 1)
	assert.Equal(t, 1, result.Dropped[0].Line)
	assert.Equal(t, "line too long", result.Dropped[0].Err)
}

func TestParseBlankFinalLineNoTrailingNewline(t *testing.T) {
	// A trailing blank "line" (whitespace only) with no terminating "\n" --
	// exercises the blank+EOF break path.
	input := `{"v":1,"type":"tile","label":"a","value":"1"}` + "\n   "

	result, err := Parse(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Fragments, 1)
	assert.Empty(t, result.Dropped)
}

func TestTruncateRaw(t *testing.T) {
	short := "short line"
	assert.Equal(t, short, truncateRaw(short))

	long := strings.Repeat("y", maxDropRawBytes+50)
	truncated := truncateRaw(long)
	assert.Equal(t, maxDropRawBytes+len("…"), len(truncated))
}

func TestTruncateRaw_RuneSafeBoundary(t *testing.T) {
	// A multibyte rune ('€' is 3 bytes in UTF-8) straddling the
	// maxDropRawBytes boundary: naive s[:maxDropRawBytes] would slice mid-rune.
	padded := strings.Repeat("x", maxDropRawBytes-1) + "€" + strings.Repeat("z", 50)
	require.Greater(t, len(padded), maxDropRawBytes)

	truncated := truncateRaw(padded)
	assert.True(t, utf8.ValidString(truncated), "truncated raw must be valid UTF-8")
	assert.True(t, strings.HasSuffix(truncated, "…"))
}

// errReader always fails on Read, to exercise Parse's scanner-error path.
type errReader struct{}

func (errReader) Read(_ []byte) (int, error) {
	return 0, assert.AnError
}

func TestParseReaderError(t *testing.T) {
	_, err := Parse(errReader{})
	require.Error(t, err)
}
