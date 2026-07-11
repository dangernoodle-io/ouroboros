package app

import (
	"context"
	"testing"

	"github.com/dangernoodle-io/mcpkit/mcpx"
	"github.com/dangernoodle-io/mcpkit/testkit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
