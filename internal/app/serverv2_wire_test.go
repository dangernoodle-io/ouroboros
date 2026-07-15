package app

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/dangernoodle-io/mcpkit/mcpx"
	"github.com/dangernoodle-io/mcpkit/testkit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/kb"
	"dangernoodle.io/ouroboros/internal/query"
	"dangernoodle.io/ouroboros/internal/roadmap"
)

// TestWireQueryV2_OmittedDomain_ReturnsVerbatimMessage is the regression
// guard a direct handler call can't provide: it drives buildServerV2's
// "query" tool over an in-process mcpkit/testkit client (mcpx.InMemoryPair
// under the hood — the same in-process wiring mcpkit's own tests use),
// omitting the "domain" key from the call args entirely (fetch/filter
// mode). If Domain were schema-required (a bare `json:"domain"` field, no
// omitempty), go-sdk would reject this call before handleQueryV2 ever runs,
// with a generic schema-validation error — not the verbatim
// domain-required message. This proves the omitempty tag + explicit
// handler check (see queryInputV2.Domain's comment) preserve wire parity
// for the omitted-key case.
func TestWireQueryV2_OmittedDomain_ReturnsVerbatimMessage(t *testing.T) {
	resetDB(t)

	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)
	res, err := h.CallTool(context.Background(), "query", map[string]any{})
	require.NoError(t, err, "an omitted domain key must not fail schema validation")
	require.True(t, res.IsError)
	assert.Equal(t, query.ErrDomainRequired, mcpx.ResultText(res))
}

// TestWireQueryV2_OmittedDomain_SearchMode_ReturnsVerbatimMessage is the
// same omitted-domain guard for search mode (query present).
func TestWireQueryV2_OmittedDomain_SearchMode_ReturnsVerbatimMessage(t *testing.T) {
	resetDB(t)

	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)
	res, err := h.CallTool(context.Background(), "query", map[string]any{"query": "widget"})
	require.NoError(t, err, "an omitted domain key must not fail schema validation")
	require.True(t, res.IsError)
	assert.Equal(t, query.ErrDomainRequired, mcpx.ResultText(res))
}

// TestWireKBV2_OmittedEntries_ReturnsVerbatimMessage is kb's counterpart to
// TestWireGetV2_OmittedDomain_ReturnsVerbatimMessage: entries is non-required
// (see kbInput.Entries's comment) so an omitted entries key reaches
// handleKBV2's own check instead of failing go-sdk schema validation,
// preserving the verbatim empty-entries message over the wire.
func TestWireKBV2_OmittedEntries_ReturnsVerbatimMessage(t *testing.T) {
	resetDB(t)

	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)
	res, err := h.CallTool(context.Background(), "kb", map[string]any{})
	require.NoError(t, err, "an omitted entries key must not fail schema validation")
	require.True(t, res.IsError)
	assert.Equal(t, errKBEntriesRequired, mcpx.ResultText(res))
}

