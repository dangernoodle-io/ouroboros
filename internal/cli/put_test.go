package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"

	"dangernoodle.io/ouroboros/internal/store"
)

func TestRunPutFlagsCreate(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runPutFlags(&buf, db, "acme-corp", "decision", "Use PostgreSQL", "Performance benefits", "", "", nil)
	require.NoError(t, err)

	var result putResult
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Greater(t, result.ID, int64(0))
	assert.Equal(t, "created", result.Action)
	assert.Equal(t, "Use PostgreSQL", result.Title)

	// Verify it was created in DB
	summaries, err := store.QueryDocuments(db, "decision", "acme-corp", "", "", nil, 10)
	require.NoError(t, err)
	assert.Len(t, summaries, 1)
	assert.Equal(t, "Use PostgreSQL", summaries[0].Title)
}

func TestRunPutFlagsUpdate(t *testing.T) {
	db := newTestDB(t)

	// First create
	var buf1 bytes.Buffer
	err := runPutFlags(&buf1, db, "acme-corp", "decision", "Use PostgreSQL", "Performance", "", "", nil)
	require.NoError(t, err)
	var result1 putResult
	require.NoError(t, json.Unmarshal(buf1.Bytes(), &result1))

	// Second update (same type/project/title)
	var buf2 bytes.Buffer
	err = runPutFlags(&buf2, db, "acme-corp", "decision", "Use PostgreSQL", "Better performance", "", "", nil)
	require.NoError(t, err)
	var result2 putResult
	require.NoError(t, json.Unmarshal(buf2.Bytes(), &result2))

	assert.Equal(t, result1.ID, result2.ID)
	assert.Equal(t, "updated", result2.Action)
}

