package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/dangernoodle-io/shesha/host/claudecode/hooks"
)

// postCommitNudgeCooldownMS is the per-project cooldown window (5 minutes).
// Faithful port of post-commit-nudge.js's COOLDOWN_MS.
const postCommitNudgeCooldownMS = 300000

// postCommitNudgeText is the stderr line emitted after a git-commit Bash
// command clears cooldown. Faithful port of post-commit-nudge.js's
// console.error('[ouroboros] /persist to save decisions').
const postCommitNudgeText = "[ouroboros] /persist to save decisions"

// postCommitNudgeProjectSanitizeRe replaces path-unsafe characters in a
// project name with hyphens before it's used as a cooldown-file path
// segment. Faithful port of post-commit-nudge.js's
// project.replace(/[^a-zA-Z0-9._-]/g, '-').
var postCommitNudgeProjectSanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// postCommitNudgeToolInput is the subset of PostToolUsePayload.ToolInput this
// hook reads: the Bash command string. Faithful port of post-commit-nudge.js's
// data.tool_input?.command, defaulting to an empty string when absent.
type postCommitNudgeToolInput struct {
	Command string `json:"command"`
}

// postCommitNudgeCooldownFile returns the cooldown-file path for project,
// falling back to an "unknown" bucket when project is empty. Faithful port
// of post-commit-nudge.js's getCooldownFile.
func postCommitNudgeCooldownFile(project string) string {
	if project == "" {
		return getCooldownDir("post-commit-nudge", "unknown")
	}
	sanitized := postCommitNudgeProjectSanitizeRe.ReplaceAllString(project, "-")
	return getCooldownDir("post-commit-nudge", sanitized)
}

// runPostCommitNudge implements the PostToolUse Bash-branch hook flow: if the
// Bash command run is a git commit, applies a 5-minute per-project cooldown,
// then writes a /persist nudge to stderr. No KB/DB read at all — this hook
// only ever inspects tool_input and a cooldown-file mtime, matching
// post-commit-nudge.js exactly. Never returns an error; malformed tool_input
// is silently ignored (mirrors the JS's JSON.parse-failure exit(0)). Benign
// divergence: JS logs fire unconditionally, Go logs after parsing — only differs
// when tool_input is absent, which does not occur in real PostToolUse payloads.
func runPostCommitNudge(p hooks.PostToolUsePayload) {
	var input postCommitNudgeToolInput
	if err := json.Unmarshal(p.ToolInput, &input); err != nil {
		return
	}

	project := ""
	if p.Cwd != "" {
		project = projectFromPath(p.Cwd)
	}

	logHookEvent(map[string]any{"hook": "post_commit_nudge", "kind": "fire", "session_id": p.SessionID, "project": project})

	if !strings.Contains(strings.ToLower(input.Command), "git commit") {
		logHookEvent(map[string]any{"hook": "post_commit_nudge", "kind": "noop", "session_id": p.SessionID, "project": project})
		return
	}

	cooldownFile := postCommitNudgeCooldownFile(project)
	if isWithinCooldown(cooldownFile, postCommitNudgeCooldownMS) {
		logHookEvent(map[string]any{"hook": "post_commit_nudge", "kind": "noop", "session_id": p.SessionID, "project": project})
		return
	}

	touchFile(cooldownFile)
	fmt.Fprintln(os.Stderr, postCommitNudgeText)
	logHookEvent(map[string]any{"hook": "post_commit_nudge", "kind": "nudge", "session_id": p.SessionID, "project": project})
}
