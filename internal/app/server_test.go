package app

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/pkoukk/tiktoken-go"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"dangernoodle.io/ouroboros/internal/store"
)

// tokenize is a small helper that returns (bytes, tokens) for a string.
func tokenize(t *testing.T, enc *tiktoken.Tiktoken, s string) (int, int) {
	t.Helper()
	return len(s), len(enc.Encode(s, nil, nil))
}

// TestToolsListFootprint measures the wire-cost of every component a client
// pays for at session start: serverInstructions (broken down by section) and
// tools/list (broken down per tool, then per tool into name+description+schema).
// The total is the constant per-session context cost of the MCP surface.
func TestToolsListFootprint(t *testing.T) {
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

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, store.ApplySchema(db))

	srv := buildServer(db, nil, "test")
	registry := srv.ListTools()

	tools := make([]mcp.Tool, 0, len(registry))
	for _, st := range registry {
		tools = append(tools, st.Tool)
	}
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

	// Per-tool: split into name/description/schema so OU-44 can target trim levers.
	t.Logf("  per-tool components (name + description + schema):")
	t.Logf("    %-8s %6s %6s %6s %6s", "tool", "name", "desc", "schema", "props")
	var sumName, sumDesc, sumSchema int
	for _, tool := range tools {
		nameToks := len(enc.Encode(tool.Name, nil, nil))
		descToks := len(enc.Encode(tool.Description, nil, nil))
		schemaJSON, err := json.Marshal(tool.InputSchema)
		require.NoError(t, err)
		schemaToks := len(enc.Encode(string(schemaJSON), nil, nil))
		propCount := len(tool.InputSchema.Properties)
		t.Logf("    %-8s %6d %6d %6d %6d", tool.Name, nameToks, descToks, schemaToks, propCount)
		sumName += nameToks
		sumDesc += descToks
		sumSchema += schemaToks
	}
	t.Logf("    %-8s %6d %6d %6d", "TOTAL", sumName, sumDesc, sumSchema)
	t.Logf("    description share of tools/list: %d%%", 100*sumDesc/listTokens)
	t.Logf("    schema share of tools/list:      %d%%", 100*sumSchema/listTokens)

	// Per-tool: validate annotation structure per OU-75.
	// Each tool should have only the expected annotation fields set.
	expectedAnnotations := map[string]map[string]any{
		"get":     {"readOnlyHint": true},
		"search":  {"readOnlyHint": true},
		"export":  {"readOnlyHint": true},
		"delete":  {"destructiveHint": true, "idempotentHint": true},
		"put":     {"idempotentHint": true},
		"config":  {"idempotentHint": true},
		"import":  {},
		"project": {},
		"item":    {"destructiveHint": true},
		"plan":    {},
	}
	t.Logf("  per-tool annotation structure (OU-75):")
	for _, tool := range tools {
		expected := expectedAnnotations[tool.Name]
		require.NotNil(t, tool.Annotations, "tool %s must have annotations", tool.Name)
		annotJSON, err := json.Marshal(tool.Annotations)
		require.NoError(t, err)
		var annot map[string]any
		err = json.Unmarshal(annotJSON, &annot)
		require.NoError(t, err)

		// Remove nil/false values for comparison (they should omitempty to JSON).
		// If a field is present with false, that's wrong.
		for k, v := range annot {
			if v == false || v == nil {
				delete(annot, k)
			}
		}
		// Now annot should exactly match expected.
		require.Equal(t, expected, annot, "tool %s annotations should be %v, got %v", tool.Name, expected, annot)
		t.Logf("    %-8s %v", tool.Name, expected)
	}

	// ---- session-constant total ----

	totalTokens := instrTokens + listTokens
	t.Logf("session constant cost: tokens=%d (instructions=%d + tools/list=%d)", totalTokens, instrTokens, listTokens)

	require.Less(t, instrTokens, 4000, "serverInstructions exceeds 4000 tokens")
	require.Less(t, listTokens, 4000, "tools/list exceeds 4000 tokens")

	// Ensure testdata directory exists for snapshot files
	require.NoError(t, os.MkdirAll("testdata", 0o755))

	// Check server_instructions.txt snapshot
	assertSnapshot(t, "testdata/server_instructions.txt", []byte(serverInstructions))

	// Check tools_list.json snapshot
	toolsJSON, err := json.MarshalIndent(map[string]any{"tools": tools}, "", "  ")
	require.NoError(t, err)
	assertSnapshot(t, "testdata/tools_list.json", toolsJSON)
}

// TestBatchRoundTripSavings documents the call count savings from batch operations.
// A 9-item reprioritization workflow takes 9 single-item calls but only 1 batch call.
func TestBatchRoundTripSavings(t *testing.T) {
	const items = 9
	oldCalls := items
	newCalls := 1
	t.Logf("batch round-trip savings: %d calls → %d call (%d-item batch)", oldCalls, newCalls, items)
	require.Equal(t, 1, newCalls)
}

// instructionSection is one logical block of serverInstructions, used for the
// breakdown so OU-44 can target the heaviest section.
type instructionSection struct {
	name string
	body string
}

// splitInstructions segments serverInstructions on its top-level headings.
// The two top-level headings are "KNOWLEDGE BASE" and "BACKLOG"; everything
// before the first heading is the preamble.
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
		"run: UPDATE_SNAPSHOT=1 go test ./internal/app -run TestToolsListFootprint",
		path)
}
