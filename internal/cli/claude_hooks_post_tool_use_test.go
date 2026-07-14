package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/dangernoodle-io/mcpkit/host/claudecode/hooks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// postEditGitRepo creates a fake git repo (a directory containing a .git
// entry) named "ouroboros" under a fresh t.TempDir(), and returns the path
// to a file inside it whose basename stem is stem-searchable (>=3 chars,
// no extension collision issues).
func postEditGitRepo(t *testing.T, fileName string) string {
	t.Helper()
	root := t.TempDir()
	repoDir := filepath.Join(root, "ouroboros")
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755))
	filePath := filepath.Join(repoDir, fileName)
	require.NoError(t, os.WriteFile(filePath, []byte("package p\n"), 0o644))
	return filePath
}

func editToolInput(t *testing.T, filePath string) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(map[string]string{"file_path": filePath})
	require.NoError(t, err)
	return data
}

// captureStderr redirects os.Stderr for the duration of fn, returning
// everything written to it.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	fn()

	require.NoError(t, w.Close())
	os.Stderr = orig
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(out)
}

// --- referencing KB entry: stderr hint emitted, zero Response --------------

func TestRunPostEditCheck_ReferencingKbEntry_EmitsStderrHint(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	filePath := postEditGitRepo(t, "crud.go")
	seedKbDoc(t, db, "ouroboros", "crud handler needs a transaction fix")

	p := hooks.PostToolUsePayload{
		Common:    hooks.Common{SessionID: "sess1"},
		ToolName:  "Edit",
		ToolInput: editToolInput(t, filePath),
	}

	out := captureStderr(t, func() { runPostEditCheck(p, db) })
	assert.Contains(t, out, "[ouroboros] KB refs crud.go:")
	assert.Contains(t, out, "crud handler needs a transaction fix")
	assert.Contains(t, out, "check staleness")
}

func TestHookHandlePostToolUse_EditToolName_ReferencingEntry_ZeroResponse(t *testing.T) {
	isolateHookLog(t)
	t.Setenv("HOME", t.TempDir())
	dbPath := filepath.Join(t.TempDir(), "kb.db")
	t.Setenv("PROJECT_KB_PATH", dbPath)
	t.Setenv("QM_DB_PATH", "")

	filePath := postEditGitRepo(t, "crud.go")

	p := hooks.PostToolUsePayload{
		Common:    hooks.Common{SessionID: "sess1b"},
		ToolName:  "Edit",
		ToolInput: editToolInput(t, filePath),
	}

	var resp hooks.Response
	out := captureStderr(t, func() { resp = hookHandlePostToolUse(t.Context(), nil, p) })
	assert.Equal(t, hooks.Response{}, resp, "PostToolUse must always return the zero Response — the hint goes to stderr, never AdditionalContext")
	// No seeded KB doc against a fresh DB, so no hint fires; this asserts
	// the Response contract independent of hint content.
	_ = out
}

// --- no referencing entry: silent -------------------------------------------

func TestRunPostEditCheck_NoReferencingEntry_Silent(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	filePath := postEditGitRepo(t, "crud.go")
	// No KB doc seeded at all.

	p := hooks.PostToolUsePayload{
		Common:    hooks.Common{SessionID: "sess2"},
		ToolName:  "Edit",
		ToolInput: editToolInput(t, filePath),
	}

	out := captureStderr(t, func() { runPostEditCheck(p, db) })
	assert.Empty(t, out)
}

// --- per-file MD5 cooldown ---------------------------------------------------

func TestRunPostEditCheck_PerFileCooldown_SuppressesRepeat(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	filePath := postEditGitRepo(t, "crud.go")
	seedKbDoc(t, db, "ouroboros", "crud handler needs a transaction fix")

	p := hooks.PostToolUsePayload{
		Common:    hooks.Common{SessionID: "sess3"},
		ToolName:  "Edit",
		ToolInput: editToolInput(t, filePath),
	}

	first := captureStderr(t, func() { runPostEditCheck(p, db) })
	assert.Contains(t, first, "check staleness", "first edit within the window should fire")

	second := captureStderr(t, func() { runPostEditCheck(p, db) })
	assert.Empty(t, second, "a repeat edit of the same file within the 10-minute cooldown must be suppressed")
}

