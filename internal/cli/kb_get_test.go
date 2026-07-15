package cli

import (
	"bytes"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/edges"
	"dangernoodle.io/ouroboros/internal/store"
)

func TestRunKBGetSingle(t *testing.T) {
	db := newTestDB(t)
	result, err := store.UpsertDocument(db, store.Document{
		Type: "decision", Project: "acme-corp", Category: "arch", Title: "Use PostgreSQL",
		Content: "For performance", Tags: []string{"db", "backend"}, Notes: "Approved",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	idStr := formatID(result.ID)
	err = runKBGet(&buf, db, []string{idStr}, false, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "[decision]")
	assert.Contains(t, output, "Use PostgreSQL")
	assert.Contains(t, output, "Content:")
	// Notes are omitted without --verbose.
	assert.NotContains(t, output, "Approved")
}

func TestRunKBGetVerboseNoEdges(t *testing.T) {
	db := newTestDB(t)
	result, err := store.UpsertDocument(db, store.Document{
		Type: "note", Project: "acme-corp", Title: "Solo note", Content: "n", Notes: "Extra context",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runKBGet(&buf, db, []string{formatID(result.ID)}, true, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Extra context")
	assert.NotContains(t, output, "Edges:")
}

func TestRunKBGetVerboseIncludesNotesAndEdges(t *testing.T) {
	db := newTestDB(t)
	result, err := store.UpsertDocument(db, store.Document{
		Type: "decision", Project: "acme-corp", Title: "Use PostgreSQL",
		Content: "For performance", Notes: "Approved by team",
	})
	require.NoError(t, err)
	other, err := store.UpsertDocument(db, store.Document{
		Type: "note", Project: "acme-corp", Title: "Related note", Content: "n",
	})
	require.NoError(t, err)

	_, err = edges.Link(db, edges.TypeKB, formatID(result.ID), "relates", edges.TypeKB, formatID(other.ID), 0)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runKBGet(&buf, db, []string{formatID(result.ID)}, true, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Approved by team")
	assert.Contains(t, output, "Edges:")
}

func TestRunKBGetJSON(t *testing.T) {
	db := newTestDB(t)
	result, err := store.UpsertDocument(db, store.Document{
		Type: "fact", Project: "acme-corp", Title: "Team size", Content: "5",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runKBGet(&buf, db, []string{formatID(result.ID)}, false, true)
	require.NoError(t, err)

	var docs []store.Document
	require.NoError(t, json.Unmarshal(buf.Bytes(), &docs))
	require.Len(t, docs, 1)
	assert.Equal(t, "Team size", docs[0].Title)
}

func TestRunKBGetMultiple(t *testing.T) {
	db := newTestDB(t)
	a, err := store.UpsertDocument(db, store.Document{Type: "note", Project: "acme-corp", Title: "A", Content: "a"})
	require.NoError(t, err)
	b, err := store.UpsertDocument(db, store.Document{Type: "note", Project: "acme-corp", Title: "B", Content: "b"})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runKBGet(&buf, db, []string{formatID(a.ID), formatID(b.ID)}, false, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "A")
	assert.Contains(t, output, "B")
}

func TestRunKBGetMissingIDErrors(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runKBGet(&buf, db, []string{"999999"}, false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found: 999999")
}

func TestRunKBGetMixedValidAndMissingID(t *testing.T) {
	db := newTestDB(t)
	result, err := store.UpsertDocument(db, store.Document{
		Type: "fact", Project: "acme-corp", Title: "Team size", Content: "5",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runKBGet(&buf, db, []string{formatID(result.ID), "999999"}, false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found: 999999")

	// Only the found doc is rendered to out; the missing-id diagnostic
	// travels solely via the returned error, never appended to out.
	output := buf.String()
	assert.Contains(t, output, "Team size")
	assert.NotContains(t, output, "not found")
}

// TestRunKBGetJSONMissing guards against corrupting the JSON stream: a
// missing id alongside a valid one must still leave out a clean, parseable
// JSON array (the found doc), with the "not found" diagnostic carried only
// by the returned error — never appended to out.
func TestRunKBGetJSONMissing(t *testing.T) {
	db := newTestDB(t)
	result, err := store.UpsertDocument(db, store.Document{
		Type: "fact", Project: "acme-corp", Title: "Team size", Content: "5",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runKBGet(&buf, db, []string{formatID(result.ID), "999999"}, false, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found: 999999")

	var docs []store.Document
	require.NoError(t, json.Unmarshal(buf.Bytes(), &docs))
	require.Len(t, docs, 1)
	assert.Equal(t, "Team size", docs[0].Title)
}

func TestRunKBGetInvalidID(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runKBGet(&buf, db, []string{"not-a-number"}, false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid id "not-a-number"`)
}

func formatID(id int64) string {
	return strconv.FormatInt(id, 10)
}

// TestKBGetCmdRunEWiring exercises kbGetCmd's RunE closure directly (flag
// parsing + withDB wiring), proving the cobra registration works
// end-to-end -- runKBGet's own behavior is already covered above.
func TestKBGetCmdRunEWiring(t *testing.T) {
	db := setupEdgesFileDB(t)
	result, err := store.UpsertDocument(db, store.Document{
		Type: "note", Project: "acme-corp", Title: "Wired note", Content: "n",
	})
	require.NoError(t, err)

	kbGetVerboseFlag = false
	kbGetJSONFlag = false
	defer func() {
		kbGetVerboseFlag = false
		kbGetJSONFlag = false
	}()

	var buf bytes.Buffer
	kbGetCmd.SetOut(&buf)
	err = kbGetCmd.RunE(kbGetCmd, []string{formatID(result.ID)})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Wired note")
}
