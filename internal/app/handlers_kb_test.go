package app

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

var db *sql.DB

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

	result, err := handlePut(db)(context.TODO(), req)
	require.NoError(t, err)
	require.NotNil(t, result)

	var resp map[string]interface{}
	err = unmarshalResult(result, &resp)
	require.NoError(t, err)
	action, actionExists := resp["action"].(string)
	require.True(t, actionExists)
	assert.Equal(t, "created", action)
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

	result1, err := handlePut(db)(context.TODO(), req1)
	require.NoError(t, err)
	var resp1 map[string]interface{}
	err = unmarshalResult(result1, &resp1)
	require.NoError(t, err)
	id1Float, ok := resp1["id"].(float64)
	require.True(t, ok)
	id1 := int64(id1Float)
	action1, ok := resp1["action"].(string)
	require.True(t, ok, "action must be a string")
	assert.Equal(t, "created", action1)

	// Put same document with updated content
	req2 := makeRequest(map[string]interface{}{
		"type":     "decision",
		"project":  "acme-corp",
		"title":    "Use PostgreSQL",
		"content":  "Updated rationale",
		"category": "",
	})

	result2, err := handlePut(db)(context.TODO(), req2)
	require.NoError(t, err)
	var resp2 map[string]interface{}
	err = unmarshalResult(result2, &resp2)
	require.NoError(t, err)
	id2Float, ok := resp2["id"].(float64)
	require.True(t, ok)
	id2 := int64(id2Float)
	action2, ok := resp2["action"].(string)
	require.True(t, ok, "action must be a string")
	assert.Equal(t, "updated", action2)

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
	upsertResult, err := store.UpsertDocument(db, doc)
	require.NoError(t, err)
	id := upsertResult.ID

	// Get by ID
	req := makeRequest(map[string]interface{}{
		"id": float64(id),
	})

	result, err := handleGet(db)(context.TODO(), req)
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
	result2, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Use gRPC",
	})
	require.NoError(t, err)
	_ = result2

	// Get list (no id)
	req := makeRequest(map[string]interface{}{
		"type":    "decision",
		"project": "acme-corp",
	})

	result, err := handleGet(db)(context.TODO(), req)
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
	result2, err := store.UpsertDocument(db, store.Document{
		Type:    "fact",
		Project: "acme-corp",
		Title:   "Fact 1",
	})
	require.NoError(t, err)
	_ = result2

	// Filter by type
	req := makeRequest(map[string]interface{}{
		"type": "decision",
	})

	result, err := handleGet(db)(context.TODO(), req)
	require.NoError(t, err)

	var summaries []store.DocumentSummary
	err = unmarshalResult(result, &summaries)
	require.NoError(t, err)
	assert.Len(t, summaries, 1)
	assert.Equal(t, "decision", summaries[0].Type)
}

func TestHandleDelete(t *testing.T) {
	resetDB(t)

	upsertResult, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Use PostgreSQL",
	})
	require.NoError(t, err)
	id := upsertResult.ID

	req := makeRequest(map[string]interface{}{
		"id": float64(id),
	})

	result, err := handleDelete(db)(context.TODO(), req)
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
	result2, err := store.UpsertDocument(db, store.Document{
		Type:    "fact",
		Project: "acme-corp",
		Title:   "Database Type",
		Content: "PostgreSQL",
	})
	require.NoError(t, err)
	_ = result2

	req := makeRequest(map[string]interface{}{
		"query": "PostgreSQL",
	})

	result, err := handleSearch(db)(context.TODO(), req)
	require.NoError(t, err)

	var summaries []store.DocumentSummary
	err = unmarshalResult(result, &summaries)
	require.NoError(t, err)
	assert.Len(t, summaries, 2)
}

