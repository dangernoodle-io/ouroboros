package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/store"
)

func TestMain(m *testing.M) {
	var err error
	db, err = sql.Open("sqlite", ":memory:")
	if err != nil {
		panic(err)
	}
	if err = store.ApplySchema(db); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func resetDB(t *testing.T) {
	t.Helper()
	_, err := db.Exec("DELETE FROM documents")
	require.NoError(t, err)
	require.NoError(t, store.RebuildFTS(db))
}

func makeRequest(args map[string]interface{}) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
}

func TestHandlePut(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"type":     "decision",
		"project":  "acme-corp",
		"title":    "Use PostgreSQL",
		"content":  "Superior query performance for our use case",
		"category": "",
		"tags":     []interface{}{"database", "infrastructure"},
	})

	result, err := handlePut(context.TODO(), req)
	require.NoError(t, err)
	require.NotNil(t, result)

	var resp map[string]interface{}
	err = unmarshalResult(result, &resp)
	require.NoError(t, err)
	ok, okExists := resp["ok"].(bool)
	require.True(t, okExists)
	assert.True(t, ok)
	assert.NotZero(t, resp["id"])
}

func TestHandlePutUpsert(t *testing.T) {
	resetDB(t)

	// Put document first time
	req1 := makeRequest(map[string]interface{}{
		"type":     "decision",
		"project":  "acme-corp",
		"title":    "Use PostgreSQL",
		"content":  "Original rationale",
		"category": "",
	})

	result1, err := handlePut(context.TODO(), req1)
	require.NoError(t, err)
	var resp1 map[string]interface{}
	err = unmarshalResult(result1, &resp1)
	require.NoError(t, err)
	id1Float, ok := resp1["id"].(float64)
	require.True(t, ok)
	id1 := int64(id1Float)

	// Put same document with updated content
	req2 := makeRequest(map[string]interface{}{
		"type":     "decision",
		"project":  "acme-corp",
		"title":    "Use PostgreSQL",
		"content":  "Updated rationale",
		"category": "",
	})

	result2, err := handlePut(context.TODO(), req2)
	require.NoError(t, err)
	var resp2 map[string]interface{}
	err = unmarshalResult(result2, &resp2)
	require.NoError(t, err)
	id2Float, ok := resp2["id"].(float64)
	require.True(t, ok)
	id2 := int64(id2Float)

	// IDs should match (upsert)
	assert.Equal(t, id1, id2)

	// Content should be updated
	doc, err := store.GetDocument(db, id1)
	require.NoError(t, err)
	assert.Equal(t, "Updated rationale", doc.Content)
}

func TestHandleGetByID(t *testing.T) {
	resetDB(t)

	// Insert document
	doc := store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Use PostgreSQL",
		Content: "Full rationale content here",
		Tags:    []string{"database"},
	}
	id, err := store.UpsertDocument(db, doc)
	require.NoError(t, err)

	// Get by ID
	req := makeRequest(map[string]interface{}{
		"id": float64(id),
	})

	result, err := handleGet(context.TODO(), req)
	require.NoError(t, err)

	var retrieved store.Document
	err = unmarshalResult(result, &retrieved)
	require.NoError(t, err)
	assert.Equal(t, "Use PostgreSQL", retrieved.Title)
	assert.Equal(t, "Full rationale content here", retrieved.Content)
}

func TestHandleGetList(t *testing.T) {
	resetDB(t)

	// Insert multiple documents
	_, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Use PostgreSQL",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Use gRPC",
	})
	require.NoError(t, err)

	// Get list (no id)
	req := makeRequest(map[string]interface{}{
		"type":    "decision",
		"project": "acme-corp",
	})

	result, err := handleGet(context.TODO(), req)
	require.NoError(t, err)

	var summaries []store.DocumentSummary
	err = unmarshalResult(result, &summaries)
	require.NoError(t, err)
	assert.Len(t, summaries, 2)
	// DocumentSummary type has no Content field — type system enforces token conservation
	for _, s := range summaries {
		assert.NotEmpty(t, s.Title)
		assert.Equal(t, "decision", s.Type)
	}
}

func TestHandleGetByType(t *testing.T) {
	resetDB(t)

	// Insert different types
	_, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Decision 1",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type:    "fact",
		Project: "acme-corp",
		Title:   "Fact 1",
	})
	require.NoError(t, err)

	// Filter by type
	req := makeRequest(map[string]interface{}{
		"type": "decision",
	})

	result, err := handleGet(context.TODO(), req)
	require.NoError(t, err)

	var summaries []store.DocumentSummary
	err = unmarshalResult(result, &summaries)
	require.NoError(t, err)
	assert.Len(t, summaries, 1)
	assert.Equal(t, "decision", summaries[0].Type)
}

func TestHandleDelete(t *testing.T) {
	resetDB(t)

	id, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Use PostgreSQL",
	})
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"id": float64(id),
	})

	result, err := handleDelete(context.TODO(), req)
	require.NoError(t, err)

	var resp map[string]bool
	err = unmarshalResult(result, &resp)
	require.NoError(t, err)
	assert.True(t, resp["ok"])

	// Verify deletion
	doc, err := store.GetDocument(db, id)
	require.NoError(t, err)
	assert.Nil(t, doc)
}

