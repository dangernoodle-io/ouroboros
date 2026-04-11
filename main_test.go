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
)

func TestMain(m *testing.M) {
	var err error
	db, err = sql.Open("sqlite", ":memory:")
	if err != nil {
		panic(err)
	}
	if err = applySchema(db); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func resetDB(t *testing.T) {
	t.Helper()
	tables := []string{"decisions", "facts", "relations", "notes"}
	for _, table := range tables {
		_, err := db.Exec("DELETE FROM " + table)
		require.NoError(t, err)
	}
	require.NoError(t, rebuildFTS(db, "decisions"))
	require.NoError(t, rebuildFTS(db, "facts"))
	require.NoError(t, rebuildFTS(db, "notes"))
}

func makeRequest(args map[string]interface{}) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
}

func TestHandleLogDecision(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"project":   "acme-corp",
		"summary":   "Use PostgreSQL for persistence",
		"rationale": "Superior query performance for our use case",
		"tags":      []interface{}{"database", "infrastructure"},
	})

	result, err := handleLogDecision(context.TODO(), req)
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

func TestHandleGetDecisions(t *testing.T) {
	resetDB(t)

	// Log two decisions
	_, err := insertDecision(db, "acme-corp", "Use PostgreSQL", "performance", []string{"database"})
	require.NoError(t, err)
	_, err = insertDecision(db, "acme-corp", "Use gRPC", "low latency", []string{"api"})
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"project": "acme-corp",
	})

	result, err := handleGetDecisions(context.TODO(), req)
	require.NoError(t, err)

	var decisions []DecisionSummary
	err = unmarshalResult(result, &decisions)
	require.NoError(t, err)
	assert.Len(t, decisions, 2)
}

func TestHandleDeleteDecision(t *testing.T) {
	resetDB(t)

	id, err := insertDecision(db, "acme-corp", "Use PostgreSQL", "", nil)
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"id": float64(id),
	})

	result, err := handleDeleteDecision(context.TODO(), req)
	require.NoError(t, err)

	var resp map[string]bool
	err = unmarshalResult(result, &resp)
	require.NoError(t, err)
	assert.True(t, resp["ok"])

	// Verify deletion
	decisions, err := queryDecisions(db, "acme-corp", nil, "", 0)
	require.NoError(t, err)
	assert.Len(t, decisions, 0)
}

func TestHandleSetFact(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"project":  "acme-corp",
		"category": "hardware",
		"key":      "cpu_cores",
		"value":    "8",
	})

	result, err := handleSetFact(context.TODO(), req)
	require.NoError(t, err)

	var resp map[string]interface{}
	err = unmarshalResult(result, &resp)
	require.NoError(t, err)
	ok, okExists := resp["ok"].(bool)
	require.True(t, okExists)
	assert.True(t, ok)
	assert.NotZero(t, resp["id"])
}

func TestHandleGetFacts(t *testing.T) {
	resetDB(t)

	_, err := upsertFact(db, "acme-corp", "hardware", "cpu_cores", "8")
	require.NoError(t, err)
	_, err = upsertFact(db, "acme-corp", "hardware", "ram_gb", "16")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"project":  "acme-corp",
		"category": "hardware",
	})

	result, err := handleGetFacts(context.TODO(), req)
	require.NoError(t, err)

	var facts []Fact
	err = unmarshalResult(result, &facts)
	require.NoError(t, err)
	assert.Len(t, facts, 2)
}

func TestHandleDeleteFact(t *testing.T) {
	resetDB(t)

	_, err := upsertFact(db, "acme-corp", "hardware", "cpu_cores", "8")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"project":  "acme-corp",
		"category": "hardware",
		"key":      "cpu_cores",
	})

	result, err := handleDeleteFact(context.TODO(), req)
	require.NoError(t, err)

	var resp map[string]bool
	err = unmarshalResult(result, &resp)
	require.NoError(t, err)
	assert.True(t, resp["ok"])

	// Verify deletion
	facts, err := queryFacts(db, "acme-corp", "hardware", "cpu_cores", "", 0)
	require.NoError(t, err)
	assert.Len(t, facts, 0)
}

