package cli

import (
	"context"
	"crypto/md5" //nolint:gosec // non-cryptographic cache key, faithful port of post-edit-check.js's cooldown hash
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/dangernoodle-io/shesha/host/claudecode/hooks"

	"dangernoodle.io/ouroboros/internal/store"
)

// postEditCheckCooldownMS is the per-file cooldown window (10 minutes).
// Faithful port of post-edit-check.js's COOLDOWN_MS.
const postEditCheckCooldownMS = 600000

// postEditCheckMaxRows caps the KB rows returned per edited file. Faithful
// port of post-edit-check.js's queryKb({ limit: 5 }) call.
const postEditCheckMaxRows = 5

// postEditCheckMinStemLen is the minimum basename-stem length worth a KB
// search. Faithful port of post-edit-check.js's `stem.length < 3` guard —
// short stems (e.g. "id", "a") are too generic to search meaningfully.
const postEditCheckMinStemLen = 3

// trailingExtRe matches a trailing ".ext" suffix (at least one non-dot
// character after the final dot) for stripping a basename down to its stem.
// Faithful port of post-edit-check.js's `/\.[^.]+$/` regex — a basename
// ending in a bare dot with nothing after it (e.g. "file.") is NOT stripped,
// matching the JS regex's requirement of >=1 char after the dot.
var trailingExtRe = regexp.MustCompile(`\.[^.]+$`)

// editToolNames is the Edit family of tool names post-edit-check.js reacts
// to, matching hooks.json's PostToolUse Edit-family matcher.
var editToolNames = map[string]bool{
	"Edit":         true,
	"Write":        true,
	"MultiEdit":    true,
	"NotebookEdit": true,
}

// hookHandlePostToolUse is the single PostToolUse-event registry handler; it
// dispatches on payload.ToolName. Two branches today: the Edit family
// (post-edit-check.js's per-file KB-staleness nudge on Edit/Write/MultiEdit/
// NotebookEdit, OU-275) opens its own scoped DB via withDB; Bash
// (post-commit-nudge.js's post-git-commit /persist nudge, OU-277) does no KB
// read at all, so it runs with no DB access whatsoever. Any other ToolName
// is a silent no-op (zero Response).
func hookHandlePostToolUse(_ context.Context, _ io.Reader, p hooks.PostToolUsePayload) hooks.Response {
	switch {
	case editToolNames[p.ToolName]:
		if err := withDB(func(db *sql.DB) error {
			runPostEditCheck(p, db)
			return nil
		}); err != nil {
			logHookEvent(map[string]any{"hook": "post_edit_check", "kind": "error", "detail": err.Error(), "session_id": p.SessionID})
		}
	case p.ToolName == "Bash":
		runPostCommitNudge(p)
	}
	return hooks.Response{}
}

// postEditCheckToolInput is the subset of PostToolUsePayload.ToolInput this
// hook reads. Edit/Write carry a single file_path; NotebookEdit carries a
// single notebook_path instead (Claude Code's NotebookEdit tool_input shape);
// MultiEdit carries an edits array of {file_path, ...}. Faithful port of
// post-edit-check.js's tool_input field extraction.
// Edits is a pointer to distinguish "key absent" (nil) from "key present but
// empty" (non-nil pointing to an empty slice), matching the JS behavior:
// if Array.isArray(tool_input.edits) is true, we use its contents even if
// empty, with no fallback to file_path.
type postEditCheckToolInput struct {
	FilePath     string `json:"file_path"`
	NotebookPath string `json:"notebook_path"`
	Edits        *[]struct {
		FilePath string `json:"file_path"`
	} `json:"edits"`
}

// runPostEditCheck implements the PostToolUse Edit-family hook flow: for
// each edited file (deduplicated only in the sense the Node original
// wasn't — every path in a MultiEdit's edits array is checked
// independently), it applies a 10-minute per-file cooldown (keyed by an
// MD5 hash of the file's path, not its content), then searches the KB for
// entries mentioning the file's basename stem. A hit writes a staleness
// hint to stderr; the Response is always the zero value — this hook never
// blocks or injects AdditionalContext, matching post-edit-check.js writing
// via process.stderr.write and always exiting 0.
func runPostEditCheck(p hooks.PostToolUsePayload, db *sql.DB) {
	var input postEditCheckToolInput
	if err := json.Unmarshal(p.ToolInput, &input); err != nil {
		return
	}

	var filePaths []string
	if input.Edits != nil {
		// edits key is present: use its contents (may be empty, no fallback).
		for _, e := range *input.Edits {
			if e.FilePath != "" {
				filePaths = append(filePaths, e.FilePath)
			}
		}
	} else if input.FilePath != "" {
		// edits key absent: fall back to top-level file_path.
		filePaths = []string{input.FilePath}
	} else if input.NotebookPath != "" {
		// edits and file_path both absent: fall back to notebook_path
		// (NotebookEdit's tool_input shape).
		filePaths = []string{input.NotebookPath}
	}

	firstFilePath := ""
	if len(filePaths) > 0 {
		firstFilePath = filePaths[0]
	}
	project := projectFromPath(firstFilePath)

	logHookEvent(map[string]any{"hook": "post_edit_check", "kind": "fire", "session_id": p.SessionID, "project": project})

	if len(filePaths) == 0 || project == "" {
		logHookEvent(map[string]any{"hook": "post_edit_check", "kind": "noop", "session_id": p.SessionID, "project": project})
		return
	}

	nudgeFired := false
	for _, filePath := range filePaths {
		fileHash := postEditCheckFileHash(filePath)
		cooldownFile := getCooldownDir("post-edit-check", fileHash)
		if isWithinCooldown(cooldownFile, postEditCheckCooldownMS) {
			continue
		}

		basename := filepath.Base(filePath)
		stem := trailingExtRe.ReplaceAllString(basename, "")
		if stem == "" || utf8.RuneCountInString(stem) < postEditCheckMinStemLen {
			continue
		}

		escaped := strings.ReplaceAll(stem, "'", "")
		rows, err := store.KeywordSearch(db, escaped, []string{project}, postEditCheckMaxRows)
		if err != nil || len(rows) == 0 {
			continue
		}

		touchFile(cooldownFile)

		titles := make([]string, len(rows))
		for i, r := range rows {
			titles[i] = "[" + r.Type + "] " + r.Title
		}
		fmt.Fprintf(os.Stderr, "[ouroboros] KB refs %s: %s — check staleness\n", basename, strings.Join(titles, ", "))
		nudgeFired = true
	}

	if nudgeFired {
		logHookEvent(map[string]any{"hook": "post_edit_check", "kind": "nudge", "session_id": p.SessionID, "project": project})
	} else {
		logHookEvent(map[string]any{"hook": "post_edit_check", "kind": "noop", "session_id": p.SessionID, "project": project})
	}
}

// postEditCheckFileHash returns the first 8 hex characters of the MD5 hash
// of filePath itself (the path string, not the file's contents). Faithful
// port of post-edit-check.js's per-file cooldown key:
// `crypto.createHash('md5').update(filePath).digest('hex').substring(0, 8)`.
func postEditCheckFileHash(filePath string) string {
	sum := md5.Sum([]byte(filePath)) //nolint:gosec // cache key, not a security boundary
	return hex.EncodeToString(sum[:])[:8]
}