// TestWireKBV2_Create_RoundTripsThroughRealCodec is a regression guard the
// direct handler tests (handlers_kb_v2_test.go) can't provide: those
// construct kbInput directly, bypassing go-sdk's JSON decode + schema
// validation entirely. This drives the "kb" tool over the same in-process
// testkit wiring as the other Wire* tests with a raw map[string]any (as a
// real client would send), proving a create entry -- and in particular its
// id-absent kbEntryInput.ID `any` field -- actually decodes and validates
// against the generated schema instead of being rejected before handleKBV2
// ever runs (see kbEntryInput.ID's comment on why ID is `any`).
func TestWireKBV2_Create_RoundTripsThroughRealCodec(t *testing.T) {
	resetDB(t)

	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)
	res, err := h.CallTool(context.Background(), "kb", map[string]any{
		"entries": []any{
			map[string]any{
				"type":    "note",
				"project": "acme-corp",
				"title":   "wire create t",
				"content": "wire create c",
			},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	var resp []kb.PutResult
	require.NoError(t, json.Unmarshal([]byte(mcpx.ResultText(res)), &resp))
	require.Len(t, resp, 1)
	assert.Equal(t, "created", resp[0].Action)
	assert.NotZero(t, resp[0].ID)
}

// TestWireKBV2_UpdateByNumericID_RoundTripsThroughRealCodec proves a JSON
// number id -- kbEntryInput.ID's dual-type case -- passes go-sdk's
// generated schema and reaches parseKBIDV2 unmolested, over the real wire
// codec (not a direct kbInput{ID: float64(...)} construction).
func TestWireKBV2_UpdateByNumericID_RoundTripsThroughRealCodec(t *testing.T) {
	resetDB(t)

	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)

	createRes, err := h.CallTool(context.Background(), "kb", map[string]any{
		"entries": []any{
			map[string]any{
				"type":    "note",
				"project": "acme-corp",
				"title":   "wire update orig",
				"content": "wire update c",
			},
		},
	})
	require.NoError(t, err)
	require.False(t, createRes.IsError)
	var createResp []kb.PutResult
	require.NoError(t, json.Unmarshal([]byte(mcpx.ResultText(createRes)), &createResp))
	id := createResp[0].ID

	updateRes, err := h.CallTool(context.Background(), "kb", map[string]any{
		"entries": []any{
			map[string]any{
				"id":    id,
				"title": "wire update renamed",
			},
		},
	})
	require.NoError(t, err)
	require.False(t, updateRes.IsError)

	var updateResp []kb.PutResult
	require.NoError(t, json.Unmarshal([]byte(mcpx.ResultText(updateRes)), &updateResp))
	require.Len(t, updateResp, 1)
	assert.Equal(t, "updated", updateResp[0].Action)
	assert.Equal(t, "wire update renamed", updateResp[0].Title)
}

// TestWireKBV2_UpdateByStringID_RoundTripsThroughRealCodec locks the
// dual-type id decode's other case (a numeric string) over the real wire
// codec, mirroring TestWireKBV2_UpdateByNumericID_RoundTripsThroughRealCodec.
func TestWireKBV2_UpdateByStringID_RoundTripsThroughRealCodec(t *testing.T) {
	resetDB(t)

	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)

	createRes, err := h.CallTool(context.Background(), "kb", map[string]any{
		"entries": []any{
			map[string]any{
				"type":    "note",
				"project": "acme-corp",
				"title":   "wire string-id orig",
				"content": "wire string-id c",
			},
		},
	})
	require.NoError(t, err)
	require.False(t, createRes.IsError)
	var createResp []kb.PutResult
	require.NoError(t, json.Unmarshal([]byte(mcpx.ResultText(createRes)), &createResp))
	id := createResp[0].ID

	updateRes, err := h.CallTool(context.Background(), "kb", map[string]any{
		"entries": []any{
			map[string]any{
				"id":    strconv.FormatInt(id, 10),
				"title": "wire string-id renamed",
			},
		},
	})
	require.NoError(t, err)
	require.False(t, updateRes.IsError)

	var updateResp []kb.PutResult
	require.NoError(t, json.Unmarshal([]byte(mcpx.ResultText(updateRes)), &updateResp))
	require.Len(t, updateResp, 1)
	assert.Equal(t, "updated", updateResp[0].Action)
	assert.Equal(t, "wire string-id renamed", updateResp[0].Title)
}

// TestWireBacklogV2_BothOmitted_ReturnsVerbatimMessage is backlog's
// counterpart to TestWireKBV2_OmittedEntries_ReturnsVerbatimMessage: both
// entries and delete_ids are non-required (see backlogInput's comment) so
// omitting both reaches handleBacklogV2's own check instead of failing
// go-sdk schema validation, preserving the verbatim "one of" message over
// the wire.
func TestWireBacklogV2_BothOmitted_ReturnsVerbatimMessage(t *testing.T) {
	resetAllDB(t)

	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)
	res, err := h.CallTool(context.Background(), "backlog", map[string]any{})
	require.NoError(t, err, "omitting both entries and delete_ids must not fail schema validation")
	require.True(t, res.IsError)
	assert.Equal(t, errBacklogEntriesOrDeleteIDsRequired, mcpx.ResultText(res))
}