func TestHandleSearch(t *testing.T) {
	resetDB(t)

	_, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Use PostgreSQL",
		Content: "Performance benefits",
	})
	require.NoError(t, err)
	_, err = store.UpsertDocument(db, store.Document{
		Type:    "fact",
		Project: "acme-corp",
		Title:   "Database Type",
		Content: "PostgreSQL",
	})
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"query": "PostgreSQL",
	})

	result, err := handleSearch(context.TODO(), req)
	require.NoError(t, err)

	var summaries []store.DocumentSummary
	err = unmarshalResult(result, &summaries)
	require.NoError(t, err)
	assert.Len(t, summaries, 2)
}

func TestHandleExport(t *testing.T) {
	resetDB(t)

	_, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Use PostgreSQL",
		Content: "Good performance",
		Tags:    []string{"database"},
	})
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"project": "acme-corp",
		"type":    "decision",
	})

	result, err := handleExport(context.TODO(), req)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Extract markdown text from result
	require.Len(t, result.Content, 1)
	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	markdown := textContent.Text

	assert.Contains(t, markdown, "# Knowledge Base Export")
	assert.Contains(t, markdown, "acme-corp")
	assert.Contains(t, markdown, "PostgreSQL")
}

func TestHandleImport(t *testing.T) {
	resetDB(t)

	importJSON := `{
		"documents": [
			{
				"type": "decision",
				"project": "acme-corp",
				"title": "Use PostgreSQL",
				"content": "Good performance"
			},
			{
				"type": "fact",
				"project": "acme-corp",
				"category": "hardware",
				"title": "cpu_cores",
				"content": "8"
			}
		]
	}`

	req := makeRequest(map[string]interface{}{
		"content": importJSON,
		"project": "acme-corp",
	})

	result, err := handleImport(context.TODO(), req)
	require.NoError(t, err)

	var resp map[string]bool
	err = unmarshalResult(result, &resp)
	require.NoError(t, err)
	assert.True(t, resp["ok"])

	// Verify import
	summaries, err := store.QueryDocuments(db, "decision", "acme-corp", "", "", nil, 0)
	require.NoError(t, err)
	assert.Len(t, summaries, 1)

	facts, err := store.QueryDocuments(db, "fact", "acme-corp", "", "", nil, 0)
	require.NoError(t, err)
	assert.Len(t, facts, 1)
}

func TestHandleMissingRequiredParams(t *testing.T) {
	resetDB(t)

	// Test missing type in put
	result, err := handlePut(context.TODO(), makeRequest(map[string]interface{}{"project": "p", "title": "t"}))
	require.NoError(t, err)
	assert.True(t, result.IsError)

	// Test missing project in put
	result, err = handlePut(context.TODO(), makeRequest(map[string]interface{}{"type": "decision", "title": "t"}))
	require.NoError(t, err)
	assert.True(t, result.IsError)

	// Test missing title in put
	result, err = handlePut(context.TODO(), makeRequest(map[string]interface{}{"type": "decision", "project": "p"}))
	require.NoError(t, err)
	assert.True(t, result.IsError)

	// Test missing id in delete
	result, err = handleDelete(context.TODO(), makeRequest(map[string]interface{}{}))
	require.NoError(t, err)
	assert.True(t, result.IsError)

	// Test missing query in search
	result, err = handleSearch(context.TODO(), makeRequest(map[string]interface{}{}))
	require.NoError(t, err)
	assert.True(t, result.IsError)

	// Test missing content in import
	result, err = handleImport(context.TODO(), makeRequest(map[string]interface{}{"project": "p"}))
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestParseQueryArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want queryArgs
	}{
		{
			name: "all flags",
			args: []string{"--project", "acme-corp", "--type", "decision", "--limit", "5"},
			want: queryArgs{project: "acme-corp", docType: "decision", limit: 5},
		},
		{
			name: "project only",
			args: []string{"--project", "acme-corp"},
			want: queryArgs{project: "acme-corp", limit: 10},
		},
		{
			name: "type only",
			args: []string{"--type", "fact"},
			want: queryArgs{docType: "fact", limit: 10},
		},
		{
			name: "limit only",
			args: []string{"--limit", "25"},
			want: queryArgs{limit: 25},
		},
		{
			name: "no flags",
			args: []string{},
			want: queryArgs{limit: 10},
		},
		{
			name: "invalid limit falls back to default",
			args: []string{"--limit", "abc"},
			want: queryArgs{limit: 10},
		},
		{
			name: "missing flag value at end",
			args: []string{"--project"},
			want: queryArgs{limit: 10},
		},
		{
			name: "missing flag value in middle",
			args: []string{"--project", "--type", "decision"},
			want: queryArgs{project: "--type", limit: 10},
		},
		{
			name: "duplicate flags uses last value",
			args: []string{"--project", "acme-corp", "--project", "test-co"},
			want: queryArgs{project: "test-co", limit: 10},
		},
		{
			name: "limit zero",
			args: []string{"--limit", "0"},
			want: queryArgs{limit: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseQueryArgs(tt.args)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Helper to extract text from tool result and unmarshal JSON.
func unmarshalResult(result *mcp.CallToolResult, v interface{}) error {
	if len(result.Content) == 0 {
		return json.Unmarshal([]byte("{}"), v)
	}
	// Extract text from the first content item
	textContent, ok := mcp.AsTextContent(result.Content[0])
	if !ok {
		return json.Unmarshal([]byte("{}"), v)
	}
	return json.Unmarshal([]byte(textContent.Text), v)
}