func TestRunPutFlagsValidationFailure(t *testing.T) {
	db := newTestDB(t)

	// Missing content
	var buf bytes.Buffer
	err := runPutFlags(&buf, db, "acme-corp", "decision", "Use PostgreSQL", "", "", "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "content is required")
}

func TestRunPutFlagsTypeEnum(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runPutFlags(&buf, db, "acme-corp", "bogus", "Title", "Content", "", "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid type")
}

func TestRunPutFlagsContentCap(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runPutFlags(&buf, db, "acme-corp", "decision", "Title", strings.Repeat("x", 501), "", "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "content exceeds")
}

func TestRunPutStdinArray(t *testing.T) {
	db := newTestDB(t)

	input := `[{"type":"note","project":"tk-test","title":"Note 1","content":"Content 1"}]`
	var buf bytes.Buffer
	err := runPutStdin(&buf, strings.NewReader(input), db, "")
	require.NoError(t, err)

	var results []putResult
	require.NoError(t, json.Unmarshal(buf.Bytes(), &results))
	assert.Len(t, results, 1)
	assert.Equal(t, "created", results[0].Action)
	assert.Equal(t, "Note 1", results[0].Title)
}

func TestRunPutStdinDocuments(t *testing.T) {
	db := newTestDB(t)

	input := `{"documents":[{"type":"fact","project":"acme-corp","title":"DB Type","content":"PostgreSQL"}]}`
	var buf bytes.Buffer
	err := runPutStdin(&buf, strings.NewReader(input), db, "")
	require.NoError(t, err)

	var results []putResult
	require.NoError(t, json.Unmarshal(buf.Bytes(), &results))
	assert.Len(t, results, 1)
	assert.Equal(t, "DB Type", results[0].Title)
}

func TestRunPutStdinEmptyArray(t *testing.T) {
	db := newTestDB(t)

	input := `[]`
	var buf bytes.Buffer
	err := runPutStdin(&buf, strings.NewReader(input), db, "")
	require.NoError(t, err)
	assert.Equal(t, "[]\n", buf.String())
}

func TestRunPutStdinProjectFallback(t *testing.T) {
	db := newTestDB(t)

	input := `[{"type":"note","title":"Note 1","content":"Content 1"}]`
	var buf bytes.Buffer
	err := runPutStdin(&buf, strings.NewReader(input), db, "default-project")
	require.NoError(t, err)

	var results []putResult
	require.NoError(t, json.Unmarshal(buf.Bytes(), &results))
	assert.Len(t, results, 1)

	// Verify it was created with fallback project
	summaries, err := store.QueryDocuments(db, "note", "default-project", "", "", nil, 10)
	require.NoError(t, err)
	assert.Len(t, summaries, 1)
}

func TestRunPutStdinValidationFailureNoPartialWrites(t *testing.T) {
	db := newTestDB(t)

	// Entry 0 valid, entry 1 invalid (missing content)
	input := `[{"type":"note","project":"tk-test","title":"Valid","content":"Content"},{"type":"note","project":"tk-test","title":"Invalid"}]`
	var buf bytes.Buffer
	err := runPutStdin(&buf, strings.NewReader(input), db, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")

	// Verify nothing was written
	summaries, err := store.QueryDocuments(db, "", "", "", "", nil, 10)
	require.NoError(t, err)
	assert.Len(t, summaries, 0)
}

func TestRunPutStdinInvalidJSON(t *testing.T) {
	db := newTestDB(t)

	input := `not json`
	var buf bytes.Buffer
	err := runPutStdin(&buf, strings.NewReader(input), db, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON")
}

func TestRunPutStdinEmptyInput(t *testing.T) {
	db := newTestDB(t)

	input := ``
	var buf bytes.Buffer
	err := runPutStdin(&buf, strings.NewReader(input), db, "")
	require.NoError(t, err)
	assert.Equal(t, "[]\n", buf.String())
}

func TestRunPutFlagsWithTags(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runPutFlags(&buf, db, "acme-corp", "decision", "Use PostgreSQL", "Performance", "", "", []string{"database", "infrastructure"})
	require.NoError(t, err)

	var result putResult
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Greater(t, result.ID, int64(0))

	// Verify tags were stored
	doc, err := store.GetDocument(db, result.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"database", "infrastructure"}, doc.Tags)
}

func TestRunPutStdinMultipleEntries(t *testing.T) {
	db := newTestDB(t)

	input := `[
		{"type":"decision","project":"acme-corp","title":"Decision 1","content":"Content 1"},
		{"type":"fact","project":"acme-corp","title":"Fact 1","content":"Content 2"},
		{"type":"note","project":"acme-corp","title":"Note 1","content":"Content 3"}
	]`
	var buf bytes.Buffer
	err := runPutStdin(&buf, strings.NewReader(input), db, "")
	require.NoError(t, err)

	var results []putResult
	require.NoError(t, json.Unmarshal(buf.Bytes(), &results))
	assert.Len(t, results, 3)

	// Verify all three were created
	summaries, err := store.QueryDocuments(db, "", "acme-corp", "", "", nil, 10)
	require.NoError(t, err)
	assert.Len(t, summaries, 3)
}

func TestRunPutFlagsRequiredFlags(t *testing.T) {
	db := newTestDB(t)

	tests := []struct {
		name        string
		project     string
		docType     string
		title       string
		content     string
		expectedErr string
	}{
		{
			name:        "missing type",
			project:     "acme-corp",
			docType:     "",
			title:       "Title",
			content:     "Content",
			expectedErr: "type is required",
		},
		{
			name:        "missing project",
			project:     "",
			docType:     "decision",
			title:       "Title",
			content:     "Content",
			expectedErr: "project is required",
		},
		{
			name:        "missing title",
			project:     "acme-corp",
			docType:     "decision",
			title:       "",
			content:     "Content",
			expectedErr: "title is required",
		},
		{
			name:        "missing content",
			project:     "acme-corp",
			docType:     "decision",
			title:       "Title",
			content:     "",
			expectedErr: "content is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := runPutFlags(&buf, db, tt.project, tt.docType, tt.title, tt.content, "", "", nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}
