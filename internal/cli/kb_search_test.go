package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/store"
)

func TestRunKBSearchBasic(t *testing.T) {
	db := newTestDB(t)
	_, err := store.UpsertDocument(db, store.Document{
		Type: "decision", Project: "acme-corp", Title: "Database choice",
		Content: "We chose PostgreSQL for its performance",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type: "fact", Project: "acme-corp", Title: "Team size", Content: "5 engineers",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runKBSearch(&buf, db, "PostgreSQL", "", "", "", 50, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Database choice")
	assert.NotContains(t, output, "Team size")
}

func TestRunKBSearchJSON(t *testing.T) {
	db := newTestDB(t)
	_, err := store.UpsertDocument(db, store.Document{
		Type: "decision", Project: "acme-corp", Title: "Database choice", Content: "PostgreSQL wins",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runKBSearch(&buf, db, "PostgreSQL", "", "", "", 50, true)
	require.NoError(t, err)

	var summaries []store.DocumentSummary
	require.NoError(t, json.Unmarshal(buf.Bytes(), &summaries))
	require.Len(t, summaries, 1)
	assert.Equal(t, "Database choice", summaries[0].Title)
}

func TestRunKBSearchProjectFilter(t *testing.T) {
	db := newTestDB(t)
	_, err := store.UpsertDocument(db, store.Document{
		Type: "note", Project: "acme-corp", Title: "ACME note", Content: "widget details",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type: "note", Project: "other-project", Title: "OTHER note", Content: "widget details",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runKBSearch(&buf, db, "widget", "acme-corp", "", "", 50, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "ACME note")
	assert.NotContains(t, output, "OTHER note")
}

func TestRunKBSearchTypeAndCategoryFilter(t *testing.T) {
	db := newTestDB(t)
	_, err := store.UpsertDocument(db, store.Document{
		Type: "decision", Project: "acme-corp", Category: "arch", Title: "Decision widget",
		Content: "widget stuff",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type: "note", Project: "acme-corp", Category: "process", Title: "Note widget",
		Content: "widget stuff",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runKBSearch(&buf, db, "widget", "", "decision", "arch", 50, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Decision widget")
	assert.NotContains(t, output, "Note widget")
}

func TestRunKBSearchEmptyTextError(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runKBSearch(&buf, db, "", "", "", "", 50, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "text is required")
}

// TestKBSearchCmdRunEWiring exercises kbSearchCmd's RunE closure directly
// (flag parsing + withDB wiring), proving the cobra registration works
// end-to-end -- runKBSearch's own behavior is already covered above.
func TestKBSearchCmdRunEWiring(t *testing.T) {
	db := setupEdgesFileDB(t)
	_, err := store.UpsertDocument(db, store.Document{
		Type: "note", Project: "acme-corp", Title: "Wired search gizmo", Content: "gizmo stuff",
	})
	require.NoError(t, err)

	kbSearchProjectFlag = ""
	kbSearchTypeFlag = ""
	kbSearchCategoryFlag = ""
	kbSearchLimitFlag = 50
	kbSearchJSONFlag = false
	defer func() {
		kbSearchProjectFlag = ""
		kbSearchTypeFlag = ""
		kbSearchCategoryFlag = ""
		kbSearchLimitFlag = 50
		kbSearchJSONFlag = false
	}()

	var buf bytes.Buffer
	kbSearchCmd.SetOut(&buf)
	err = kbSearchCmd.RunE(kbSearchCmd, []string{"gizmo"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Wired search gizmo")
}
