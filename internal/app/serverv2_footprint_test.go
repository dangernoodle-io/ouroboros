package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/dangernoodle-io/mcpkit/mcpx"
	"github.com/dangernoodle-io/mcpkit/testkit"
	"github.com/pkoukk/tiktoken-go"
	"github.com/stretchr/testify/require"
)

// tokenize is a small helper that returns (bytes, tokens) for a string.
func tokenize(t *testing.T, enc *tiktoken.Tiktoken, s string) (int, int) {
	t.Helper()
	return len(s), len(enc.Encode(s, nil, nil))
}

// TestToolsListFootprintV2 measures the wire-cost of every component a
// client pays for at session start against the SERVED server: total and
// per-tool/per-component (name/description/schema) breakdowns for
// tools/list, plus serverInstructions. Descriptions were trimmed at OU-298
// (tools/list ~1996 tokens); the require.Less budgets below are a conscious
// ceiling, not a byte-for-byte replay of any prior measurement.
func TestToolsListFootprintV2(t *testing.T) {
	enc, err := tiktoken.GetEncoding("cl100k_base")
	require.NoError(t, err)

	// ---- serverInstructions: total + per-section ----

	instrBytes, instrTokens := tokenize(t, enc, serverInstructions)
	t.Logf("serverInstructions: bytes=%d tokens=%d", instrBytes, instrTokens)

	for _, sec := range splitInstructions(serverInstructions) {
		b, toks := tokenize(t, enc, sec.body)
		t.Logf("  %-15s bytes=%4d tokens=%4d", sec.name, b, toks)
	}

	// ---- tools/list: total + per-tool + per-component ----

	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)
	res, err := h.ListTools(context.Background())
	require.NoError(t, err)

	tools := make([]*mcpx.Tool, len(res.Tools))
	copy(tools, res.Tools)
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })

	payload, err := json.Marshal(map[string]any{"tools": tools})
	require.NoError(t, err)
	listBytes, listTokens := tokenize(t, enc, string(payload))
	t.Logf("tools/list: tools=%d bytes=%d tokens=%d", len(tools), listBytes, listTokens)

	// Per-tool: total cost
	t.Logf("  per-tool totals:")
	for _, tool := range tools {
		b, err := json.Marshal(tool)
		require.NoError(t, err)
		bts, toks := tokenize(t, enc, string(b))
		t.Logf("    %-8s bytes=%4d tokens=%4d", tool.Name, bts, toks)
	}

	// Per-tool: split into name/description/schema so future trims can
	// target the heaviest lever.
	t.Logf("  per-tool components (name + description + schema):")
	t.Logf("    %-8s %6s %6s %6s", "tool", "name", "desc", "schema")
	var sumName, sumDesc, sumSchema int
	for _, tool := range tools {
		nameToks := len(enc.Encode(tool.Name, nil, nil))
		descToks := len(enc.Encode(tool.Description, nil, nil))
		schemaJSON, err := json.Marshal(tool.InputSchema)
		require.NoError(t, err)
		schemaToks := len(enc.Encode(string(schemaJSON), nil, nil))
		t.Logf("    %-8s %6d %6d %6d", tool.Name, nameToks, descToks, schemaToks)
		sumName += nameToks
		sumDesc += descToks
		sumSchema += schemaToks
	}
	t.Logf("    %-8s %6d %6d %6d", "TOTAL", sumName, sumDesc, sumSchema)
	if listTokens > 0 {
		t.Logf("    description share of tools/list: %d%%", 100*sumDesc/listTokens)
		t.Logf("    schema share of tools/list:      %d%%", 100*sumSchema/listTokens)
	}

	// Per-tool: validate annotation structure (mirrors the old server's
	// OU-75 check, restored on V2 at OU-5a).
	expectedReadOnly := map[string]bool{"query": true}
	expectedIdempotent := map[string]bool{"kb": true}
	expectedDestructive := map[string]bool{"backlog": true, "roadmap": true}
	for _, tool := range tools {
		require.NotNil(t, tool.Annotations, "tool %s must have annotations", tool.Name)
		if expectedReadOnly[tool.Name] {
			require.True(t, tool.Annotations.ReadOnlyHint, "tool %s should carry ReadOnlyHint", tool.Name)
		}
		if expectedIdempotent[tool.Name] {
			require.True(t, tool.Annotations.IdempotentHint, "tool %s should carry IdempotentHint", tool.Name)
		}
		if expectedDestructive[tool.Name] {
			require.NotNil(t, tool.Annotations.DestructiveHint, "tool %s should carry DestructiveHint", tool.Name)
			require.True(t, *tool.Annotations.DestructiveHint, "tool %s DestructiveHint should be true", tool.Name)
		}
	}

	// ---- session-constant total ----

	totalTokens := instrTokens + listTokens
	t.Logf("session constant cost: tokens=%d (instructions=%d + tools/list=%d)", totalTokens, instrTokens, listTokens)

	require.Less(t, instrTokens, 4000, "serverInstructions exceeds 4000 tokens")
	// Tightened at OU-323 (get+search collapsed into one query tool) from the
	// OU-298 budget (2200 tokens, ~1996 measured with 5 tools) to a
	// conscious budget just above the new measured cost (~1717 tokens with
	// 4 tools): a deliberate increase must consciously raise this budget.
	require.Less(t, listTokens, 1800, "tools/list exceeds 1800 tokens — a deliberate increase must consciously raise this budget")

	// Ensure testdata directory exists for snapshot files
	require.NoError(t, os.MkdirAll("testdata", 0o755))

	// Check server_instructions.txt snapshot (unchanged content, moved home
	// at OU-5a; still asserted here since this is now the served surface).
	assertSnapshot(t, "testdata/server_instructions.txt", []byte(serverInstructions))

	// Check tools_list.json snapshot (V2 shape: go-sdk-generated schemas,
	// regenerated at the OU-5 cutover -- NOT a replay of the old mark3labs
	// snapshot).
	toolsJSON, err := json.MarshalIndent(map[string]any{"tools": tools}, "", "  ")
	require.NoError(t, err)
	assertSnapshot(t, "testdata/tools_list.json", toolsJSON)
}