func TestRunPostEditCheck_PerFileCooldown_KeyedByPath_DifferentFileNotSuppressed(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	filePath1 := postEditGitRepo(t, "crud.go")
	root := filepath.Dir(filepath.Dir(filePath1))
	filePath2 := filepath.Join(root, "ouroboros", "store.go")
	require.NoError(t, os.WriteFile(filePath2, []byte("package p\n"), 0o644))
	seedKbDoc(t, db, "ouroboros", "crud handler needs a transaction fix")
	seedKbDoc(t, db, "ouroboros", "store layer needs an index review")

	p1 := hooks.PostToolUsePayload{Common: hooks.Common{SessionID: "sess4"}, ToolName: "Edit", ToolInput: editToolInput(t, filePath1)}
	_ = captureStderr(t, func() { runPostEditCheck(p1, db) })

	p2 := hooks.PostToolUsePayload{Common: hooks.Common{SessionID: "sess4"}, ToolName: "Edit", ToolInput: editToolInput(t, filePath2)}
	out2 := captureStderr(t, func() { runPostEditCheck(p2, db) })
	assert.Contains(t, out2, "store.go", "a different file's cooldown key must be independent — no cross-file suppression")
}

// --- unhandled ToolName: no-op, proves the dispatch structure ---------------

func TestHookHandlePostToolUse_UnhandledToolName_NoOp(t *testing.T) {
	isolateHookLog(t)
	t.Setenv("HOME", t.TempDir())
	dbPath := filepath.Join(t.TempDir(), "kb.db")
	t.Setenv("PROJECT_KB_PATH", dbPath)
	t.Setenv("QM_DB_PATH", "")

	p := hooks.PostToolUsePayload{
		Common:    hooks.Common{SessionID: "sess5"},
		ToolName:  "Read",
		ToolInput: json.RawMessage(`{"file_path":"/tmp/x"}`),
	}

	var resp hooks.Response
	out := captureStderr(t, func() { resp = hookHandlePostToolUse(t.Context(), nil, p) })
	assert.Equal(t, hooks.Response{}, resp)
	assert.Empty(t, out, "a ToolName that is neither Edit-family nor Bash must not run either branch's stderr logic")
}

// --- Bash ToolName: dispatches to post-commit-nudge -------------------------

func TestHookHandlePostToolUse_BashGitCommit_EmitsPersistNudge(t *testing.T) {
	isolateHookLog(t)
	t.Setenv("HOME", t.TempDir())

	p := hooks.PostToolUsePayload{
		Common:    hooks.Common{SessionID: "sess5b"},
		ToolName:  "Bash",
		ToolInput: json.RawMessage(`{"command":"git commit -m x"}`),
	}

	var resp hooks.Response
	out := captureStderr(t, func() { resp = hookHandlePostToolUse(t.Context(), nil, p) })
	assert.Equal(t, hooks.Response{}, resp)
	assert.Contains(t, out, "[ouroboros] /persist to save decisions")
}

// --- withDB failure: fail-open silent ---------------------------------------

