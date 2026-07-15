package cli

import (
	"context"
	"database/sql"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dangernoodle-io/mcpkit/host/claudecode/hooks"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/store"
)

// upcCooldownMS is the per-project cooldown window (5 minutes), overridable
// via OUROBOROS_UPC_COOLDOWN_MS. Faithful port of user-prompt-context.js's
// COOLDOWN_MS.
const upcCooldownMS = 300000

// upcMaxEntries caps the number of rows returned for a "resume" intent's KB
// query. Faithful port of MAX_ENTRIES.
const upcMaxEntries = 10

// upcMaxSearch caps the number of rows returned for a "specific" intent's KB
// search. Faithful port of MAX_SEARCH.
const upcMaxSearch = 5

// upcBM25Threshold: bm25() returns a negative score; more negative = better
// match, so keep rows at or below the threshold. Search results weaker
// (less negative) than this threshold are dropped.
const upcBM25Threshold = -2.0

// upcSkipPatterns: low-signal or unrelated prompts (acks, greetings, single
// words). Faithful port of user-prompt-context.js's SKIP_PATTERNS.
var upcSkipPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^\s*(hi|hey|hello|thanks|thank you|yes|no|ok|okay|yep|nope|sure|got it)[\s.!?]*$`),
	regexp.MustCompile(`(?i)^\s*tool loaded\.?\s*$`),
	regexp.MustCompile(`(?i)^\s*(next|continue|more)\s*[.!?]*$`),
}

// upcResumePatterns: the user is starting or continuing work on a project.
// Faithful port of RESUME_PATTERNS.
var upcResumePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(pick up|picking up|continue|continuing|resume|resuming)\b`),
	regexp.MustCompile(`(?i)\b(back to|get back|return to)\b`),
	regexp.MustCompile(`(?i)\b(where were we|where did we leave|what's the status|what is the status)\b`),
	regexp.MustCompile(`(?i)\b(let's work on|lets work on|start on|starting on)\b`),
	regexp.MustCompile(`(?i)\b(what's next|what is next|what do we need)\b`),
	regexp.MustCompile(`(?i)\bbacklog\b`),
}

// classifyPrompt classifies a UserPromptSubmit prompt into one of: "none"
// (empty), "unrelated" (slash command, low-signal ack, or too short to be
// substantive), "resume" (starting/continuing project work), or "specific"
// (a substantive, non-resume prompt). Faithful port of
// user-prompt-context.js's classifyPrompt.
func classifyPrompt(message string) string {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return "none"
	}
	if strings.HasPrefix(trimmed, "/") {
		return "unrelated"
	}
	if matchesAnyPattern(trimmed, upcSkipPatterns) {
		return "unrelated"
	}
	if utf8.RuneCountInString(trimmed) < 15 {
		return "unrelated"
	}
	if matchesAnyPattern(message, upcResumePatterns) {
		return "resume"
	}
	return "specific"
}

// matchesAnyPattern reports whether any pattern in patterns matches message.
func matchesAnyPattern(message string, patterns []*regexp.Regexp) bool {
	for _, p := range patterns {
		if p.MatchString(message) {
			return true
		}
	}
	return false
}

// hookHandleUserPromptSubmit is the UserPromptSubmit-hook KB/backlog context
// injector, a native port of plugin/scripts/user-prompt-context.js, wired as
// a hooks.Handler[hooks.UserPromptSubmitPayload]: given the prompt and the
// session's cwd, it resolves a project, and — cooldown permitting — injects
// a compact KB (and, for a "resume" intent, backlog) summary as additional
// context. This hook is advisory: a withDB failure is logged to stderr only
// and never surfaces as a blocking Response — the zero hooks.Response
// ("silent allow") is returned instead, matching the Node script's
// catch-all exit(0).
func hookHandleUserPromptSubmit(_ context.Context, _ io.Reader, p hooks.UserPromptSubmitPayload) hooks.Response {
	var resp hooks.Response
	if err := withDB(func(db *sql.DB) error {
		resp = runHookUserPromptSubmit(p, db)
		return nil
	}); err != nil {
		logHookEvent(map[string]any{"hook": "user_prompt_context", "kind": "error", "detail": err.Error(), "session_id": p.SessionID})
	}
	return resp
}