// TestWireBacklogV2_Create_RoundTripsThroughRealCodec is a regression guard
// the direct handler tests (handlers_backlog_v2_test.go) can't provide: this
// drives the "backlog" tool over the real in-process testkit wiring with a
// raw map[string]any (as a real client would send), proving a create entry
// actually decodes and validates against the generated schema.
func TestWireBacklogV2_Create_RoundTripsThroughRealCodec(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)
	res, err := h.CallTool(context.Background(), "backlog", map[string]any{
		"entries": []any{
			map[string]any{
				"project":  "acme-corp",
				"priority": "P1",
				"title":    "wire create task",
			},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	var resp []backlogResult
	require.NoError(t, json.Unmarshal([]byte(mcpx.ResultText(res)), &resp))
	require.Len(t, resp, 1)
	assert.Equal(t, "create", resp[0].Action)
	assert.Equal(t, "AC-1", resp[0].ID)
}

// TestWireBacklogV2_EpicBackrefSameBatchCreate_RoundTripsThroughRealCodec
// proves a "$N" epic back-reference resolving to a same-batch create round-
// trips over the real wire codec, not just a direct backlogInput
// construction (handlers_backlog_v2_test.go's
// TestHandleBacklogV2BatchEpicBackref_SameBatchCreate).
func TestWireBacklogV2_EpicBackrefSameBatchCreate_RoundTripsThroughRealCodec(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)
	res, err := h.CallTool(context.Background(), "backlog", map[string]any{
		"entries": []any{
			map[string]any{
				"project":  "acme-corp",
				"priority": "P1",
				"title":    "EPIC: wire X",
			},
			map[string]any{
				"project":  "acme-corp",
				"priority": "P2",
				"title":    "wire child",
				"epic":     "$0",
			},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	var resp []backlogResult
	require.NoError(t, json.Unmarshal([]byte(mcpx.ResultText(res)), &resp))
	require.Len(t, resp, 2)

	child, err := backlog.GetItem(db, resp[1].ID)
	require.NoError(t, err)
	assert.Equal(t, resp[0].ID, child.Epic)
}

// TestWireBacklogV2_EdgeMissingLabel_ReturnsVerbatimMessage proves
// edgeInput.Label/Target's omitempty tags (see edgeInput's comment) restore
// wire parity: an edges[] element with the "label" key ABSENT decodes to ""
// instead of failing go-sdk schema validation, reaching
// buildEdgeSpecsV2's existing empty-label/target check over the real wire
// codec.
func TestWireBacklogV2_EdgeMissingLabel_ReturnsVerbatimMessage(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)
	res, err := h.CallTool(context.Background(), "backlog", map[string]any{
		"entries": []any{
			map[string]any{
				"project":  "acme-corp",
				"priority": "P1",
				"title":    "wire edge missing label",
				"edges": []any{
					map[string]any{"target": "AC-1"},
				},
			},
		},
	})
	require.NoError(t, err, "an edges[] element missing the label key must not fail schema validation")
	require.True(t, res.IsError)
	assert.Equal(t, "edges[] entry requires label and target", mcpx.ResultText(res))
}

// TestWireBacklogV2_EdgeInvalidLabel_ReturnsVerbatimMessage proves an
// edges[] element with a present-but-unrecognized label still yields the
// verbatim invalid-label message over the real wire codec.
func TestWireBacklogV2_EdgeInvalidLabel_ReturnsVerbatimMessage(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)
	res, err := h.CallTool(context.Background(), "backlog", map[string]any{
		"entries": []any{
			map[string]any{
				"project":  "acme-corp",
				"priority": "P1",
				"title":    "wire edge bad label",
				"edges": []any{
					map[string]any{"label": "bogus", "target": "AC-1"},
				},
			},
		},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	assert.Equal(t, `invalid edge label "bogus": must be one of blocks, relates, explains`, mcpx.ResultText(res))
}

// TestWireBacklogV2_ComponentNotScalar_Errors documents+guards the accepted
// divergence (see backlogEntryInput's ACCEPTED DIVERGENCE comment): a
// non-scalar component (a JSON list) over the real wire codec still errors,
// just with a different (go-sdk decode/schema) message than the old
// verbatim "must be a single string value" text -- intentionally NOT
// asserted here, only that the call errors.
func TestWireBacklogV2_ComponentNotScalar_Errors(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)
	res, err := h.CallTool(context.Background(), "backlog", map[string]any{
		"entries": []any{
			map[string]any{
				"project":   "acme-corp",
				"priority":  "P3",
				"title":     "t",
				"component": []any{"a", "b"},
			},
		},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	assert.NotEmpty(t, mcpx.ResultText(res))
}

// TestWireRoadmapV2_OmittedOp_ReturnsVerbatimMessage proves op's omitempty
// tag (see roadmapInput's comment) restores wire parity: an omitted op key
// reaches handleRoadmapV2's own switch-default check instead of failing
// go-sdk schema validation, preserving the verbatim invalid-op message over
// the wire.
func TestWireRoadmapV2_OmittedOp_ReturnsVerbatimMessage(t *testing.T) {
	resetDB(t)

	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)
	res, err := h.CallTool(context.Background(), "roadmap", map[string]any{"project": "acme-corp"})
	require.NoError(t, err, "an omitted op key must not fail schema validation")
	require.True(t, res.IsError)
	assert.Equal(t, errRoadmapOpRequired, mcpx.ResultText(res))
}

// TestWireRoadmapV2_OmittedProject_ReturnsVerbatimMessage is op's counterpart:
// project is checked FIRST (matching handleRoadmap's dispatch order), so an
// omitted project key must still return the verbatim project-required
// message over the wire even with op present.
func TestWireRoadmapV2_OmittedProject_ReturnsVerbatimMessage(t *testing.T) {
	resetDB(t)

	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)
	res, err := h.CallTool(context.Background(), "roadmap", map[string]any{"op": "add", "section": "now", "title": "x"})
	require.NoError(t, err, "an omitted project key must not fail schema validation")
	require.True(t, res.IsError)
	assert.Equal(t, errRoadmapProjectRequired, mcpx.ResultText(res))
}

// TestWireRoadmapV2_Add_RoundTripsThroughRealCodec is a regression guard the
// direct handler tests (handlers_roadmap_v2_test.go) can't provide: drives
// the "roadmap" tool over the real in-process testkit wiring with a raw
// map[string]any, proving op=add decodes and validates against the
// generated schema and returns the expected {id,section} result.
func TestWireRoadmapV2_Add_RoundTripsThroughRealCodec(t *testing.T) {
	resetDB(t)

	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)
	res, err := h.CallTool(context.Background(), "roadmap", map[string]any{
		"op": "add", "project": "acme-corp", "section": "now", "title": "wire add item",
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	var resp map[string]any
	require.NoError(t, json.Unmarshal([]byte(mcpx.ResultText(res)), &resp))
	assert.Equal(t, float64(1), resp["id"])
	assert.Equal(t, "now", resp["section"])
}

// TestWireRoadmapV2_UpdatePresentEmpty_ClearsField proves the presence-vs-
// empty pointer round-trip works over the REAL wire codec (not just direct
// roadmapInput construction): an update entry with body="" (present, empty)
// clears the field, distinguishing it from an update that omits body
// entirely (which leaves it untouched) -- this is the build spec's
// mandated update-clear wire test.
func TestWireRoadmapV2_UpdatePresentEmpty_ClearsField(t *testing.T) {
	resetDB(t)

	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)

	addRes, err := h.CallTool(context.Background(), "roadmap", map[string]any{
		"op": "add", "project": "acme-corp", "section": "now", "title": "orig", "body": "orig body",
	})
	require.NoError(t, err)
	require.False(t, addRes.IsError)

	// Present-but-empty body clears; omitted title leaves it untouched.
	updRes, err := h.CallTool(context.Background(), "roadmap", map[string]any{
		"op": "update", "project": "acme-corp", "id": 1, "body": "",
	})
	require.NoError(t, err)
	require.False(t, updRes.IsError)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 1)
	assert.Equal(t, "orig", rm.Sections.Now[0].Title, "title untouched — omitted from the update call")
	assert.Empty(t, rm.Sections.Now[0].Body, "body cleared — present-but-empty in the update call")
}

// TestWireRoadmapV2_UpdateEmptyArray_ClearsKB proves the SUPPORTED clear
// path for slice-typed update fields: an update with kb: [] (present,
// empty array) decodes to a non-nil pointer to an empty slice, clearing
// the item's kb over the real wire codec -- the flip side of
// TestWireRoadmapV2_UpdateNullKB_LeavesUnchanged below (see roadmapInput's
// ACCEPTED DIVERGENCE comment on explicit null vs empty array).
func TestWireRoadmapV2_UpdateEmptyArray_ClearsKB(t *testing.T) {
	resetDB(t)

	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)

	addRes, err := h.CallTool(context.Background(), "roadmap", map[string]any{
		"op": "add", "project": "acme-corp", "section": "now", "title": "wire kb item",
		"kb": []any{1, 2},
	})
	require.NoError(t, err)
	require.False(t, addRes.IsError)

	updRes, err := h.CallTool(context.Background(), "roadmap", map[string]any{
		"op": "update", "project": "acme-corp", "id": 1, "kb": []any{},
	})
	require.NoError(t, err)
	require.False(t, updRes.IsError)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 1)
	assert.Empty(t, rm.Sections.Now[0].KB, "kb: [] (present, empty) clears the field")
}