func TestHookHandlePostToolUse_DBOpenFailure_SilentAllow(t *testing.T) {
	isolateHookLog(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	blocker := filepath.Join(home, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	t.Setenv("PROJECT_KB_PATH", filepath.Join(blocker, "sub", "kb.db"))
	t.Setenv("QM_DB_PATH", "")

	filePath := postEditGitRepo(t, "crud.go")
	p := hooks.PostToolUsePayload{
		Common:    hooks.Common{SessionID: "sess6"},
		ToolName:  "Edit",
		ToolInput: editToolInput(t, filePath),
	}

	var resp hooks.Response
	out := captureStderr(t, func() { resp = hookHandlePostToolUse(t.Context(), nil, p) })
	assert.Equal(t, hooks.Response{}, resp, "a withDB failure must silently allow, never block")
	assert.Empty(t, out)
}

// --- no filePaths / no project: noop, no panic ------------------------------

func TestRunPostEditCheck_NoFilePath_Noop(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	p := hooks.PostToolUsePayload{
		Common:    hooks.Common{SessionID: "sess7"},
		ToolName:  "Edit",
		ToolInput: json.RawMessage(`{}`),
	}
	out := captureStderr(t, func() { runPostEditCheck(p, db) })
	assert.Empty(t, out)
}

func TestRunPostEditCheck_NoResolvableProject_Noop(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	filePath := filepath.Join(t.TempDir(), "orphan.go") // no .git ancestor
	require.NoError(t, os.WriteFile(filePath, []byte("package p\n"), 0o644))

	p := hooks.PostToolUsePayload{
		Common:    hooks.Common{SessionID: "sess8"},
		ToolName:  "Edit",
		ToolInput: editToolInput(t, filePath),
	}
	out := captureStderr(t, func() { runPostEditCheck(p, db) })
	assert.Empty(t, out)
}

// --- MultiEdit: edits array file-path extraction ----------------------------

func TestRunPostEditCheck_EmptyEditsArray_NoFallback(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	filePath := postEditGitRepo(t, "crud.go")
	seedKbDoc(t, db, "ouroboros", "crud handler needs a transaction fix")

	// Tool input has an empty edits array AND a fallback file_path.
	// Byte-faithful to JS: if edits key is PRESENT (even empty),
	// fall back to file_path is skipped. Since edits is empty, no files
	// to check, so no nudge fires and no stderr is emitted.
	input := map[string]any{
		"edits":     []map[string]string{}, // present but empty
		"file_path": filePath,              // should NOT be used
	}
	data, err := json.Marshal(input)
	require.NoError(t, err)

	p := hooks.PostToolUsePayload{
		Common:    hooks.Common{SessionID: "sess_empty_edits"},
		ToolName:  "MultiEdit",
		ToolInput: data,
	}
	out := captureStderr(t, func() { runPostEditCheck(p, db) })
	assert.Empty(t, out, "empty edits array must not fall back to file_path, so no files to check, so no stderr")
}

func TestRunPostEditCheck_MultiEdit_ChecksEachFile(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	root := t.TempDir()
	repoDir := filepath.Join(root, "ouroboros")
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755))
	file1 := filepath.Join(repoDir, "crud.go")
	file2 := filepath.Join(repoDir, "store.go")
	require.NoError(t, os.WriteFile(file1, []byte("package p\n"), 0o644))
	require.NoError(t, os.WriteFile(file2, []byte("package p\n"), 0o644))
	seedKbDoc(t, db, "ouroboros", "crud handler needs a transaction fix")
	seedKbDoc(t, db, "ouroboros", "store layer needs an index review")

	input := map[string]any{
		"edits": []map[string]string{
			{"file_path": file1},
			{"file_path": file2},
		},
	}
	data, err := json.Marshal(input)
	require.NoError(t, err)

	p := hooks.PostToolUsePayload{
		Common:    hooks.Common{SessionID: "sess9"},
		ToolName:  "MultiEdit",
		ToolInput: data,
	}
	out := captureStderr(t, func() { runPostEditCheck(p, db) })
	assert.Contains(t, out, "crud.go")
	assert.Contains(t, out, "store.go")
}

// --- stem-length / trailing-extension edge cases ----------------------------

func TestRunPostEditCheck_ShortStem_Skipped(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	filePath := postEditGitRepo(t, "id.go") // stem "id" < 3 chars
	seedKbDoc(t, db, "ouroboros", "id field naming convention")

	p := hooks.PostToolUsePayload{
		Common:    hooks.Common{SessionID: "sess10"},
		ToolName:  "Edit",
		ToolInput: editToolInput(t, filePath),
	}
	out := captureStderr(t, func() { runPostEditCheck(p, db) })
	assert.Empty(t, out)
}

// --- malformed tool_input: silent, no panic ---------------------------------

func TestRunPostEditCheck_MalformedToolInput_Silent(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	p := hooks.PostToolUsePayload{
		Common:    hooks.Common{SessionID: "sess11"},
		ToolName:  "Edit",
		ToolInput: json.RawMessage(`not json`),
	}
	out := captureStderr(t, func() { runPostEditCheck(p, db) })
	assert.Empty(t, out)
}

// --- NotebookEdit: notebook_path fallback (OU-306) --------------------------

func TestRunPostEditCheck_NotebookEdit_UsesNotebookPath(t *testing.T) {
	isolateHookLog(t)
	db := newTestDB(t)
	filePath := postEditGitRepo(t, "analysis.ipynb")
	seedKbDoc(t, db, "ouroboros", "analysis notebook needs a data refresh")

	data, err := json.Marshal(map[string]string{"notebook_path": filePath})
	require.NoError(t, err)

	p := hooks.PostToolUsePayload{
		Common:    hooks.Common{SessionID: "sess_notebook"},
		ToolName:  "NotebookEdit",
		ToolInput: data,
	}

	out := captureStderr(t, func() { runPostEditCheck(p, db) })
	assert.Contains(t, out, "[ouroboros] KB refs analysis.ipynb:")
	assert.Contains(t, out, "analysis notebook needs a data refresh")
	assert.Contains(t, out, "check staleness")
}

func TestPostEditCheckFileHash_DifferentPaths_DifferentHashes(t *testing.T) {
	h1 := postEditCheckFileHash("/a/b/crud.go")
	h2 := postEditCheckFileHash("/a/b/store.go")
	assert.NotEqual(t, h1, h2)
	assert.Len(t, h1, 8)
}
