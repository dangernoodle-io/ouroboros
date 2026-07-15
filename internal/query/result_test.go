package query

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/edges"
	"dangernoodle.io/ouroboros/internal/store"
)

// TestDocResult_JSONShape locks in the flat, wire-stable shape of DocResult
// (OU-328): the embedded *store.Document's fields are flattened (no nested
// "Document" key), and "edges" appears only when non-empty (omitempty) —
// this is the exact shape the former []any Docs union (bare *store.Document,
// or DocWithEdges) produced on both the MCP wire (jsonResultV2) and the CLI
// --json path; a typed carrier must reproduce it byte-for-byte.
func TestDocResult_JSONShape(t *testing.T) {
	doc := &store.Document{
		ID:        1,
		Type:      "decision",
		Project:   "acme-corp",
		Title:     "Use PostgreSQL",
		Content:   "content body",
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-01T00:00:00Z",
	}

	t.Run("without edges (non-verbose)", func(t *testing.T) {
		result := DocResult{Document: doc}
		data, err := json.Marshal(result)
		require.NoError(t, err)

		want, err := json.Marshal(doc)
		require.NoError(t, err)

		assert.JSONEq(t, string(want), string(data))
		assert.NotContains(t, string(data), `"edges"`)
	})

	t.Run("with edges (verbose)", func(t *testing.T) {
		edgeList := []edges.Edge{{ID: 1, SourceType: "item", SourceID: "AC-1", TargetType: "kb", TargetID: "1", Label: "relates", CreatedAt: "2026-01-01T00:00:00Z"}}
		result := DocResult{Document: doc, Edges: edgeList}
		data, err := json.Marshal(result)
		require.NoError(t, err)

		var got map[string]any
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, "Use PostgreSQL", got["title"])
		edgesOut, ok := got["edges"].([]any)
		require.True(t, ok)
		require.Len(t, edgesOut, 1)
		edge0, ok := edgesOut[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "relates", edge0["label"])
	})
}

// TestItemResult_JSONShape mirrors TestDocResult_JSONShape for the backlog
// side (OU-328): ItemResult must reproduce the former []any ItemsJSON
// union's flat item+edges shape byte-for-byte.
func TestItemResult_JSONShape(t *testing.T) {
	item := &backlog.Item{
		ID:          "AC-1",
		Priority:    "P1",
		Title:       "fix the bug",
		Description: "desc text",
		Status:      "open",
		Created:     "2026-01-01T00:00:00Z",
		Updated:     "2026-01-01T00:00:00Z",
	}

	t.Run("without edges (non-verbose)", func(t *testing.T) {
		result := ItemResult{Item: item}
		data, err := json.Marshal(result)
		require.NoError(t, err)

		want, err := json.Marshal(item)
		require.NoError(t, err)

		assert.JSONEq(t, string(want), string(data))
		assert.NotContains(t, string(data), `"edges"`)
	})

	t.Run("with edges (verbose)", func(t *testing.T) {
		edgeList := []edges.Edge{{ID: 1, SourceType: "item", SourceID: "AC-1", TargetType: "kb", TargetID: "1", Label: "relates", CreatedAt: "2026-01-01T00:00:00Z"}}
		result := ItemResult{Item: item, Edges: edgeList}
		data, err := json.Marshal(result)
		require.NoError(t, err)

		var got map[string]any
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, "fix the bug", got["title"])
		edgesOut, ok := got["edges"].([]any)
		require.True(t, ok)
		require.Len(t, edgesOut, 1)
		edge0, ok := edgesOut[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "relates", edge0["label"])
	})
}
