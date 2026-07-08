package cli

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"

	"dangernoodle.io/ouroboros/internal/roadmap"
	"dangernoodle.io/ouroboros/internal/store"
)

// These tests exercise the cobra RunE closures directly (flag parsing +
// validation + withDB wiring) rather than the run* helper functions the
// other tests in roadmap_test.go call, since those closures never execute
// otherwise: cobra never dispatches through rootCmd in the test binary.

func resetRoadmapAddVars() {
	roadmapAddSection = ""
	roadmapAddTitle = ""
	roadmapAddBody = ""
	roadmapAddComponent = ""
	roadmapAddWhy = ""
	roadmapAddResume = ""
	roadmapAddKB = nil
	roadmapAddTicket = nil
	roadmapAddBlockedBy = nil
}

func resetRoadmapUpdateVars() {
	roadmapUpdateTitle = ""
	roadmapUpdateBody = ""
	roadmapUpdateComponent = ""
	roadmapUpdateWhy = ""
	roadmapUpdateResume = ""
	roadmapUpdateSection = ""
	roadmapUpdateKB = nil
	roadmapUpdateTicket = nil
	roadmapUpdateBlockedBy = nil
	for _, name := range []string{"title", "body", "component", "why", "resume", "section", "kb", "ticket", "blocked-by"} {
		if f := roadmapUpdateCmd.Flags().Lookup(name); f != nil {
			f.Changed = false
		}
	}
}

// ── add ──────────────────────────────────────────────────────────────────────