func TestHandleLink(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"source_project": "acme-corp",
		"source":         "backend-api",
		"target_project": "acme-corp",
		"target":         "postgres-db",
		"relation_type":  "depends_on",
		"description":    "Backend API queries the database",
	})

	result, err := handleLink(context.TODO(), req)
	require.NoError(t, err)

	var resp map[string]interface{}
	err = unmarshalResult(result, &resp)
	require.NoError(t, err)
	ok, okExists := resp["ok"].(bool)
	require.True(t, okExists)
	assert.True(t, ok)
	assert.NotZero(t, resp["id"])
}

func TestHandleGetLinks(t *testing.T) {
	resetDB(t)

	_, err := insertRelation(db, "acme-corp", "backend-api", "acme-corp", "postgres-db", "depends_on", "")
	require.NoError(t, err)
	_, err = insertRelation(db, "acme-corp", "backend-api", "acme-corp", "redis-cache", "uses", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"project": "acme-corp",
	})

	result, err := handleGetLinks(context.TODO(), req)
	require.NoError(t, err)

	var relations []Relation
	err = unmarshalResult(result, &relations)
	require.NoError(t, err)
	assert.Len(t, relations, 2)
}

func TestHandleDeleteLink(t *testing.T) {
	resetDB(t)

	id, err := insertRelation(db, "acme-corp", "backend-api", "acme-corp", "postgres-db", "depends_on", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"id": float64(id),
	})

	result, err := handleDeleteLink(context.TODO(), req)
	require.NoError(t, err)

	var resp map[string]bool
	err = unmarshalResult(result, &resp)
	require.NoError(t, err)
	assert.True(t, resp["ok"])

	// Verify deletion
	relations, err := queryRelations(db, "acme-corp", "", "", 0)
	require.NoError(t, err)
	assert.Len(t, relations, 0)
}

func TestHandleSearch(t *testing.T) {
	resetDB(t)

	_, err := insertDecision(db, "acme-corp", "Use PostgreSQL", "performance for queries", nil)
	require.NoError(t, err)
	_, err = upsertFact(db, "acme-corp", "database", "type", "PostgreSQL")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"query": "PostgreSQL",
	})

	result, err := handleSearch(context.TODO(), req)
	require.NoError(t, err)

	var searchResult SearchResult
	err = unmarshalResult(result, &searchResult)
	require.NoError(t, err)
	assert.Len(t, searchResult.Decisions, 1)
	assert.Len(t, searchResult.Facts, 1)
}