// TestWireRoadmapV2_UpdateNullKB_LeavesUnchanged documents+guards the
// ACCEPTED DIVERGENCE on roadmapInput (explicit JSON null on a slice
// update field): an update with kb: null decodes *[]int to a nil pointer
// -- indistinguishable from the key being absent -- so the field is left
// UNTOUCHED here, whereas the old handler cleared it on an explicit null
// (see roadmapInput's comment for the full mechanism). This is a no-op,
// not an error.
func TestWireRoadmapV2_UpdateNullKB_LeavesUnchanged(t *testing.T) {
	resetDB(t)

	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)

	addRes, err := h.CallTool(context.Background(), "roadmap", map[string]any{
		"op": "add", "project": "acme-corp", "section": "now", "title": "wire kb null item",
		"kb": []any{3, 4},
	})
	require.NoError(t, err)
	require.False(t, addRes.IsError)

	updRes, err := h.CallTool(context.Background(), "roadmap", map[string]any{
		"op": "update", "project": "acme-corp", "id": 1, "kb": nil,
	})
	require.NoError(t, err)
	require.False(t, updRes.IsError)

	rm, err := roadmap.Load(db, "acme-corp")
	require.NoError(t, err)
	require.Len(t, rm.Sections.Now, 1)
	assert.Equal(t, []int{3, 4}, rm.Sections.Now[0].KB, "kb: null is a no-op here (divergence from the old handler, which cleared it)")
}