// TestAllToolsRegisteredAtStartupV2 verifies all 4 tools are available
// immediately after buildServerV2 composes, without requiring any prior
// tool invocations -- V2's counterpart to the old buildServer's
// TestAllToolsRegisteredAtStartup (deleted at the OU-5 cutover).
func TestAllToolsRegisteredAtStartupV2(t *testing.T) {
	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)
	res, err := h.ListTools(context.Background())
	require.NoError(t, err)

	t.Logf("startup: %d total tools (expected 4)", len(res.Tools))
	require.Len(t, res.Tools, 4, "should have exactly 4 tools at startup")

	byName := make(map[string]bool, len(res.Tools))
	for _, tool := range res.Tools {
		byName[tool.Name] = true
	}
	for _, name := range []string{"query", "kb", "backlog", "roadmap"} {
		require.True(t, byName[name], "tool %s should be present", name)
	}
}

// instructionSection is one logical block of serverInstructions, used for
// the breakdown so a future trim can target the heaviest section.
type instructionSection struct {
	name string
	body string
}

// splitInstructions segments serverInstructions on its top-level headings.
// V2's serverInstructions (OU-5a) is a flat bullet list with no top-level
// headings, so this always falls through to the single "all" section here
// -- kept for parity with the old server's breakdown shape and in case a
// future revision reintroduces headings.
func splitInstructions(s string) []instructionSection {
	const (
		kbMarker = "KNOWLEDGE BASE ("
		blMarker = "BACKLOG ("
	)
	kbIdx := strings.Index(s, kbMarker)
	blIdx := strings.Index(s, blMarker)
	if kbIdx < 0 || blIdx < 0 || blIdx < kbIdx {
		return []instructionSection{{name: "all", body: s}}
	}
	return []instructionSection{
		{name: "preamble", body: strings.TrimSpace(s[:kbIdx])},
		{name: "KNOWLEDGE BASE", body: strings.TrimSpace(s[kbIdx:blIdx])},
		{name: "BACKLOG", body: strings.TrimSpace(s[blIdx:])},
	}
}

// assertSnapshot reads the snapshot file at path and compares it to the live
// output. If the file is missing and UPDATE_SNAPSHOT=1, it creates the file.
// If there's a drift and UPDATE_SNAPSHOT=1, it regenerates the file.
// If there's a drift and UPDATE_SNAPSHOT is not set, it fails with a clear hint.
func assertSnapshot(t *testing.T, path string, live []byte) {
	t.Helper()

	existing, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) && os.Getenv("UPDATE_SNAPSHOT") == "1" {
		require.NoError(t, os.WriteFile(path, live, 0o644))
		t.Logf("snapshot created: %s", path)
		return
	}
	require.NoError(t, err, "snapshot %s missing", path)

	if bytes.Equal(existing, live) {
		return
	}

	if os.Getenv("UPDATE_SNAPSHOT") == "1" {
		require.NoError(t, os.WriteFile(path, live, 0o644))
		t.Logf("snapshot regenerated: %s", path)
		return
	}

	t.Fatalf("snapshot drift: %s is out of sync with live output.\n"+
		"run: UPDATE_SNAPSHOT=1 go test ./internal/app -run TestToolsListFootprintV2",
		path)
}
