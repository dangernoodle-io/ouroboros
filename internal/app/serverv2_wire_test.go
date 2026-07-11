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
