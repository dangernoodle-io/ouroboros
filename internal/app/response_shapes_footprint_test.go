package app

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/dangernoodle-io/shesha/mcpx"
	"github.com/dangernoodle-io/shesha/testkit"
	"github.com/pkoukk/tiktoken-go"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
)

// responseShapeCeiling is the per-shape token budget for
// TestResponseShapesFootprint (OU-211). This is a conscious budget, not an
// aspirational one: the worst shape measured at authoring time was roadmap
// format=html render at 2245 tokens (see the test's t.Logf table); 2700
// gives it ~20% headroom while staying well under OU-77's original
// 4000-token red-flag line -- every shape here has real room before that
// line, so this ceiling is deliberately tighter than 4000, not a re-cast of
// it. A future increase must be a deliberate, reviewed edit here -- not a
// silent creep as response shapes grow richer.
const responseShapeCeiling = 2700

// shapeMeasurement is one row of the per-shape token-cost table.
type shapeMeasurement struct {
	name   string
	bytes  int
	tokens int
}

// seedResponseShapeFixtures populates a deterministic, anonymized fixture
// DB (kb docs + backlog items + roadmap entries) over the real MCP wire,
// mirroring the OU-77 python probe's seed shape (bench/response-shapes/
// probe.py, removed at OU-211), and returns the IDs the shape table needs.
func seedResponseShapeFixtures(t *testing.T, h *testkit.Harness) (kbIDs []int64, acmeIDs, umbrellaIDs []string) {
	t.Helper()
	ctx := context.Background()

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.CreateProject(db, "umbrella-co", "UM")
	require.NoError(t, err)

	// ---- kb: 15 docs (5 decisions, 5 facts, 5 notes) ----

	kbEntries := make([]any, 0, 15)
	for i := 1; i <= 5; i++ {
		kbEntries = append(kbEntries, map[string]any{
			"type": "decision", "project": "acme-corp", "category": "architecture",
			"title":   "Acme decision " + strconv.Itoa(i),
			"content": "Decision content " + strconv.Itoa(i) + ": " + strings.TrimSpace(strings.Repeat("lorem ipsum dolor sit amet ", 8)),
		})
	}
	// Facts 1-3 [[Title]]-autolink back to decisions 1-3 (same project,
	// already created earlier in this batch, so AutolinkKB's same-project
	// title lookup resolves them): this gives the KB docs read by the
	// ids=[1..15] stress shapes real "explains" edges, so verbose=true's
	// edges sidecar measures a genuinely non-empty payload instead of a
	// hollow one (see OU-211 review finding).
	for i := 1; i <= 5; i++ {
		content := "Fact content " + strconv.Itoa(i) + ": " + strings.TrimSpace(strings.Repeat("key=value configuration entry ", 6))
		if i <= 3 {
			content += " See [[Acme decision " + strconv.Itoa(i) + "]]."
		}
		kbEntries = append(kbEntries, map[string]any{
			"type": "fact", "project": "acme-corp", "category": "config",
			"title":   "Acme fact " + strconv.Itoa(i),
			"content": content,
		})
	}
	// Notes 2-3 likewise [[Title]]-autolink back to note 1 (same project,
	// umbrella-co), giving that project's docs real edges too.
	for i := 1; i <= 5; i++ {
		content := "Note content " + strconv.Itoa(i) + ": " + strings.TrimSpace(strings.Repeat("operational detail for umbrella ", 6))
		if i == 2 || i == 3 {
			content += " See [[Umbrella note 1]]."
		}
		kbEntries = append(kbEntries, map[string]any{
			"type": "note", "project": "umbrella-co", "category": "ops",
			"title":   "Umbrella note " + strconv.Itoa(i),
			"content": content,
		})
	}

	kbRes, err := h.CallTool(ctx, "kb", map[string]any{"entries": kbEntries})
	require.NoError(t, err)
	require.False(t, kbRes.IsError, mcpx.ResultText(kbRes))

	var kbCreated []struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(mcpx.ResultText(kbRes)), &kbCreated))
	require.Len(t, kbCreated, 15)
	for _, c := range kbCreated {
		kbIDs = append(kbIDs, c.ID)
	}

	// ---- backlog: 15 items (10 acme-corp P2, 5 umbrella-co P3) ----

	itemEntries := make([]any, 0, 15)
	for i := 1; i <= 10; i++ {
		itemEntries = append(itemEntries, map[string]any{
			"project": "acme-corp", "priority": "P2",
			"title": "Acme backlog item " + strconv.Itoa(i),
		})
	}
	for i := 1; i <= 5; i++ {
		itemEntries = append(itemEntries, map[string]any{
			"project": "umbrella-co", "priority": "P3",
			"title": "Umbrella backlog item " + strconv.Itoa(i),
		})
	}

	itemRes, err := h.CallTool(ctx, "backlog", map[string]any{"entries": itemEntries})
	require.NoError(t, err)
	require.False(t, itemRes.IsError, mcpx.ResultText(itemRes))

	var itemsCreated []struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(mcpx.ResultText(itemRes)), &itemsCreated))
	require.Len(t, itemsCreated, 15)
	for i, c := range itemsCreated {
		if i < 10 {
			acmeIDs = append(acmeIDs, c.ID)
		} else {
			umbrellaIDs = append(umbrellaIDs, c.ID)
		}
	}

	// ---- roadmap: 8 acme-corp items across now/next/deferred, with
	// representative body/component/epic/kb/ticket/blocked_by fields so the
	// md/html render shapes reflect realistic (not empty) output. ----

	roadmapAdds := []map[string]any{
		{"section": "now", "title": "Ship response-shape gate", "body": strings.TrimSpace(strings.Repeat("token-budget rollout notes ", 10)), "component": "app"},
		{"section": "now", "title": "Stabilize backlog write batch", "body": strings.TrimSpace(strings.Repeat("batch write hardening notes ", 10)), "component": "backlog"},
		{"section": "next", "title": "KB semantic search wiring", "body": strings.TrimSpace(strings.Repeat("embedding integration plan ", 10)), "component": "kb", "kb": []any{1, 2}},
		{"section": "next", "title": "Roadmap render caching", "body": strings.TrimSpace(strings.Repeat("render perf notes ", 10)), "component": "roadmap", "ticket": []any{"OU-150"}},
		{"section": "deferred", "title": "Dashboard segment producer v2", "body": strings.TrimSpace(strings.Repeat("dashboard segment notes ", 10)), "component": "dashboard", "why": "waiting on schema settle"},
		{"section": "deferred", "title": "Cross-project edge audit", "body": strings.TrimSpace(strings.Repeat("edge audit notes ", 10)), "component": "edges"},
		{"section": "now", "title": "Config bootstrap overlay hardening", "body": strings.TrimSpace(strings.Repeat("bootstrap overlay notes ", 10)), "component": "config",
			"blocked_by": []any{map[string]any{"project": "umbrella-co", "ref": "UM-1", "note": "shared schema"}}},
		{"section": "next", "title": "Plugin hook latency budget", "body": strings.TrimSpace(strings.Repeat("hook latency notes ", 10)), "component": "plugin"},
	}
	for _, add := range roadmapAdds {
		args := map[string]any{"op": "add", "project": "acme-corp"}
		for k, v := range add {
			args[k] = v
		}
		res, err := h.CallTool(ctx, "roadmap", args)
		require.NoError(t, err)
		require.False(t, res.IsError, mcpx.ResultText(res))
	}

	return kbIDs, acmeIDs, umbrellaIDs
}