func TestHandleExport(t *testing.T) {
	resetDB(t)

	result, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Use PostgreSQL",
		Content: "Good performance",
		Tags:    []string{"database"},
	})
	require.NoError(t, err)
	_ = result

	req := makeRequest(map[string]interface{}{
		"project": "acme-corp",
		"type":    "decision",
	})

	exportResult, err := handleExport(db)(context.TODO(), req)
	require.NoError(t, err)
	require.NotNil(t, exportResult)

	// Extract markdown text from result
	require.Len(t, exportResult.Content, 1)
	textContent, ok := mcp.AsTextContent(exportResult.Content[0])
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

	result, err := handleImport(db)(context.TODO(), req)
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
	result, err := handlePut(db)(context.TODO(), makeRequest(map[string]interface{}{"project": "p", "title": "t"}))
	require.NoError(t, err)
	assert.True(t, result.IsError)

	// Test missing project in put
	result, err = handlePut(db)(context.TODO(), makeRequest(map[string]interface{}{"type": "decision", "title": "t"}))
	require.NoError(t, err)
	assert.True(t, result.IsError)

	// Test missing title in put
	result, err = handlePut(db)(context.TODO(), makeRequest(map[string]interface{}{"type": "decision", "project": "p"}))
	require.NoError(t, err)
	assert.True(t, result.IsError)

	// Test missing id in delete
	result, err = handleDelete(db)(context.TODO(), makeRequest(map[string]interface{}{}))
	require.NoError(t, err)
	assert.True(t, result.IsError)

	// Test missing query in search
	result, err = handleSearch(db)(context.TODO(), makeRequest(map[string]interface{}{}))
	require.NoError(t, err)
	assert.True(t, result.IsError)

	// Test missing content in import
	result, err = handleImport(db)(context.TODO(), makeRequest(map[string]interface{}{"project": "p"}))
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestHandlePutWithNotes(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"type":    "decision",
		"project": "acme-corp",
		"title":   "Use PostgreSQL",
		"content": "Superior performance",
		"notes":   "We chose PostgreSQL because of better query optimization and advanced features.",
	})

	result, err := handlePut(db)(context.TODO(), req)
	require.NoError(t, err)
	var resp map[string]interface{}
	err = unmarshalResult(result, &resp)
	require.NoError(t, err)
	idFloat, ok := resp["id"].(float64)
	require.True(t, ok)
	id := int64(idFloat)

	// Get with verbose=true should include notes
	getReq := makeRequest(map[string]interface{}{
		"id":      float64(id),
		"verbose": true,
	})

	getResult, err := handleGet(db)(context.TODO(), getReq)
	require.NoError(t, err)
	var doc store.Document
	err = unmarshalResult(getResult, &doc)
	require.NoError(t, err)
	assert.Equal(t, "Use PostgreSQL", doc.Title)
	assert.Equal(t, "We chose PostgreSQL because of better query optimization and advanced features.", doc.Notes)
}

func TestHandleGetVerboseFalse(t *testing.T) {
	resetDB(t)

	// Insert document with notes
	result, err := store.UpsertDocument(db, store.Document{
		Type:    "decision",
		Project: "acme-corp",
		Title:   "Use PostgreSQL",
		Content: "Superior performance",
		Notes:   "Human readable rationale",
	})
	require.NoError(t, err)
	id := result.ID

	// Get with verbose=false (default) should NOT include notes
	getReq := makeRequest(map[string]interface{}{
		"id":      float64(id),
		"verbose": false,
	})

	getResult, err := handleGet(db)(context.TODO(), getReq)
	require.NoError(t, err)
	var doc store.Document
	err = unmarshalResult(getResult, &doc)
	require.NoError(t, err)
	assert.Equal(t, "Use PostgreSQL", doc.Title)
	assert.Equal(t, "", doc.Notes)
}

