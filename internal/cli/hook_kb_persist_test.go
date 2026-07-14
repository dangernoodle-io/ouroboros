package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/store"
)

// --- persistOneKbBlock: guard honors a block's own project: field --------

// TestPersistOneKbBlock_EmptyProjectFlag_BlockDeclaresOwnProject_Persisted
// pins the coupled persist-guard fix: a cwd-derived projectFlag of "" must
// not drop a block that declares its own project — kb.WriteBatch's
// per-entry project wins, and no warning is produced.
func TestPersistOneKbBlock_EmptyProjectFlag_BlockDeclaresOwnProject_Persisted(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	body := `{"type":"decision","project":"ouroboros","category":"arch","title":"Own Project Wins","content":"x"}`

	persisted, warning := persistOneKbBlock(body, db, "main", "sess1234", "stop", "sess1234", "", nil)

	assert.True(t, persisted)
	assert.Empty(t, warning)

	summaries, err := store.QueryDocuments(db, nil, []string{"ouroboros"}, nil, "", nil, 10)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "Own Project Wins", summaries[0].Title)
}

// TestPersistOneKbBlock_EmptyProjectFlag_NoOwnProject_WarnsNoWrite pins the
// genuine-ambiguity path: an empty projectFlag AND a block with no project
// of its own must warn and skip, never write.
func TestPersistOneKbBlock_EmptyProjectFlag_NoOwnProject_WarnsNoWrite(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	body := `{"type":"decision","category":"arch","title":"Orphan Block","content":"x"}`

	persisted, warning := persistOneKbBlock(body, db, "main", "sess1234", "stop", "sess1234", "", nil)

	assert.False(t, persisted)
	assert.Contains(t, warning, "no project")
	assert.Contains(t, warning, "declare")

	summaries, err := store.QueryDocuments(db, nil, nil, nil, "", nil, 10)
	require.NoError(t, err)
	assert.Empty(t, summaries)
}

// TestPersistOneKbBlock_NonEmptyProjectFlag_NoOwnProject_UsesFlag pins that
// the pre-existing cwd-derived-project path is unchanged: a non-empty
// projectFlag with a project-less block persists to that project.
func TestPersistOneKbBlock_NonEmptyProjectFlag_NoOwnProject_UsesFlag(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	body := `{"type":"decision","category":"arch","title":"Flag Project Used","content":"x"}`

	persisted, warning := persistOneKbBlock(body, db, "main", "sess1234", "stop", "sess1234", "myproj", nil)

	assert.True(t, persisted)
	assert.Empty(t, warning)

	summaries, err := store.QueryDocuments(db, nil, []string{"myproj"}, nil, "", nil, 10)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "Flag Project Used", summaries[0].Title)
}

// TestPersistOneKbBlock_EmptyProjectFlag_MixedOwnProject_WarnsNoWrite pins
// the mixed-batch behavior: a batch where one entry declares its own project
// and another does not, with an empty projectFlag, must warn-and-drop the
// WHOLE block — never a partial write of just the entry that declared its
// own project.
func TestPersistOneKbBlock_EmptyProjectFlag_MixedOwnProject_WarnsNoWrite(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	body := `[
		{"type":"decision","project":"ouroboros","category":"arch","title":"Has Own Project","content":"x"},
		{"type":"decision","category":"arch","title":"No Own Project","content":"x"}
	]`

	persisted, warning := persistOneKbBlock(body, db, "main", "sess1234", "stop", "sess1234", "", nil)

	assert.False(t, persisted)
	assert.Contains(t, warning, "no project")

	summaries, err := store.QueryDocuments(db, nil, []string{"ouroboros"}, nil, "", nil, 10)
	require.NoError(t, err)
	assert.Empty(t, summaries)

	allSummaries, err := store.QueryDocuments(db, nil, nil, nil, "", nil, 10)
	require.NoError(t, err)
	assert.Empty(t, allSummaries)
}

// --- allEntriesHaveOwnProject ---------------------------------------------

func TestAllEntriesHaveOwnProject(t *testing.T) {
	require.False(t, allEntriesHaveOwnProject(nil), "empty slice is not all-have-own-project")
}
