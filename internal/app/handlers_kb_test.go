package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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

// TestHandlePutBatch tests batch put with single entry.
func TestHandlePutBatch(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"type":    "decision",
				"project": "acme-corp",
				"title":   "Use PostgreSQL",
				"content": "Superior query performance for our use case",
				"tags":    []interface{}{"database", "infrastructure"},
			},
		},
	})

	result, err := handlePut(db)(context.TODO(), req)
	require.NoError(t, err)
	require.NotNil(t, result)

	var resp []map[string]interface{}
	err = unmarshalResult(result, &resp)
	require.NoError(t, err)
	require.Len(t, resp, 1)

	assert.Equal(t, "created", resp[0]["action"])
	assert.NotZero(t, resp[0]["id"])
	assert.Equal(t, "Use PostgreSQL", resp[0]["title"])
}

// TestHandlePutBatchMultiple tests batch put with multiple entries.
func TestHandlePutBatchMultiple(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"type":    "decision",
				"project": "acme-corp",
				"title":   "Use PostgreSQL",
				"content": "Decision 1",
			},
			map[string]interface{}{
				"type":    "fact",
				"project": "acme-corp",
				"title":   "Database Version",
				"content": "PostgreSQL 15",
			},
			map[string]interface{}{
				"type":    "note",
				"project": "acme-corp",
				"title":   "Schema Changes",
				"content": "Need migration",
			},
		},
	})

	result, err := handlePut(db)(context.TODO(), req)
	require.NoError(t, err)

	var resp []map[string]interface{}
	err = unmarshalResult(result, &resp)
	require.NoError(t, err)
	require.Len(t, resp, 3)

	for i, r := range resp {
		assert.Equal(t, "created", r["action"])
		assert.NotZero(t, r["id"])
		assert.NotEmpty(t, r["title"])
		t.Logf("created entry %d: id=%v title=%s", i+1, r["id"], r["title"])
	}
}

// TestHandlePutBatchEmpty tests batch put with empty array.
func TestHandlePutBatchEmpty(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{},
	})

	result, err := handlePut(db)(context.TODO(), req)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should return error for empty batch
	assert.True(t, result.IsError)
}

// TestHandleGetBatch tests batch get with ids.
func TestHandleGetBatch(t *testing.T) {
	resetDB(t)

	// Insert test data
	putReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"type":    "decision",
				"project": "acme-corp",
				"title":   "Decision 1",
				"content": "Content 1",
			},
			map[string]interface{}{
				"type":    "fact",
				"project": "acme-corp",
				"title":   "Fact 1",
				"content": "Content 2",
			},
		},
	})

	putResult, err := handlePut(db)(context.TODO(), putReq)
	require.NoError(t, err)

	var putResp []map[string]interface{}
	err = unmarshalResult(putResult, &putResp)
	require.NoError(t, err)
	require.Len(t, putResp, 2)

	id1, ok1 := putResp[0]["id"].(float64)
	require.True(t, ok1)
	id2, ok2 := putResp[1]["id"].(float64)
	require.True(t, ok2)

	// Fetch both by id
	getReq := makeRequest(map[string]interface{}{
		"ids": []interface{}{id1, id2},
	})

	getResult, err := handleGet(db)(context.TODO(), getReq)
	require.NoError(t, err)

	var docs []map[string]interface{}
	err = unmarshalResult(getResult, &docs)
	require.NoError(t, err)
	require.Len(t, docs, 2)

	assert.Equal(t, "Decision 1", docs[0]["title"])
	assert.Equal(t, "Fact 1", docs[1]["title"])
}

// TestHandleGetBatchWithMiss tests batch get with missing IDs (should omit).
func TestHandleGetBatchWithMiss(t *testing.T) {
	resetDB(t)

	// Insert one document
	putReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"type":    "decision",
				"project": "acme-corp",
				"title":   "Decision 1",
				"content": "Content 1",
			},
		},
	})

	putResult, err := handlePut(db)(context.TODO(), putReq)
	require.NoError(t, err)

	var putResp []map[string]interface{}
	err = unmarshalResult(putResult, &putResp)
	require.NoError(t, err)

	id1, ok := putResp[0]["id"].(float64)
	require.True(t, ok)

	// Fetch with existing and non-existing IDs
	getReq := makeRequest(map[string]interface{}{
		"ids": []interface{}{id1, 9999.0}, // 9999 doesn't exist
	})

	getResult, err := handleGet(db)(context.TODO(), getReq)
	require.NoError(t, err)

	var docs []map[string]interface{}
	err = unmarshalResult(getResult, &docs)
	require.NoError(t, err)
	require.Len(t, docs, 1, "should omit missing ID 9999")
	assert.Equal(t, "Decision 1", docs[0]["title"])
}