func TestHandlePutContentCap(t *testing.T) {
	resetDB(t)

	// Create content that exceeds 500 chars
	longContent := string(make([]byte, 501))
	for i := 0; i < 501; i++ {
		longContent = longContent[:i] + "x"
	}

	req := makeRequest(map[string]interface{}{
		"type":    "decision",
		"project": "acme-corp",
		"title":   "Test",
		"content": longContent,
	})

	result, err := handlePut(db)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestHandlePutAcceptsContentUnderCap(t *testing.T) {
	resetDB(t)

	// Content exactly at 500 chars should succeed
	content500 := string(make([]byte, 500))
	for i := 0; i < 500; i++ {
		content500 = content500[:i] + "x"
	}

	req := makeRequest(map[string]interface{}{
		"type":    "decision",
		"project": "acme-corp",
		"title":   "Test 500",
		"content": content500,
	})

	result, err := handlePut(db)(context.TODO(), req)
	require.NoError(t, err)
	assert.False(t, result.IsError, "content of exactly 500 chars should succeed")

	var resp map[string]interface{}
	err = unmarshalResult(result, &resp)
	require.NoError(t, err)
	action, ok := resp["action"].(string)
	require.True(t, ok)
	assert.Equal(t, "created", action)

	// Content with 501 chars should reject with "got 501" in error message
	content501 := content500 + "x"
	req2 := makeRequest(map[string]interface{}{
		"type":    "decision",
		"project": "acme-corp",
		"title":   "Test 501",
		"content": content501,
	})

	result2, err := handlePut(db)(context.TODO(), req2)
	require.NoError(t, err)
	assert.True(t, result2.IsError, "content of 501 chars should be rejected")
	assert.Len(t, result2.Content, 1)
	errContent, ok := mcp.AsTextContent(result2.Content[0])
	require.True(t, ok)
	assert.Contains(t, errContent.Text, "got 501", "error message should specify the actual length")
}

func TestHandlePutPreservesContentLengthIndependentOfNotes(t *testing.T) {
	resetDB(t)

	// content=260 chars + notes=830 chars, both should succeed and preserve lengths
	content260 := string(make([]byte, 260))
	for i := 0; i < 260; i++ {
		content260 = content260[:i] + "a"
	}
	notes830 := string(make([]byte, 830))
	for i := 0; i < 830; i++ {
		notes830 = notes830[:i] + "b"
	}

	req := makeRequest(map[string]interface{}{
		"type":    "fact",
		"project": "acme-corp",
		"title":   "OU-24 regression test",
		"content": content260,
		"notes":   notes830,
	})

	result, err := handlePut(db)(context.TODO(), req)
	require.NoError(t, err)
	assert.False(t, result.IsError, "put with content=260 and notes=830 should succeed")

	var resp map[string]interface{}
	err = unmarshalResult(result, &resp)
	require.NoError(t, err)
	idFloat, ok := resp["id"].(float64)
	require.True(t, ok)
	id := int64(idFloat)

	// Retrieve and verify stored content length is intact
	doc, err := store.GetDocument(db, id)
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, 260, len(doc.Content), "stored content length should match sent content length")
	assert.Equal(t, 830, len(doc.Notes), "stored notes length should match sent notes length")
}

func TestHandlePutAction(t *testing.T) {
	resetDB(t)

	// First put should return action=created
	req1 := makeRequest(map[string]interface{}{
		"type":    "decision",
		"project": "test-proj",
		"title":   "Test decision",
		"content": "Content v1",
	})

	result1, err := handlePut(db)(context.TODO(), req1)
	require.NoError(t, err)
	var resp1 map[string]interface{}
	err = unmarshalResult(result1, &resp1)
	require.NoError(t, err)
	action1, ok := resp1["action"].(string)
	require.True(t, ok)
	assert.Equal(t, "created", action1)

	// Second put with same title should return action=updated
	req2 := makeRequest(map[string]interface{}{
		"type":    "decision",
		"project": "test-proj",
		"title":   "Test decision",
		"content": "Content v2",
	})

	result2, err := handlePut(db)(context.TODO(), req2)
	require.NoError(t, err)
	var resp2 map[string]interface{}
	err = unmarshalResult(result2, &resp2)
	require.NoError(t, err)
	action2, ok := resp2["action"].(string)
	require.True(t, ok)
	assert.Equal(t, "updated", action2)
}

func unmarshalResult(result *mcp.CallToolResult, v interface{}) error {
	if len(result.Content) == 0 {
		return json.Unmarshal([]byte("{}"), v)
	}
	textContent, ok := mcp.AsTextContent(result.Content[0])
	if !ok {
		return json.Unmarshal([]byte("{}"), v)
	}
	return json.Unmarshal([]byte(textContent.Text), v)
}
