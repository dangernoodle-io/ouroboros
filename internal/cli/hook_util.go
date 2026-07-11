package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
	"unicode/utf8"
)

// maxHookTailBytes caps how much of a transcript file is read into memory
// for the turn-boundary scans below. Mirrors plugin/scripts/lib.js's
// MAX_TAIL_BYTES: avoids loading full multi-MB transcripts, reading only the
// tail where recent turns live.
const maxHookTailBytes = 2 * 1024 * 1024 // 2 MB

// maxHookScanLines caps how many trailing JSONL lines are scanned.
const maxHookScanLines = 2000

// kbFenceRe matches the first fenced ```kb block in a message, capturing its
// body. Mirrors lib.js's extractKbBlock regex.
var kbFenceRe = regexp.MustCompile("```kb\\s*\n([\\s\\S]*?)\n```")

// ouroborosWriteToolRe matches a tool_use name that is one of the ouroboros
// MCP write tools (kb, backlog, roadmap, and legacy put/item), served under
// an "ouroboros" MCP server namespace.
var ouroborosWriteToolRe = regexp.MustCompile(`(?i)(^|_)(kb|backlog|roadmap|put|item)$`)

var ouroborosNameRe = regexp.MustCompile(`(?i)ouroboros`)

// tier2Patterns: self-claim signals meaning the context already persisted
// via a tool/MCP call.
var tier2Patterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\[ouroboros\][^\n]*persisted \d+ entr`),
	regexp.MustCompile(`(?i)mcp__[^"]*__kb[^"]*"action":\s*"(create|update)"`),
}

// tier1Patterns: narrow decision-language signals — the DECISION_PATTERNS
// const in lib.js, not the broader decisionPatterns list used for
// extractAllKbBlocks.
var tier1Patterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bdecided to\b`),
	regexp.MustCompile(`(?i)\bchose .+ over\b`),
	regexp.MustCompile(`(?i)\bdesign decision`),
	regexp.MustCompile(`(?i)\bgoing with\b`),
	regexp.MustCompile(`(?i)\bwe('|\x{2019})ll use\b`),
	regexp.MustCompile(`(?i)\binstead of\b.{0,30}\bbecause\b`),
}

// extractKbBlock returns the body of the first fenced ```kb block in message
// and whether one was found.
func extractKbBlock(message string) (bool, string) {
	m := kbFenceRe.FindStringSubmatch(message)
	if m == nil {
		return false, ""
	}
	return true, m[1]
}

// isOuroborosWriteTool reports whether name looks like an ouroboros MCP
// write-tool invocation (kb/backlog/roadmap/put/item under an
// "ouroboros"-named server).
func isOuroborosWriteTool(name string) bool {
	if name == "" {
		return false
	}
	if !ouroborosNameRe.MatchString(name) {
		return false
	}
	return ouroborosWriteToolRe.MatchString(name)
}

// truncateMessage caps s at maxBytes, backing off to a valid UTF-8 rune
// boundary. This is a byte-safe analog of the Node hook's `substring(0,5000)`
// character truncation — a large multi-byte-heavy message may be truncated a
// few runes shorter than the JS char-count equivalent, but never splits a
// rune. maxBytes is a parameter (rather than a hardcoded 5000) so its
// boundary-safety logic is directly unit-testable at multiple cut points.
func truncateMessage(s string, maxBytes int) string { //nolint:unparam
	if len(s) <= maxBytes {
		return s
	}
	b := s[:maxBytes]
	for len(b) > 0 {
		r, size := utf8.DecodeLastRuneInString(b)
		if r != utf8.RuneError || size != 1 {
			break
		}
		b = b[:len(b)-1]
	}
	return b
}