// TestHandlePutValidationAbortsEntireBatch tests that validation failure aborts whole batch.
func TestHandlePutValidationAbortsEntireBatch(t *testing.T) {
	resetDB(t)

	// Entry 2 has missing "type" field (invalid)
	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"type":    "decision",
				"project": "acme-corp",
				"title":   "Valid entry",
				"content": "Content 1",
			},
			map[string]interface{}{
				"project": "acme-corp", // missing required "type"
				"title":   "Invalid entry",
				"content": "Content 2",
			},
		},
	})

	result, err := handlePut(db)(context.TODO(), req)
	require.NoError(t, err)

	// Should have validation error
	assert.True(t, result.IsError, "should return error due to invalid entry")

	// Verify no entries were written
	getListReq := makeRequest(map[string]interface{}{
		"type": "decision",
	})
	getResult, err := handleGet(db)(context.TODO(), getListReq)
	require.NoError(t, err)
	var docs []map[string]interface{}
	err = unmarshalResult(getResult, &docs)
	require.NoError(t, err)
	require.Len(t, docs, 0, "batch validation failure should prevent all writes")
}

// TestHandlePutBatch50Entries tests large batch performance.
func TestHandlePutBatch50Entries(t *testing.T) {
	resetDB(t)

	entries := make([]interface{}, 50)
	for i := 0; i < 50; i++ {
		j := i + 1
		title := "Entry " + fmt.Sprintf("%02d", j) // Entry 01, Entry 02, ...
		entries[i] = map[string]interface{}{
			"type":    "note",
			"project": "acme-corp",
			"title":   title,
			"content": "Content for entry",
		}
	}

	req := makeRequest(map[string]interface{}{
		"entries": entries,
	})

	result, err := handlePut(db)(context.TODO(), req)
	require.NoError(t, err)

	var resp []map[string]interface{}
	err = unmarshalResult(result, &resp)
	require.NoError(t, err)
	require.Len(t, resp, 50)

	for _, r := range resp {
		assert.Equal(t, "created", r["action"])
		assert.NotZero(t, r["id"])
	}
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

// TestHandleSearch_SingleQuery_BackwardsCompat tests single-query mode returns flat []DocumentSummary.
func TestHandleSearch_SingleQuery_BackwardsCompat(t *testing.T) {
	resetDB(t)

	// Seed a document
	putReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"type":    "decision",
				"project": "ouroboros",
				"title":   "tiktoken",
				"content": "Use tiktoken for token counting",
			},
		},
	})

	_, err := handlePut(db)(context.TODO(), putReq)
	require.NoError(t, err)

	// Search with single query
	searchReq := makeRequest(map[string]interface{}{
		"query": "tiktoken",
	})

	searchResult, err := handleSearch(db)(context.TODO(), searchReq)
	require.NoError(t, err)

	// Should unmarshal as flat []DocumentSummary
	var summaries []store.DocumentSummary
	err = unmarshalResult(searchResult, &summaries)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "tiktoken", summaries[0].Title)
}

// TestHandleSearch_Batch_PositionalResults tests batch mode returns [][]DocumentSummary in input order.
func TestHandleSearch_Batch_PositionalResults(t *testing.T) {
	resetDB(t)

	// Seed documents
	putReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"type":    "fact",
				"project": "acme-corp",
				"title":   "First doc about alpha",
				"content": "Content about alpha",
			},
			map[string]interface{}{
				"type":    "fact",
				"project": "acme-corp",
				"title":   "Second doc about beta",
				"content": "Content about beta",
			},
			map[string]interface{}{
				"type":    "fact",
				"project": "acme-corp",
				"title":   "Third doc about gamma",
				"content": "Content about gamma",
			},
		},
	})

	_, err := handlePut(db)(context.TODO(), putReq)
	require.NoError(t, err)

	// Search with batch queries
	searchReq := makeRequest(map[string]interface{}{
		"queries": []interface{}{"alpha", "beta", "gamma"},
	})

	searchResult, err := handleSearch(db)(context.TODO(), searchReq)
	require.NoError(t, err)

	// Should unmarshal as [][]DocumentSummary
	var resultSets [][]store.DocumentSummary
	err = unmarshalResult(searchResult, &resultSets)
	require.NoError(t, err)
	require.Len(t, resultSets, 3)

	// Verify order and content
	assert.Len(t, resultSets[0], 1)
	assert.Equal(t, "First doc about alpha", resultSets[0][0].Title)
	assert.Len(t, resultSets[1], 1)
	assert.Equal(t, "Second doc about beta", resultSets[1][0].Title)
	assert.Len(t, resultSets[2], 1)
	assert.Equal(t, "Third doc about gamma", resultSets[2][0].Title)
}