// runHookUserPromptSubmit implements the UserPromptSubmit-hook flow.
//
// Project resolution order (OU-283 fix — the wrong-project injection bug):
// this deliberately differs from the Node original, which tried cwd FIRST
// and only fell back to a prompt mention when the cwd had no reachable
// .git. That ordering meant a session whose cwd happened to be a real git
// repo — but the WRONG one for the work actually being discussed, e.g. the
// dangernoodle-marketplace hub repo, which every plugin-dev session's cwd
// can be — always won, unconditionally injecting that repo's KB/backlog
// regardless of an explicit project mention in the prompt. The fixed order:
//  1. Prompt-mention first (resolveProjectFromMessage): if the prompt
//     names one of ouroboros's own REGISTERED projects (backlog.ListProjects
//     — the projects table, not a filesystem walk of workspace
//     subdirectories; see OU-309), that wins outright. Candidate names are
//     sorted longest-first so a more-specific registered project name wins
//     over a shorter one that also happens to match.
//  2. Else cwd, UNLESS the cwd's git root is the marketplace hub itself
//     (isMarketplaceRepo) — the hub is a repo, but never a "project" whose
//     KB/backlog a hook should inject.
//  3. Else inject nothing (fail-open: unresolvable cwd + no message
//     mention + never a wrong guess).
//
// Deliberately does NOT restore the transcript-scan fallback (lib.js
// resolveProject's priority-3, scanning the transcript backwards for the
// most-recently-touched file's project) — that guesses the project of
// whatever file was last read/edited, which can be a different repo than
// the one the session is actually working in, and was deliberately removed
// upstream of this port; see the Node original's inline comment.
func runHookUserPromptSubmit(p hooks.UserPromptSubmitPayload, db *sql.DB) hooks.Response {
	// Log fire event immediately, before any early exits — mirrors the Node
	// original's explicit ordering comment. The project is not yet known at
	// this point (matching the Node original, which logs fire with
	// project: undefined before intent classification/project resolution).
	logHookEvent(map[string]any{"hook": "user_prompt_context", "kind": "fire", "session_id": p.SessionID})

	intent := classifyPrompt(p.Prompt)
	if intent == "none" || intent == "unrelated" {
		return hooks.Response{}
	}

	project := resolveProjectFromMessage(p.Prompt, registeredProjectNamesLongestFirst(db))
	if project == "" && p.Cwd != "" {
		gitRoot := findGitRoot(p.Cwd)
		if gitRoot != "" && !isMarketplaceRepo(gitRoot) {
			project = filepath.Base(gitRoot)
		}
	}
	if project == "" {
		return hooks.Response{}
	}

	// Check cooldown per project (resume prompts bypass cooldown).
	cooldownFile := getCooldownDir("user-prompt-context", project)
	if intent != "resume" && isWithinCooldown(cooldownFile, upcCooldownMS) {
		return hooks.Response{}
	}

	// OU-259: a "resume" prompt ("let's pick up X", "continue working on Y")
	// often still carries usable search terms — prefer bm25-relevance-ranked
	// results (store.KeywordSearch) over an arbitrary recency dump whenever
	// the prompt tokenizes to at least one non-stopword term. Only when the
	// prompt has no usable FTS terms (or the relevance search comes back
	// empty, including empty after the weak-match floor below) do we fall
	// back to QueryDocuments' recency-ordered listing.
	var rows []store.DocumentSummary
	var err error
	if intent == "resume" {
		if len(store.TokenizeQuery(p.Prompt)) > 0 {
			search := truncateRunes(strings.ReplaceAll(p.Prompt, "'", ""), 200)
			rows, err = store.KeywordSearch(db, search, []string{project}, upcMaxEntries)
			if err == nil {
				rows = filterByBM25Threshold(rows)
			}
		}
		if err == nil && len(rows) == 0 {
			rows, err = store.QueryDocuments(db, nil, []string{project}, nil, "", nil, upcMaxEntries)
		}
	} else {
		search := truncateRunes(strings.ReplaceAll(p.Prompt, "'", ""), 200)
		rows, err = store.KeywordSearch(db, search, []string{project}, upcMaxSearch)
	}
	if err != nil || len(rows) == 0 {
		return hooks.Response{}
	}

	// OU-340: the upcBM25Threshold weak-match floor is now applied to BOTH
	// the resume path (above, immediately after KeywordSearch — with a
	// recency fallback when nothing clears the floor) and the specific path
	// (here) — a resume prompt with several usable terms should not inject a
	// real-but-weak bm25 match as if it were high-confidence, same as
	// specific.
	if intent == "specific" {
		rows = filterByBM25Threshold(rows)
		if len(rows) == 0 {
			return hooks.Response{}
		}
	}

	touchFile(cooldownFile)

	lines := []string{"[ouroboros] " + project + " KB (" + strconv.Itoa(len(rows)) + "):"}
	for _, r := range rows {
		lines = append(lines, "  ["+r.Type+"] "+r.Title)
	}
	// OU-198: the per-prompt persist-contract reminder was removed here —
	// serverInstructions (internal/app/serverv2.go) already states the
	// contract once, at session init, rather than repeating it on every
	// qualifying prompt.

	if intent == "resume" {
		if backlogLines := upcBacklogLines(db, project); len(backlogLines) > 0 {
			lines = append(lines, backlogLines...)
		}
	}

	return hooks.Response{AdditionalContext: strings.Join(lines, "\n")}
}