// findGitRoot walks up from startPath to the filesystem root, returning the
// first ancestor directory containing a .git entry, or "" if none found.
// Faithful port of lib.js findGitRoot.
func findGitRoot(startPath string) string {
	if startPath == "" {
		return ""
	}
	current, err := filepath.Abs(startPath)
	if err != nil {
		return ""
	}
	if info, statErr := os.Stat(current); statErr == nil {
		if !info.IsDir() {
			current = filepath.Dir(current)
		}
	} else {
		current = filepath.Dir(current)
	}

	for {
		gitPath := filepath.Join(current, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

// projectFromPath resolves a project name from a filesystem path by walking
// up to the nearest git root and returning its base name, or "" if none
// found. Faithful port of lib.js projectFromPath.
func projectFromPath(startPath string) string {
	root := findGitRoot(startPath)
	if root == "" {
		return ""
	}
	return filepath.Base(root)
}

// transcriptLine is the minimal shape of a Claude Code transcript JSONL
// record needed by the hook helpers below.
type transcriptLine struct {
	Type        string         `json:"type"`
	IsSidechain bool           `json:"isSidechain"`
	Message     *transcriptMsg `json:"message"`
}

type transcriptMsg struct {
	Content json.RawMessage `json:"content"`
}

// transcriptContentBlock is one entry of a transcript message's content
// array: an assistant text block or a tool_use invocation.
type transcriptContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
	Name string `json:"name"`
}

// readLastMainAssistantText scans a transcript JSONL backwards and returns
// the concatenated text of the most recent main-context (non-sidechain)
// assistant turn. Returns "" if not found or on any read error. Only scans
// the tail of the transcript (readTranscriptTail's maxHookTailBytes cap),
// same bound turnAlreadyPersisted uses, so a very large transcript file
// never triggers an unbounded read.
func readLastMainAssistantText(transcriptPath string) string {
	// Also applies maxHookScanLines (unconditionally, even on sub-cap
	// files) — a benign alignment with turnAlreadyPersisted's bounded
	// tail+scan-line window; this function previously had no line cap.
	lines := readTranscriptTail(transcriptPath)
	for i := len(lines) - 1; i >= 0; i-- {
		var entry transcriptLine
		if err := json.Unmarshal(lines[i], &entry); err != nil {
			continue
		}
		if entry.Type != "assistant" || entry.IsSidechain {
			continue
		}
		if entry.Message == nil {
			continue
		}
		var blocks []transcriptContentBlock
		if err := json.Unmarshal(entry.Message.Content, &blocks); err != nil {
			continue
		}
		text := joinTextBlocks(blocks)
		if text != "" {
			return text
		}
	}
	return ""
}

func joinTextBlocks(blocks []transcriptContentBlock) string {
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += "\n" + p
	}
	return out
}

// readTranscriptTail reads transcriptPath, capping at maxHookTailBytes from
// the end (dropping the first, possibly partial, line when capped), and
// returns up to maxHookScanLines trailing non-blank lines. Faithful port of
// lib.js's shared tail-read logic used by extractAllKbBlocks/
// turnAlreadyPersisted.
func readTranscriptTail(transcriptPath string) [][]byte {
	info, err := os.Stat(transcriptPath)
	if err != nil {
		return nil
	}

	var raw []byte
	if info.Size() > maxHookTailBytes {
		f, err := os.Open(transcriptPath)
		if err != nil {
			return nil
		}
		defer f.Close() //nolint:errcheck
		buf := make([]byte, maxHookTailBytes)
		n, err := f.ReadAt(buf, info.Size()-maxHookTailBytes)
		if err != nil && n == 0 {
			return nil
		}
		tail := buf[:n]
		if idx := bytes.IndexByte(tail, '\n'); idx >= 0 {
			raw = tail[idx+1:]
		} else {
			raw = tail
		}
	} else {
		raw, err = os.ReadFile(transcriptPath)
		if err != nil {
			return nil
		}
	}

	var lines [][]byte
	for _, l := range bytes.Split(raw, []byte("\n")) {
		if len(bytes.TrimSpace(l)) > 0 {
			lines = append(lines, l)
		}
	}
	if len(lines) > maxHookScanLines {
		lines = lines[len(lines)-maxHookScanLines:]
	}
	return lines
}

// isGenuineUserTurnBoundary distinguishes a real human-submitted user
// message from a tool_result "user" record (the Anthropic API represents
// tool results as role:"user" messages). A record is a genuine user-turn
// boundary if its content is a string, not an array, empty, or not
// exclusively tool_result blocks.
func isGenuineUserTurnBoundary(content json.RawMessage) bool {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return true
	}
	if trimmed[0] == '"' {
		return true
	}
	if trimmed[0] != '[' {
		return true
	}
	var blocks []transcriptContentBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		return true
	}
	if len(blocks) == 0 {
		return true
	}
	for _, b := range blocks {
		if b.Type != "tool_result" {
			return true
		}
	}
	return false
}