// TestHandleSearch_Batch_EmptyResultSetsAreEmptyNotNil tests empty result slots are [] not null.
func TestHandleSearch_Batch_EmptyResultSetsAreEmptyNotNil(t *testing.T) {
	resetDB(t)

	// Seed one document
	putReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"type":    "fact",
				"project": "acme-corp",
				"title":   "matching doc",
				"content": "About matches",
			},
		},
	})

	_, err := handlePut(db)(context.TODO(), putReq)
	require.NoError(t, err)

	// Search with one matching and one non-matching query
	searchReq := makeRequest(map[string]interface{}{
		"queries": []interface{}{"matches", "nothing-will-match-xyz123"},
	})

	searchResult, err := handleSearch(db)(context.TODO(), searchReq)
	require.NoError(t, err)

	// Get raw JSON text
	textContent, ok := mcp.AsTextContent(searchResult.Content[0])
	require.True(t, ok)
	jsonText := textContent.Text

	// Verify second result set is [] not null
	assert.Contains(t, jsonText, "[[", "should contain array of arrays")
	assert.Contains(t, jsonText, "[]", "should contain empty array for no matches")

	// Also verify unmarshal
	var resultSets [][]store.DocumentSummary
	err = json.Unmarshal([]byte(jsonText), &resultSets)
	require.NoError(t, err)
	require.Len(t, resultSets, 2)
	assert.Len(t, resultSets[0], 1)
	assert.Len(t, resultSets[1], 0, "empty result slot should be non-nil empty slice")
}

// TestHandleSearch_Batch_SharedFilters tests top-level filters apply to all queries.
func TestHandleSearch_Batch_SharedFilters(t *testing.T) {
	resetDB(t)

	// Seed documents in different projects
	putReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"type":    "fact",
				"project": "ouroboros",
				"title":   "about alpha in ouroboros",
				"content": "Content alpha",
			},
			map[string]interface{}{
				"type":    "fact",
				"project": "ouroboros",
				"title":   "about beta in ouroboros",
				"content": "Content beta",
			},
			map[string]interface{}{
				"type":    "fact",
				"project": "other-project",
				"title":   "about alpha in other",
				"content": "Content alpha",
			},
		},
	})

	_, err := handlePut(db)(context.TODO(), putReq)
	require.NoError(t, err)

	// Search with batch queries and project filter
	searchReq := makeRequest(map[string]interface{}{
		"queries": []interface{}{"alpha", "beta"},
		"project": "ouroboros",
	})

	searchResult, err := handleSearch(db)(context.TODO(), searchReq)
	require.NoError(t, err)

	var resultSets [][]store.DocumentSummary
	err = unmarshalResult(searchResult, &resultSets)
	require.NoError(t, err)
	require.Len(t, resultSets, 2)

	// Both result sets should only contain ouroboros project docs
	require.Len(t, resultSets[0], 1)
	assert.Equal(t, "ouroboros", resultSets[0][0].Project)
	assert.Equal(t, "about alpha in ouroboros", resultSets[0][0].Title)

	require.Len(t, resultSets[1], 1)
	assert.Equal(t, "ouroboros", resultSets[1][0].Project)
	assert.Equal(t, "about beta in ouroboros", resultSets[1][0].Title)
}

// TestHandleSearch_NeitherQueryNorQueries_Errors tests error when neither param provided.
func TestHandleSearch_NeitherQueryNorQueries_Errors(t *testing.T) {
	resetDB(t)

	// Request with neither query nor queries
	searchReq := makeRequest(map[string]interface{}{})

	searchResult, err := handleSearch(db)(context.TODO(), searchReq)
	require.NoError(t, err)

	// Should be an error
	assert.True(t, searchResult.IsError)
	textContent, ok := mcp.AsTextContent(searchResult.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "query or queries is required")
}

// TestHandleSearch_BothProvided_QueriesWins tests batch mode takes precedence.
func TestHandleSearch_BothProvided_QueriesWins(t *testing.T) {
	resetDB(t)

	// Seed documents
	putReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"type":    "fact",
				"project": "acme-corp",
				"title":   "about alpha",
				"content": "Content alpha",
			},
			map[string]interface{}{
				"type":    "fact",
				"project": "acme-corp",
				"title":   "about beta",
				"content": "Content beta",
			},
		},
	})

	_, err := handlePut(db)(context.TODO(), putReq)
	require.NoError(t, err)

	// Request with both query and queries; queries should win
	searchReq := makeRequest(map[string]interface{}{
		"query":   "alpha",
		"queries": []interface{}{"beta", "alpha"},
	})

	searchResult, err := handleSearch(db)(context.TODO(), searchReq)
	require.NoError(t, err)

	// Should return batch shape (array of arrays), not flat shape
	var resultSets [][]store.DocumentSummary
	err = unmarshalResult(searchResult, &resultSets)
	require.NoError(t, err)
	require.Len(t, resultSets, 2, "should use batch mode (queries[]) not single mode (query)")
	assert.Len(t, resultSets[0], 1)
	assert.Equal(t, "about beta", resultSets[0][0].Title)
	assert.Len(t, resultSets[1], 1)
	assert.Equal(t, "about alpha", resultSets[1][0].Title)
}