func TestRoadmapAddCmd_MissingSection(t *testing.T) {
	resetRoadmapAddVars()
	roadmapAddTitle = "x"

	err := roadmapAddCmd.RunE(roadmapAddCmd, []string{"acme-corp"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--section is required")
}

func TestRoadmapAddCmd_MissingTitle(t *testing.T) {
	resetRoadmapAddVars()
	roadmapAddSection = "now"

	err := roadmapAddCmd.RunE(roadmapAddCmd, []string{"acme-corp"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--title is required")
}

func TestRoadmapAddCmd_InvalidSection(t *testing.T) {
	resetRoadmapAddVars()
	roadmapAddSection = "bogus"
	roadmapAddTitle = "x"

	err := roadmapAddCmd.RunE(roadmapAddCmd, []string{"acme-corp"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid section")
}

func TestRoadmapAddCmd_InvalidKB(t *testing.T) {
	resetRoadmapAddVars()
	roadmapAddSection = "now"
	roadmapAddTitle = "x"
	roadmapAddKB = []string{"not-an-int"}

	err := roadmapAddCmd.RunE(roadmapAddCmd, []string{"acme-corp"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid id")
}

func TestRoadmapAddCmd_InvalidBlockedBy(t *testing.T) {
	resetRoadmapAddVars()
	roadmapAddSection = "now"
	roadmapAddTitle = "x"
	roadmapAddBlockedBy = []string{"missing-colon"}

	err := roadmapAddCmd.RunE(roadmapAddCmd, []string{"acme-corp"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --blocked-by")
}

func TestRoadmapAddCmd_Success(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PROJECT_KB_PATH", filepath.Join(dir, "roadmap.db"))

	resetRoadmapAddVars()
	roadmapAddSection = "now"
	roadmapAddTitle = "ship widget"
	roadmapAddBody = "build it"
	roadmapAddComponent = "widget"
	roadmapAddWhy = "n/a"
	roadmapAddResume = "n/a"
	roadmapAddKB = []string{"1", "2"}
	roadmapAddTicket = []string{"tk-test-000"}
	roadmapAddBlockedBy = []string{"other-proj:PR-1:waiting"}

	roadmapAddCmd.SetOut(&bytes.Buffer{})
	err := roadmapAddCmd.RunE(roadmapAddCmd, []string{"acme-corp"})
	require.NoError(t, err)
}

// ── update ───────────────────────────────────────────────────────────────────

func seedRoadmapDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "roadmap.db")
	t.Setenv("PROJECT_KB_PATH", path)

	seedDB, err := store.InitDB(path)
	require.NoError(t, err)
	require.NoError(t, runRoadmapAdd(&bytes.Buffer{}, seedDB, "acme-corp", roadmap.SectionNow, roadmap.Item{Title: "seed"}))
	require.NoError(t, seedDB.Close())
}

func TestRoadmapUpdateCmd_InvalidID(t *testing.T) {
	resetRoadmapUpdateVars()

	err := roadmapUpdateCmd.RunE(roadmapUpdateCmd, []string{"acme-corp", "abc"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid id")
}

func TestRoadmapUpdateCmd_InvalidKB(t *testing.T) {
	seedRoadmapDB(t)
	resetRoadmapUpdateVars()
	roadmapUpdateKB = []string{"not-an-int"}
	roadmapUpdateCmd.Flags().Lookup("kb").Changed = true

	err := roadmapUpdateCmd.RunE(roadmapUpdateCmd, []string{"acme-corp", "1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid id")
}

func TestRoadmapUpdateCmd_InvalidBlockedBy(t *testing.T) {
	seedRoadmapDB(t)
	resetRoadmapUpdateVars()
	roadmapUpdateBlockedBy = []string{"missing-colon"}
	roadmapUpdateCmd.Flags().Lookup("blocked-by").Changed = true

	err := roadmapUpdateCmd.RunE(roadmapUpdateCmd, []string{"acme-corp", "1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --blocked-by")
}

func TestRoadmapUpdateCmd_InvalidSection(t *testing.T) {
	seedRoadmapDB(t)
	resetRoadmapUpdateVars()
	roadmapUpdateSection = "bogus"
	roadmapUpdateCmd.Flags().Lookup("section").Changed = true

	err := roadmapUpdateCmd.RunE(roadmapUpdateCmd, []string{"acme-corp", "1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid section")
}

func TestRoadmapUpdateCmd_Success(t *testing.T) {
	seedRoadmapDB(t)
	resetRoadmapUpdateVars()

	roadmapUpdateTitle = "new title"
	roadmapUpdateBody = "new body"
	roadmapUpdateComponent = "widget"
	roadmapUpdateWhy = "why"
	roadmapUpdateResume = "resume"
	roadmapUpdateKB = []string{"5"}
	roadmapUpdateTicket = []string{"tk-test-001"}
	roadmapUpdateBlockedBy = []string{"other-proj:PR-2"}
	roadmapUpdateSection = "next"

	for _, name := range []string{"title", "body", "component", "why", "resume", "kb", "ticket", "blocked-by", "section"} {
		roadmapUpdateCmd.Flags().Lookup(name).Changed = true
	}

	roadmapUpdateCmd.SetOut(&bytes.Buffer{})
	err := roadmapUpdateCmd.RunE(roadmapUpdateCmd, []string{"acme-corp", "1"})
	require.NoError(t, err)
}

// ── move ─────────────────────────────────────────────────────────────────────

func TestRoadmapMoveCmd_InvalidID(t *testing.T) {
	roadmapMoveTo = ""

	err := roadmapMoveCmd.RunE(roadmapMoveCmd, []string{"acme-corp", "abc"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid id")
}

func TestRoadmapMoveCmd_MissingTo(t *testing.T) {
	roadmapMoveTo = ""

	err := roadmapMoveCmd.RunE(roadmapMoveCmd, []string{"acme-corp", "1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--to is required")
}

func TestRoadmapMoveCmd_InvalidTo(t *testing.T) {
	roadmapMoveTo = "bogus"

	err := roadmapMoveCmd.RunE(roadmapMoveCmd, []string{"acme-corp", "1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid section")
}

func TestRoadmapMoveCmd_Success(t *testing.T) {
	seedRoadmapDB(t)
	roadmapMoveTo = "parked"

	roadmapMoveCmd.SetOut(&bytes.Buffer{})
	err := roadmapMoveCmd.RunE(roadmapMoveCmd, []string{"acme-corp", "1"})
	require.NoError(t, err)
}

// ── done ─────────────────────────────────────────────────────────────────────

func TestRoadmapDoneCmd_InvalidID(t *testing.T) {
	err := roadmapDoneCmd.RunE(roadmapDoneCmd, []string{"acme-corp", "abc"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid id")
}

func TestRoadmapDoneCmd_Success(t *testing.T) {
	seedRoadmapDB(t)

	roadmapDoneCmd.SetOut(&bytes.Buffer{})
	err := roadmapDoneCmd.RunE(roadmapDoneCmd, []string{"acme-corp", "1"})
	require.NoError(t, err)
}

// ── remove ───────────────────────────────────────────────────────────────────

func TestRoadmapRemoveCmd_InvalidID(t *testing.T) {
	err := roadmapRemoveCmd.RunE(roadmapRemoveCmd, []string{"acme-corp", "abc"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid id")
}

func TestRoadmapRemoveCmd_Success(t *testing.T) {
	seedRoadmapDB(t)

	roadmapRemoveCmd.SetOut(&bytes.Buffer{})
	err := roadmapRemoveCmd.RunE(roadmapRemoveCmd, []string{"acme-corp", "1"})
	require.NoError(t, err)
}

// ── show ─────────────────────────────────────────────────────────────────────

func TestRoadmapShowCmd_Success(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PROJECT_KB_PATH", filepath.Join(dir, "roadmap.db"))

	var buf bytes.Buffer
	roadmapShowCmd.SetOut(&buf)
	err := roadmapShowCmd.RunE(roadmapShowCmd, []string{"acme-corp"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "# Roadmap")
}

// ── parseSection valid path + runRoadmapAdd Mutate error ────────────────────

func TestParseSectionValid(t *testing.T) {
	section, err := parseSection("now")
	require.NoError(t, err)
	assert.Equal(t, roadmap.SectionNow, section)
}

func TestRunRoadmapAddMutateError(t *testing.T) {
	db := newTestDB(t)

	var buf bytes.Buffer
	err := runRoadmapAdd(&buf, db, "acme-corp", roadmap.Section("bogus"), roadmap.Item{Title: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "roadmap:")
}

// ── runRoadmapShow Load error ────────────────────────────────────────────────

func TestRunRoadmapShowLoadError(t *testing.T) {
	db := newTestDB(t)
	_, err := db.Exec("DROP TABLE documents")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = runRoadmapShow(&buf, db, "acme-corp")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "roadmap:")
}