func TestHandleExport(t *testing.T) {
	resetDB(t)

	_, err := insertDecision(db, "acme-corp", "Use PostgreSQL", "good performance", []string{"database"})
	require.NoError(t, err)
	_, err = upsertFact(db, "acme-corp", "hardware", "cpu_cores", "8")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"project": "acme-corp",
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
		"decisions": [
			{"project": "acme-corp", "summary": "Use PostgreSQL", "rationale": "good performance"}
		],
		"facts": [
			{"project": "acme-corp", "category": "hardware", "key": "cpu_cores", "value": "8"}
		],
		"relations": []
	}`

	req := makeRequest(map[string]interface{}{
		"project": "acme-corp",
		"content": importJSON,
	})

	result, err := handleImport(context.TODO(), req)
	require.NoError(t, err)

	var resp map[string]bool
	err = unmarshalResult(result, &resp)
	require.NoError(t, err)
	assert.True(t, resp["ok"])

	// Verify import
	decisions, err := queryDecisions(db, "acme-corp", nil, "", 0)
	require.NoError(t, err)
	assert.Len(t, decisions, 1)

	facts, err := queryFacts(db, "acme-corp", "", "", "", 0)
	require.NoError(t, err)
	assert.Len(t, facts, 1)
}

func TestHandleSetNote(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"project":  "acme-corp",
		"category": "procedure",
		"title":    "release-process",
		"body":     "1. Tag version\n2. Push tag\n3. Monitor CI",
		"tags":     []interface{}{"release", "ci"},
	})

	result, err := handleSetNote(context.TODO(), req)
	require.NoError(t, err)

	var resp map[string]interface{}
	err = unmarshalResult(result, &resp)
	require.NoError(t, err)
	ok, okExists := resp["ok"].(bool)
	require.True(t, okExists)
	assert.True(t, ok)
	assert.NotZero(t, resp["id"])
}

func TestHandleGetNotesListMode(t *testing.T) {
	resetDB(t)

	_, err := upsertNote(db, "acme-corp", "procedure", "release-process", "long body content here", []string{"release"})
	require.NoError(t, err)
	_, err = upsertNote(db, "acme-corp", "guide", "onboarding", "welcome guide body", nil)
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"project": "acme-corp",
	})

	result, err := handleGetNotes(context.TODO(), req)
	require.NoError(t, err)

	var summaries []NoteSummary
	err = unmarshalResult(result, &summaries)
	require.NoError(t, err)
	assert.Len(t, summaries, 2)
}

func TestHandleGetNotesDetailMode(t *testing.T) {
	resetDB(t)

	id, err := upsertNote(db, "acme-corp", "procedure", "release-process", "1. Tag\n2. Push", []string{"release"})
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"id": float64(id),
	})

	result, err := handleGetNotes(context.TODO(), req)
	require.NoError(t, err)

	var note Note
	err = unmarshalResult(result, &note)
	require.NoError(t, err)
	assert.Equal(t, "release-process", note.Title)
	assert.Equal(t, "1. Tag\n2. Push", note.Body)
	assert.ElementsMatch(t, []string{"release"}, note.Tags)
}

func TestHandleGetNotesNotFound(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"id": float64(999),
	})

	result, err := handleGetNotes(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestHandleDeleteNote(t *testing.T) {
	resetDB(t)

	id, err := upsertNote(db, "acme-corp", "procedure", "release-process", "body", nil)
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"id": float64(id),
	})

	result, err := handleDeleteNote(context.TODO(), req)
	require.NoError(t, err)

	var resp map[string]bool
	err = unmarshalResult(result, &resp)
	require.NoError(t, err)
	assert.True(t, resp["ok"])

	note, err := getNote(db, id)
	require.NoError(t, err)
	assert.Nil(t, note)
}

func TestHandleMissingRequiredParams(t *testing.T) {
	resetDB(t)

	// Test missing project in log decision
	result, err := handleLogDecision(context.TODO(), makeRequest(map[string]interface{}{"summary": "test"}))
	require.NoError(t, err)
	assert.True(t, result.IsError)

	// Test missing summary in log decision
	result, err = handleLogDecision(context.TODO(), makeRequest(map[string]interface{}{"project": "test-proj"}))
	require.NoError(t, err)
	assert.True(t, result.IsError)

	// Test missing id in delete decision
	result, err = handleDeleteDecision(context.TODO(), makeRequest(map[string]interface{}{}))
	require.NoError(t, err)
	assert.True(t, result.IsError)

	// Test missing project in set fact
	result, err = handleSetFact(context.TODO(), makeRequest(map[string]interface{}{"category": "hw", "key": "k", "value": "v"}))
	require.NoError(t, err)
	assert.True(t, result.IsError)

	// Test missing key in delete fact
	result, err = handleDeleteFact(context.TODO(), makeRequest(map[string]interface{}{"project": "p", "category": "c"}))
	require.NoError(t, err)
	assert.True(t, result.IsError)

	// Test missing source_project in link
	result, err = handleLink(context.TODO(), makeRequest(map[string]interface{}{"source": "a", "target_project": "p", "target": "b", "relation_type": "depends_on"}))
	require.NoError(t, err)
	assert.True(t, result.IsError)

	// Test missing query in search
	result, err = handleSearch(context.TODO(), makeRequest(map[string]interface{}{}))
	require.NoError(t, err)
	assert.True(t, result.IsError)

	// Test missing project in import
	result, err = handleImport(context.TODO(), makeRequest(map[string]interface{}{"content": "{}"}))
	require.NoError(t, err)
	assert.True(t, result.IsError)
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