// turnAlreadyPersisted scans a transcript JSONL backwards from the end,
// within the CURRENT turn only (stopping at the most recent genuine
// user-prompt record), looking for a signal that the session already
// persisted knowledge this turn: either an ouroboros MCP KB/backlog write
// tool_use, or a prior emitted ```kb fenced block in an earlier assistant
// message this turn. Fail-open: returns false on any read/parse failure.
func turnAlreadyPersisted(transcriptPath string) bool {
	lines := readTranscriptTail(transcriptPath)
	for i := len(lines) - 1; i >= 0; i-- {
		var entry transcriptLine
		if err := json.Unmarshal(lines[i], &entry); err != nil {
			continue
		}

		if entry.Type == "user" {
			var content json.RawMessage
			if entry.Message != nil {
				content = entry.Message.Content
			}
			if isGenuineUserTurnBoundary(content) {
				break
			}
			continue
		}

		if entry.Type != "assistant" || entry.Message == nil {
			continue
		}

		var blocks []transcriptContentBlock
		if err := json.Unmarshal(entry.Message.Content, &blocks); err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "tool_use" && isOuroborosWriteTool(b.Name) {
				return true
			}
			if b.Type == "text" && b.Text != "" {
				if matched, _ := extractKbBlock(b.Text); matched {
					return true
				}
			}
		}
	}
	return false
}

// logHookEvent best-effort appends a JSONL event line to
// ${HOME}/.ouroboros/hooks.log, gated by OUROBOROS_HOOK_LOG (skip on
// "0"/"false"/"off"). Size-based rotation (present in the Node original) is
// intentionally dropped as non-essential; all failures are swallowed — this
// must never break a hook.
func logHookEvent(fields map[string]any) {
	flag := os.Getenv("OUROBOROS_HOOK_LOG")
	if flag == "0" || flag == "false" || flag == "off" {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".ouroboros")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	entry := map[string]any{"ts": time.Now().UTC().Format(time.RFC3339)}
	for k, v := range fields {
		entry[k] = v
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}

	logPath := filepath.Join(dir, "hooks.log")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close() //nolint:errcheck
	data = append(data, '\n')
	_, _ = f.Write(data)
}

// nudgeOutput is the exit-0 JSON decision protocol Claude Code expects on
// stdout to block a Stop event and surface a reason.
type nudgeOutput struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// checkNudgePatterns runs the tier-2 (self-claim) and tier-1 (decision
// language) checks on message, in that order, and returns the fired nudge
// plus its tier (1 or 2), or nil/0 if neither matched. Logs the nudge event
// as a side effect when matched, matching lib.js checkNudgePatterns.
func checkNudgePatterns(message, label, idShort, hookName, sessionID, project string) (*nudgeOutput, int) {
	for _, p := range tier2Patterns {
		if p.MatchString(message) {
			logHookEvent(map[string]any{"hook": hookName, "kind": "nudge", "session_id": sessionID, "project": project, "reason": "tier-2"})
			return &nudgeOutput{
				Decision: "block",
				Reason:   fmt.Sprintf("[ouroboros] %s %s: tier-2 self-claim detected (no kb block, but message references persistence)", label, idShort),
			}, 2
		}
	}
	for _, p := range tier1Patterns {
		if p.MatchString(message) {
			logHookEvent(map[string]any{"hook": hookName, "kind": "nudge", "session_id": sessionID, "project": project, "reason": "tier-1"})
			return &nudgeOutput{
				Decision: "block",
				Reason:   fmt.Sprintf("[ouroboros] %s %s: tier-1 nudge fired (decision language present, no kb block)", label, idShort),
			}, 1
		}
	}
	return nil, 0
}
