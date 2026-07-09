package app

import (
	"context"
	"strconv"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/edges"
)

// TestHandleBacklogCreateWithInlineEdges verifies entries[].edges[] creates
// the edge alongside item creation (the primary inline surface).
func TestHandleBacklogCreateWithInlineEdges(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	blocker, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "blocker", "", "", "", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "P1",
				"title":    "blocked item",
				"edges": []interface{}{
					map[string]interface{}{"label": "blocks", "target": blocker.ID},
				},
			},
		},
	})
	result, err := handleBacklog(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	list, err := edges.EdgesFor(db, edges.TypeItem, "AC-2")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "AC-2", list[0].SourceID)
	assert.Equal(t, blocker.ID, list[0].TargetID)
	assert.Equal(t, "blocks", list[0].Label)
}

// TestHandleBacklogUpdateWithInlineEdges verifies entries[].edges[] on an
// update-mode entry (id present, no other field changes) still links.
func TestHandleBacklogUpdateWithInlineEdges(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	a, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item a", "", "", "", "")
	require.NoError(t, err)
	b, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item b", "", "", "", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"id": a.ID,
				"edges": []interface{}{
					map[string]interface{}{"label": "relates", "target": b.ID},
				},
			},
		},
	})
	result, err := handleBacklog(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	list, err := edges.EdgesFor(db, edges.TypeItem, a.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "relates", list[0].Label)
}

func TestParseEdgeSpecsNoEdgesKey(t *testing.T) {
	specs, err := parseEdgeSpecs(map[string]interface{}{"title": "x"})
	require.NoError(t, err)
	assert.Nil(t, specs)
}

func TestParseEdgeSpecsMalformedElement(t *testing.T) {
	_, err := parseEdgeSpecs(map[string]interface{}{"edges": []interface{}{"not-an-object"}})
	assert.Error(t, err)
}

func TestParseEdgeSpecsMissingLabelOrTarget(t *testing.T) {
	_, err := parseEdgeSpecs(map[string]interface{}{
		"edges": []interface{}{map[string]interface{}{"label": "blocks"}},
	})
	assert.Error(t, err)

	_, err = parseEdgeSpecs(map[string]interface{}{
		"edges": []interface{}{map[string]interface{}{"target": "AC-1"}},
	})
	assert.Error(t, err)
}

// TestHandleBacklogInlineEdgeInvalidLabel_Errors verifies a bad label in
// edges[] is a hard error, not a silent skip.
func TestHandleBacklogInlineEdgeInvalidLabel_Errors(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "P1",
				"title":    "x",
				"edges": []interface{}{
					map[string]interface{}{"label": "part-of", "target": "AC-1"},
				},
			},
		},
	})
	result, err := handleBacklog(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "invalid edge label")
}

// TestHandleBacklogInlineEdgeMissingTarget_Errors verifies a typo'd/missing
// edges[].target is a hard error, not a silently dangling edge, and that
// the whole write (including the item itself) rolls back — proving the
// item write and its edge links are atomic.
func TestHandleBacklogInlineEdgeMissingTarget_Errors(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"project":  "acme-corp",
				"priority": "P1",
				"title":    "blocked item",
				"edges": []interface{}{
					map[string]interface{}{"label": "blocks", "target": "TM-999"},
				},
			},
		},
	})
	result, err := handleBacklog(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.True(t, result.IsError)
	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "not found")

	// The item itself must not have been created — atomic rollback.
	items, err := backlog.ListItems(db, backlog.ItemFilter{})
	require.NoError(t, err)
	assert.Empty(t, items, "a failing edges[] target must roll back the whole entry, including the item create")
}

// TestHandleBacklogUpdateInlineEdgeMissingTarget_Errors mirrors the create
// case for update-mode: a missing target rolls back the field update too.
func TestHandleBacklogUpdateInlineEdgeMissingTarget_Errors(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	a, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item a", "", "", "", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"id":       a.ID,
				"priority": "P0",
				"edges": []interface{}{
					map[string]interface{}{"label": "blocks", "target": "TM-999"},
				},
			},
		},
	})
	result, err := handleBacklog(db, nil)(context.TODO(), req)
	require.NoError(t, err)
	require.True(t, result.IsError)

	// The priority update must not have survived the rolled-back tx.
	unchanged, err := backlog.GetItem(db, a.ID)
	require.NoError(t, err)
	assert.Equal(t, "P1", unchanged.Priority, "a failing edges[] target must roll back the field update too")
}