func toAnyInt64(ids []int64) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}

func toAnyString(ids []string) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}

// TestResponseShapesFootprint is the Go/shesha-testkit replacement for the
// OU-77 python bench (bench/response-shapes/probe.py, removed at OU-211):
// it drives each representative tool-call shape over the real MCP wire
// (buildServerV2 + testkit.New, exactly as serverv2_wire_test.go does),
// tokenizes the RESPONSE payload text a caller actually pays for
// (cl100k_base, same tokenize helper as TestToolsListFootprintV2), and logs
// a per-shape table -- then GATES on responseShapeCeiling per shape so a
// response-shape regression fails CI instead of only showing up in an
// optional, easy-to-ignore bench report.
func TestResponseShapesFootprint(t *testing.T) {
	resetAllDB(t)

	enc, err := tiktoken.GetEncoding("cl100k_base")
	require.NoError(t, err)

	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)
	ctx := context.Background()

	kbIDs, acmeIDs, umbrellaIDs := seedResponseShapeFixtures(t, h)

	backlogUpdateEntries := make([]any, 0, 10)
	for _, id := range acmeIDs {
		backlogUpdateEntries = append(backlogUpdateEntries, map[string]any{"id": id, "status": "open"})
	}

	kbCreateEntries := make([]any, 0, 10)
	for i := 1; i <= 10; i++ {
		kbCreateEntries = append(kbCreateEntries, map[string]any{
			"type": "note", "project": "acme-corp", "category": "bench",
			"title": "Bench probe note " + strconv.Itoa(i), "content": "Probe note for put-shape measurement.",
		})
	}

	shapes := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"get kb filter: 5 decisions", "query", map[string]any{
			"domain": "kb", "projects": []any{"acme-corp"}, "types": []any{"decision"},
		}},
		{"get kb filter: all 10 acme docs", "query", map[string]any{
			"domain": "kb", "projects": []any{"acme-corp"},
		}},
		{"get kb ids=[1] single", "query", map[string]any{
			"domain": "kb", "ids": []any{kbIDs[0]},
		}},
		{"search kb: 6 hits", "query", map[string]any{
			"domain": "kb", "query": "Acme", "limit": 6,
		}},
		{"get backlog filter: 8 items", "query", map[string]any{
			"domain": "backlog", "projects": []any{"acme-corp"}, "limit": 8,
		}},
		{"get backlog filter v=true: 8 items", "query", map[string]any{
			"domain": "backlog", "projects": []any{"acme-corp"}, "limit": 8, "verbose": true,
		}},
		{"get backlog ids=[5]", "query", map[string]any{
			"domain": "backlog", "ids": toAnyString(acmeIDs[:5]),
		}},
		{"kb write entries=[10 new docs]", "kb", map[string]any{
			"entries": kbCreateEntries,
		}},
		{"backlog write entries=[10 updates]", "backlog", map[string]any{
			"entries": backlogUpdateEntries,
		}},
		{"stress: get kb ids=[1..15] v=false", "query", map[string]any{
			"domain": "kb", "ids": toAnyInt64(kbIDs),
		}},
		{"stress: get kb ids=[1..15] v=true", "query", map[string]any{
			"domain": "kb", "ids": toAnyInt64(kbIDs), "verbose": true,
		}},
		{"stress: get backlog ids=[1..15]", "query", map[string]any{
			"domain": "backlog", "ids": toAnyString(append(append([]string{}, acmeIDs...), umbrellaIDs...)),
		}},
		{"get roadmap format=md", "query", map[string]any{
			"domain": "roadmap", "projects": []any{"acme-corp"}, "format": "md",
		}},
		{"get roadmap format=html", "query", map[string]any{
			"domain": "roadmap", "projects": []any{"acme-corp"}, "format": "html",
		}},
	}

	results := make([]shapeMeasurement, 0, len(shapes))
	for _, s := range shapes {
		res, err := h.CallTool(ctx, s.tool, s.args)
		require.NoError(t, err, s.name)
		require.False(t, res.IsError, "%s: %s", s.name, mcpx.ResultText(res))

		text := mcpx.ResultText(res)
		b, toks := tokenize(t, enc, text)
		results = append(results, shapeMeasurement{name: s.name, bytes: b, tokens: toks})
	}

	sort.Slice(results, func(i, j int) bool { return results[i].tokens > results[j].tokens })

	t.Logf("%-38s %8s %8s", "shape", "bytes", "tokens")
	for _, r := range results {
		t.Logf("%-38s %8d %8d", r.name, r.bytes, r.tokens)
	}

	for _, r := range results {
		require.Less(t, r.tokens, responseShapeCeiling,
			"shape %q exceeds the %d-token response-shape ceiling (see responseShapeCeiling's comment)", r.name, responseShapeCeiling)
	}
}