// upcBacklogLines queries the project's open backlog items (native
// backlog.ListItems, replacing the Node original's shell-out to
// `ouroboros ls backlog --json`) and formats them the same way the Node
// original's queryKb + `ls backlog` combination did. Returns nil on any
// failure or empty result — fail-open, matching the Node original's
// try/catch silently skipping the backlog section.
func upcBacklogLines(db *sql.DB, project string) []string {
	proj, err := backlog.GetProjectByName(db, project)
	if err != nil || proj == nil {
		return nil
	}

	status := "open"
	items, err := backlog.ListItems(db, backlog.ItemFilter{
		ProjectIDs: []int64{proj.ID},
		Status:     &status,
		Limit:      10,
	})
	if err != nil || len(items) == 0 {
		return nil
	}

	lines := []string{"[ouroboros] " + project + " backlog (" + strconv.Itoa(len(items)) + "):"}
	for _, item := range items {
		lines = append(lines, "  ["+item.Priority+"] "+item.ID+": "+item.Title)
	}
	return lines
}

// filterByBM25Threshold drops rows whose bm25 score is weaker (less
// negative, i.e. a poorer match) than upcBM25Threshold, keeping only
// strong-enough matches. Shared by both the resume and specific intent
// paths so the weak-match floor is applied identically.
func filterByBM25Threshold(rows []store.DocumentSummary) []store.DocumentSummary {
	filtered := rows[:0:0] //nolint:gocritic // fresh backing array, rows is not reused after this point
	for _, r := range rows {
		if r.Score <= upcBM25Threshold {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// truncateRunes caps s at maxRunes runes (a rune-safe analog of the Node
// hook's `substring(0, 200)` character truncation). maxRunes is a parameter
// (rather than a hardcoded 200) so this stays directly unit-testable at
// multiple cut points; both of its current call sites (user-prompt-submit
// and subagent-start) happen to pass the same 200 today.
func truncateRunes(s string, maxRunes int) string { //nolint:unparam

	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes])
}
