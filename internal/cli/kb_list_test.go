package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/store"
)

func TestRunKBListDefault(t *testing.T) {
	db := newTestDB(t)
	_, err := store.UpsertDocument(db, store.Document{
		Type: "decision", Project: "acme-corp", Title: "Use PostgreSQL", Tags: []string{"database", "backend"},
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type: "fact", Project: "acme-corp", Title: "Team size",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runKBList(&buf, db, kbListRequestFlags{Limit: 50})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "decision")
	assert.Contains(t, output, "fact")
	assert.Contains(t, output, "Use PostgreSQL")
	assert.Contains(t, output, "Team size")
}

func TestRunKBListJSON(t *testing.T) {
	db := newTestDB(t)
	result, err := store.UpsertDocument(db, store.Document{
		Type: "decision", Project: "acme-corp", Title: "Use PostgreSQL",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runKBList(&buf, db, kbListRequestFlags{Limit: 50, JSON: true})
	require.NoError(t, err)

	var summaries []store.DocumentSummary
	require.NoError(t, json.Unmarshal(buf.Bytes(), &summaries))
	require.Len(t, summaries, 1)
	assert.Equal(t, result.ID, summaries[0].ID)
}

func TestRunKBListProjectFilter(t *testing.T) {
	db := newTestDB(t)
	_, err := store.UpsertDocument(db, store.Document{
		Type: "decision", Project: "acme-corp", Title: "ACME decision",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type: "decision", Project: "other-project", Title: "OTHER decision",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runKBList(&buf, db, kbListRequestFlags{Projects: []string{"acme-corp"}, Limit: 50})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "ACME decision")
	assert.NotContains(t, output, "OTHER decision")
}

func TestRunKBListTypeFilter(t *testing.T) {
	db := newTestDB(t)
	_, err := store.UpsertDocument(db, store.Document{
		Type: "decision", Project: "acme-corp", Title: "Doc1",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type: "fact", Project: "acme-corp", Title: "Doc2",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runKBList(&buf, db, kbListRequestFlags{Type: "decision", Limit: 50})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Doc1")
	assert.NotContains(t, output, "Doc2")
}

func TestRunKBListCategoryFilter(t *testing.T) {
	db := newTestDB(t)
	_, err := store.UpsertDocument(db, store.Document{
		Type: "decision", Project: "acme-corp", Category: "architecture", Title: "Architecture decision",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type: "decision", Project: "acme-corp", Category: "process", Title: "Process decision",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runKBList(&buf, db, kbListRequestFlags{Category: "architecture", Limit: 50})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Architecture decision")
	assert.NotContains(t, output, "Process decision")
}

func TestRunKBListTagFilter(t *testing.T) {
	db := newTestDB(t)
	_, err := store.UpsertDocument(db, store.Document{
		Type: "note", Project: "acme-corp", Title: "Tagged note", Tags: []string{"urgent"},
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type: "note", Project: "acme-corp", Title: "Untagged note",
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runKBList(&buf, db, kbListRequestFlags{Tags: []string{"urgent"}, Limit: 50})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Tagged note")
	assert.NotContains(t, output, "Untagged note")
}

func TestRunKBListLimit(t *testing.T) {
	db := newTestDB(t)
	for i := 0; i < 5; i++ {
		_, err := store.UpsertDocument(db, store.Document{
			Type: "fact", Project: "acme-corp", Title: fmt.Sprintf("Fact %d", i),
		})
		require.NoError(t, err)
	}

	var buf bytes.Buffer
	err := runKBList(&buf, db, kbListRequestFlags{Limit: 2, JSON: true})
	require.NoError(t, err)

	var summaries []store.DocumentSummary
	require.NoError(t, json.Unmarshal(buf.Bytes(), &summaries))
	assert.Equal(t, 2, len(summaries))
}

// TestKBListCmdRunEWiring exercises kbListCmd's RunE closure directly (flag
// parsing + withDB wiring), proving the cobra registration works
// end-to-end -- runKBList's own behavior is already covered above.
func TestKBListCmdRunEWiring(t *testing.T) {
	db := setupEdgesFileDB(t)
	_, err := store.UpsertDocument(db, store.Document{
		Type: "note", Project: "acme-corp", Title: "Wired list note", Content: "n",
	})
	require.NoError(t, err)

	kbListProjectFlag = nil
	kbListTypeFlag = ""
	kbListCategoryFlag = ""
	kbListTagFlag = nil
	kbListLimitFlag = 50
	kbListJSONFlag = false
	defer func() {
		kbListProjectFlag = nil
		kbListTypeFlag = ""
		kbListCategoryFlag = ""
		kbListTagFlag = nil
		kbListLimitFlag = 50
		kbListJSONFlag = false
	}()

	var buf bytes.Buffer
	kbListCmd.SetOut(&buf)
	err = kbListCmd.RunE(kbListCmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Wired list note")
}