// TestWireV2_ServerInstructions_AdvertisedAtInitialize proves buildServerV2
// advertises serverInstructions verbatim over the wire. testkit.Harness
// doesn't expose the underlying ClientSession's InitializeResult, so this
// connects an in-process client directly against the *mcpkit.App (the same
// mcpx.InMemoryPair + mcpx.NewClient pattern mcpkit's own mcpx_test.go uses
// for TestServerInstructions), reading Instructions off
// ClientSession.InitializeResult() -- the sole wire-level path mcpx exposes.
func TestWireV2_ServerInstructions_AdvertisedAtInitialize(t *testing.T) {
	resetDB(t)

	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)

	serverT, clientT := mcpx.InMemoryPair()

	ctx := context.Background()
	srvSess, err := app.Connect(ctx, serverT)
	require.NoError(t, err)
	t.Cleanup(func() { _ = srvSess.Close() })

	client := mcpx.NewClient(mcpx.Implementation{Name: "instructions-test-client", Version: "0.0.0"}, nil)
	clientSess, err := client.Connect(ctx, clientT)
	require.NoError(t, err)
	t.Cleanup(func() { _ = clientSess.Close() })

	res := clientSess.InitializeResult()
	require.NotNil(t, res)
	assert.Equal(t, serverInstructions, res.Instructions)
}

// TestWireV2_ToolAnnotations_MatchOldServerMapping proves tools/list carries
// the exact annotation mapping buildServer's toolAnnotation(...) calls set
// (server.go): query ReadOnlyHint, kb IdempotentHint,
// backlog/roadmap DestructiveHint.
func TestWireV2_ToolAnnotations_MatchOldServerMapping(t *testing.T) {
	resetDB(t)

	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)
	res, err := h.ListTools(context.Background())
	require.NoError(t, err)

	byName := make(map[string]*mcpx.Tool, len(res.Tools))
	for _, tool := range res.Tools {
		byName[tool.Name] = tool
	}

	for _, name := range []string{"query"} {
		require.NotNil(t, byName[name].Annotations, "%s tool missing annotations", name)
		assert.True(t, byName[name].Annotations.ReadOnlyHint, "%s tool should carry ReadOnlyHint", name)
	}

	require.NotNil(t, byName["kb"].Annotations)
	assert.True(t, byName["kb"].Annotations.IdempotentHint, "kb tool should carry IdempotentHint")

	for _, name := range []string{"backlog", "roadmap"} {
		require.NotNil(t, byName[name].Annotations, "%s tool missing annotations", name)
		require.NotNil(t, byName[name].Annotations.DestructiveHint, "%s tool should carry DestructiveHint", name)
		assert.True(t, *byName[name].Annotations.DestructiveHint, "%s tool's DestructiveHint should be true", name)
	}
}

// TestWireRoadmapV2_ComponentNotScalar_Errors documents+guards the accepted
// divergence (see roadmapInput's comment): a non-scalar component (a JSON
// list) over the real wire codec still errors, just with a different
// (go-sdk decode/schema) message than a hypothetical verbatim "single
// string value" text -- intentionally NOT asserted here, only that the
// call errors, mirroring TestWireBacklogV2_ComponentNotScalar_Errors.
func TestWireRoadmapV2_ComponentNotScalar_Errors(t *testing.T) {
	resetDB(t)

	app, err := buildServerV2(&serverState{db: db}, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)
	res, err := h.CallTool(context.Background(), "roadmap", map[string]any{
		"op": "add", "project": "acme-corp", "section": "now", "title": "t",
		"component": []any{"a", "b"},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	assert.NotEmpty(t, mcpx.ResultText(res))
}