// TestGetBacklogVerboseIncludesEdgesSidecar verifies get domain=backlog
// verbose=true surfaces an edges array; verbose=false omits it.
func TestGetBacklogVerboseIncludesEdgesSidecar(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	a, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item a", "", "", "", "")
	require.NoError(t, err)
	b, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "item b", "", "", "", "")
	require.NoError(t, err)
	_, err = edges.Link(db, edges.TypeItem, a.ID, "blocks", edges.TypeItem, b.ID, proj.ID)
	require.NoError(t, err)

	verboseReq := makeRequest(map[string]interface{}{
		"domain":  "backlog",
		"ids":     []interface{}{a.ID},
		"verbose": true,
	})
	result, err := getBacklogItems(db, verboseReq)
	require.NoError(t, err)
	var verboseResp []map[string]interface{}
	require.NoError(t, unmarshalResult(result, &verboseResp))
	require.Len(t, verboseResp, 1)
	edgesField, ok := verboseResp[0]["edges"].([]interface{})
	require.True(t, ok, "verbose=true response missing edges sidecar")
	require.Len(t, edgesField, 1)

	compactReq := makeRequest(map[string]interface{}{
		"domain": "backlog",
		"ids":    []interface{}{a.ID},
	})
	result, err = getBacklogItems(db, compactReq)
	require.NoError(t, err)
	var compactResp []map[string]interface{}
	require.NoError(t, unmarshalResult(result, &compactResp))
	require.Len(t, compactResp, 1)
	_, present := compactResp[0]["edges"]
	assert.False(t, present, "verbose=false response must not include an edges sidecar")
}

// TestHandleKBAutolinksKBTitle verifies the kb write tool autolinks a
// [[Title]] reference in content into an explains edge.
func TestHandleKBAutolinksKBTitle(t *testing.T) {
	resetDB(t)

	targetReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"type": "note", "project": "acme-corp", "title": "Target Doc", "content": "target",
			},
		},
	})
	_, err := handleKB(db)(context.TODO(), targetReq)
	require.NoError(t, err)

	sourceReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"type": "note", "project": "acme-corp", "title": "Source Doc", "content": "see [[Target Doc]]",
			},
		},
	})
	result, err := handleKB(db)(context.TODO(), sourceReq)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var resp []map[string]interface{}
	require.NoError(t, unmarshalResult(result, &resp))
	require.Len(t, resp, 1)
	idFloat, ok := resp[0]["id"].(float64)
	require.True(t, ok)
	sourceID := int64(idFloat)

	list, err := edges.EdgesFor(db, edges.TypeKB, strconv.FormatInt(sourceID, 10))
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "explains", list[0].Label)
}

// TestGetKBVerboseIncludesEdgesSidecar verifies get domain=kb verbose=true
// surfaces an edges array; verbose=false omits it.
func TestGetKBVerboseIncludesEdgesSidecar(t *testing.T) {
	resetDB(t)

	writeReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"type": "note", "project": "acme-corp", "title": "Target Doc", "content": "target",
			},
		},
	})
	_, err := handleKB(db)(context.TODO(), writeReq)
	require.NoError(t, err)

	sourceReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"type": "note", "project": "acme-corp", "title": "Source Doc", "content": "see [[Target Doc]]",
			},
		},
	})
	sourceResult, err := handleKB(db)(context.TODO(), sourceReq)
	require.NoError(t, err)
	var sourceResp []map[string]interface{}
	require.NoError(t, unmarshalResult(sourceResult, &sourceResp))
	sourceID, ok := sourceResp[0]["id"].(float64)
	require.True(t, ok)

	verboseReq := makeRequest(map[string]interface{}{
		"domain":  "kb",
		"ids":     []interface{}{sourceID},
		"verbose": true,
	})
	result, err := getDocuments(db, verboseReq)
	require.NoError(t, err)
	var verboseResp []map[string]interface{}
	require.NoError(t, unmarshalResult(result, &verboseResp))
	require.Len(t, verboseResp, 1)
	edgesField, ok := verboseResp[0]["edges"].([]interface{})
	require.True(t, ok, "verbose=true response missing edges sidecar")
	require.Len(t, edgesField, 1)

	compactReq := makeRequest(map[string]interface{}{
		"domain": "kb",
		"ids":    []interface{}{sourceID},
	})
	result, err = getDocuments(db, compactReq)
	require.NoError(t, err)
	var compactResp []map[string]interface{}
	require.NoError(t, unmarshalResult(result, &compactResp))
	require.Len(t, compactResp, 1)
	_, present := compactResp[0]["edges"]
	assert.False(t, present, "verbose=false response must not include an edges sidecar")
}
