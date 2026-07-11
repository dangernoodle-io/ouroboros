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
)

// TestWireGetV2_OmittedDomain_ReturnsVerbatimMessage is the regression guard
// a direct handler call can't provide: it drives buildServerV2's "get" tool
// over an in-process mcpkit/testkit client (mcpx.InMemoryPair under the
// hood — the same in-process wiring mcpkit's own tests use), omitting the
// "domain" key from the call args entirely. If Domain were schema-required
// (a bare `json:"domain"` field, no omitempty), go-sdk would reject this
// call before handleGetV2 ever runs, with a generic schema-validation
// error — not the verbatim domain-required message. This proves the
// omitempty tag + explicit handler check (see getInput.Domain's comment)
// preserve wire parity for the omitted-key case.
func TestWireGetV2_OmittedDomain_ReturnsVerbatimMessage(t *testing.T) {
	resetDB(t)

	app, err := buildServerV2(db, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)
	res, err := h.CallTool(context.Background(), "get", map[string]any{})
	require.NoError(t, err, "an omitted domain key must not fail schema validation")
	require.True(t, res.IsError)
	assert.Equal(t, errDomainRequired, mcpx.ResultText(res))
}

// TestWireSearchV2_OmittedDomain_ReturnsVerbatimMessage is search's
// counterpart to TestWireGetV2_OmittedDomain_ReturnsVerbatimMessage.
func TestWireSearchV2_OmittedDomain_ReturnsVerbatimMessage(t *testing.T) {
	resetDB(t)

	app, err := buildServerV2(db, "test")
	require.NoError(t, err)

	h := testkit.New(t, app)
	res, err := h.CallTool(context.Background(), "search", map[string]any{})
	require.NoError(t, err, "an omitted domain key must not fail schema validation")
	require.True(t, res.IsError)
	assert.Equal(t, errDomainRequired, mcpx.ResultText(res))
}

// TestWireKBV2_OmittedEntries_ReturnsVerbatimMessage is kb's counterpart to
// TestWireGetV2_OmittedDomain_ReturnsVerbatimMessage: entries is non-required
// (see kbInput.Entries's comment) so an omitted entries key reaches
// handleKBV2's own check instead of failing go-sdk schema validation,
// preserving the verbatim empty-entries message over the wire.
func TestWireKBV2_OmittedEntries_ReturnsVerbatimMessage(t *testing.T) {
	resetDB(t)

	app, err := buildServerV2(db, "test")
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

	app, err := buildServerV2(db, "test")
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

	app, err := buildServerV2(db, "test")
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

	app, err := buildServerV2(db, "test")
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

	app, err := buildServerV2(db, "test")
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

	app, err := buildServerV2(db, "test")
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

	app, err := buildServerV2(db, "test")
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

	app, err := buildServerV2(db, "test")
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

	app, err := buildServerV2(db, "test")
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

	app, err := buildServerV2(db, "test")
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
