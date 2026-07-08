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

	"dangernoodle.io/ouroboros/internal/backlog"
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

// TestHandleKBBatch tests batch kb write with single entry.
func TestHandleKBBatch(t *testing.T) {
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

	result, err := handleKB(db)(context.TODO(), req)
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

// TestHandleKBBatchMultiple tests batch kb write with multiple entries.
func TestHandleKBBatchMultiple(t *testing.T) {
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

	result, err := handleKB(db)(context.TODO(), req)
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

// TestHandleKBBatchEmpty tests batch kb write with empty array.
func TestHandleKBBatchEmpty(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{},
	})

	result, err := handleKB(db)(context.TODO(), req)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should return error for empty batch
	assert.True(t, result.IsError)
}

// TestHandleGetBatch tests domain=kb batch get with ids.
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

	putResult, err := handleKB(db)(context.TODO(), putReq)
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
		"domain": "kb",
		"ids":    []interface{}{id1, id2},
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

// TestHandleGetBatchWithMiss tests domain=kb batch get with missing IDs (should omit).
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

	putResult, err := handleKB(db)(context.TODO(), putReq)
	require.NoError(t, err)

	var putResp []map[string]interface{}
	err = unmarshalResult(putResult, &putResp)
	require.NoError(t, err)

	id1, ok := putResp[0]["id"].(float64)
	require.True(t, ok)

	// Fetch with existing and non-existing IDs
	getReq := makeRequest(map[string]interface{}{
		"domain": "kb",
		"ids":    []interface{}{id1, 9999.0}, // 9999 doesn't exist
	})

	getResult, err := handleGet(db)(context.TODO(), getReq)
	require.NoError(t, err)

	var docs []map[string]interface{}
	err = unmarshalResult(getResult, &docs)
	require.NoError(t, err)
	require.Len(t, docs, 1, "should omit missing ID 9999")
	assert.Equal(t, "Decision 1", docs[0]["title"])
}

// TestHandleKBValidationAbortsEntireBatch tests that validation failure aborts whole batch.
func TestHandleKBValidationAbortsEntireBatch(t *testing.T) {
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

	result, err := handleKB(db)(context.TODO(), req)
	require.NoError(t, err)

	// Should have validation error
	assert.True(t, result.IsError, "should return error due to invalid entry")

	// Verify no entries were written
	getListReq := makeRequest(map[string]interface{}{
		"domain": "kb",
		"types":  []interface{}{"decision"},
	})
	getResult, err := handleGet(db)(context.TODO(), getListReq)
	require.NoError(t, err)
	var docs []map[string]interface{}
	err = unmarshalResult(getResult, &docs)
	require.NoError(t, err)
	require.Len(t, docs, 0, "batch validation failure should prevent all writes")
}

// TestHandleKBBatch50Entries tests large batch performance.
func TestHandleKBBatch50Entries(t *testing.T) {
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

	result, err := handleKB(db)(context.TODO(), req)
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

// TestHandleSearch_SingleQuery_BackwardsCompat tests domain=kb single-query mode returns flat []DocumentSummary.
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

	_, err := handleKB(db)(context.TODO(), putReq)
	require.NoError(t, err)

	// Search with single query
	searchReq := makeRequest(map[string]interface{}{
		"domain": "kb",
		"query":  "tiktoken",
	})

	searchResult, err := handleSearch(db)(context.TODO(), searchReq)
	require.NoError(t, err)

	// Should unmarshal as flat []DocumentSummary
	var summaries []store.DocumentSummary
	err = unmarshalResult(searchResult, &summaries)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "tiktoken", summaries[0].Title)
	// SearchDocuments uses FTS path; score field should not be populated (SearchDocuments doesn't expose BM25)
	assert.Zero(t, summaries[0].Score)
}

// TestHandleSearch_Batch_PositionalResults tests domain=kb batch mode returns [][]DocumentSummary in input order.
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

	_, err := handleKB(db)(context.TODO(), putReq)
	require.NoError(t, err)

	// Search with batch queries
	searchReq := makeRequest(map[string]interface{}{
		"domain":  "kb",
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

	_, err := handleKB(db)(context.TODO(), putReq)
	require.NoError(t, err)

	// Search with one matching and one non-matching query
	searchReq := makeRequest(map[string]interface{}{
		"domain":  "kb",
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

	_, err := handleKB(db)(context.TODO(), putReq)
	require.NoError(t, err)

	// Search with batch queries and project filter
	searchReq := makeRequest(map[string]interface{}{
		"domain":   "kb",
		"queries":  []interface{}{"alpha", "beta"},
		"projects": []interface{}{"ouroboros"},
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
	searchReq := makeRequest(map[string]interface{}{"domain": "kb"})

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

	_, err := handleKB(db)(context.TODO(), putReq)
	require.NoError(t, err)

	// Request with both query and queries; queries should win
	searchReq := makeRequest(map[string]interface{}{
		"domain":  "kb",
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

// TestHandleSearch_ReturnsResults verifies handleSearch returns matching documents.
func TestHandleSearch_ReturnsResults(t *testing.T) {
	resetDB(t)

	// Seed a document
	putReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"type":    "decision",
				"project": "acme-corp",
				"title":   "Use SQLite",
				"content": "Lightweight embedded database for local storage",
			},
		},
	})
	_, err := handleKB(db)(context.TODO(), putReq)
	require.NoError(t, err)

	handler := handleSearch(db)
	searchReq := makeRequest(map[string]interface{}{
		"domain": "kb",
		"query":  "SQLite",
	})
	result, err := handler(context.TODO(), searchReq)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	var summaries []store.DocumentSummary
	err = unmarshalResult(result, &summaries)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "Use SQLite", summaries[0].Title)
}

// TestHandleSearch_MultiProject tests search with multiple project filters.
func TestHandleSearch_MultiProject(t *testing.T) {
	resetDB(t)

	// Seed documents in three projects
	putReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"type":    "fact",
				"project": "project-a",
				"title":   "doc with keyword",
				"content": "Content about keyword",
			},
			map[string]interface{}{
				"type":    "fact",
				"project": "project-b",
				"title":   "another doc with keyword",
				"content": "Content about keyword",
			},
			map[string]interface{}{
				"type":    "fact",
				"project": "project-c",
				"title":   "third doc with keyword",
				"content": "Content about keyword",
			},
		},
	})

	_, err := handleKB(db)(context.TODO(), putReq)
	require.NoError(t, err)

	// Search with multiple project filters
	searchReq := makeRequest(map[string]interface{}{
		"domain":   "kb",
		"query":    "keyword",
		"projects": []interface{}{"project-a", "project-b"},
	})

	searchResult, err := handleSearch(db)(context.TODO(), searchReq)
	require.NoError(t, err)

	var summaries []store.DocumentSummary
	err = unmarshalResult(searchResult, &summaries)
	require.NoError(t, err)

	// Should only return docs from project-a and project-b
	require.Len(t, summaries, 2)
	projects := make(map[string]bool)
	for _, s := range summaries {
		projects[s.Project] = true
	}
	assert.True(t, projects["project-a"])
	assert.True(t, projects["project-b"])
	assert.False(t, projects["project-c"])
}

// TestHandleKBBatch_CategoryNotesMetadata tests that category, notes, and metadata fields are stored.
func TestHandleKBBatch_CategoryNotesMetadata(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"type":     "decision",
				"project":  "acme-corp",
				"title":    "Entry with extras",
				"content":  "Content",
				"category": "architecture",
				"notes":    "Some notes here",
				"metadata": map[string]interface{}{
					"source": "meeting",
					"owner":  "team-a",
				},
			},
		},
	})

	result, err := handleKB(db)(context.TODO(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	var resp []map[string]interface{}
	err = unmarshalResult(result, &resp)
	require.NoError(t, err)
	require.Len(t, resp, 1)
	assert.Equal(t, "created", resp[0]["action"])
}

// TestHandleGet_Limit tests that the limit parameter is respected in filter/list mode.
func TestHandleGet_Limit(t *testing.T) {
	resetDB(t)

	// Insert three documents
	putReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{"type": "fact", "project": "acme-corp", "title": "Doc 1", "content": "c1"},
			map[string]interface{}{"type": "fact", "project": "acme-corp", "title": "Doc 2", "content": "c2"},
			map[string]interface{}{"type": "fact", "project": "acme-corp", "title": "Doc 3", "content": "c3"},
		},
	})
	_, err := handleKB(db)(context.TODO(), putReq)
	require.NoError(t, err)

	// Request with limit=2
	req := makeRequest(map[string]interface{}{
		"domain": "kb",
		"types":  []interface{}{"fact"},
		"limit":  float64(2),
	})
	result, err := handleGet(db)(context.TODO(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	var docs []map[string]interface{}
	err = unmarshalResult(result, &docs)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(docs), 2)
}

// TestHandleGet_BatchVerbose tests verbose mode strips notes.
func TestHandleGet_BatchVerbose(t *testing.T) {
	resetDB(t)

	putReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"type":    "decision",
				"project": "acme-corp",
				"title":   "Verbose test",
				"content": "Content",
				"notes":   "Private notes",
			},
		},
	})
	putResult, err := handleKB(db)(context.TODO(), putReq)
	require.NoError(t, err)

	var putResp []map[string]interface{}
	err = unmarshalResult(putResult, &putResp)
	require.NoError(t, err)
	id, ok := putResp[0]["id"].(float64)
	require.True(t, ok)

	// Non-verbose: notes should be empty
	req := makeRequest(map[string]interface{}{
		"domain":  "kb",
		"ids":     []interface{}{id},
		"verbose": false,
	})
	result, err := handleGet(db)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)
}

// TestHandleSearch_SingleQuery_WithLimit tests single-query mode with limit.
func TestHandleSearch_SingleQuery_WithLimit(t *testing.T) {
	resetDB(t)

	putReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{"type": "fact", "project": "acme-corp", "title": "alpha one", "content": "alpha content one"},
			map[string]interface{}{"type": "fact", "project": "acme-corp", "title": "alpha two", "content": "alpha content two"},
			map[string]interface{}{"type": "fact", "project": "acme-corp", "title": "alpha three", "content": "alpha content three"},
		},
	})
	_, err := handleKB(db)(context.TODO(), putReq)
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"domain": "kb",
		"query":  "alpha",
		"limit":  float64(1),
	})
	result, err := handleSearch(db)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var summaries []store.DocumentSummary
	err = unmarshalResult(result, &summaries)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(summaries), 1)
}

// TestHandleGet_IdsNoSessionID tests that ids-fetch response omits session_id.
func TestHandleGet_IdsNoSessionID(t *testing.T) {
	resetDB(t)

	// Insert a document with a session_id
	putReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"type":    "decision",
				"project": "acme-corp",
				"title":   "session-strip-test",
				"content": "Content",
				"metadata": map[string]interface{}{
					"session_id": "sess-test-999",
				},
			},
		},
	})
	putResult, err := handleKB(db)(context.TODO(), putReq)
	require.NoError(t, err)

	var putResp []map[string]interface{}
	require.NoError(t, unmarshalResult(putResult, &putResp))
	id, ok := putResp[0]["id"].(float64)
	require.True(t, ok)

	// Fetch by id
	getReq := makeRequest(map[string]interface{}{
		"domain": "kb",
		"ids":    []interface{}{id},
	})
	result, err := handleGet(db)(context.TODO(), getReq)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var docs []map[string]interface{}
	require.NoError(t, unmarshalResult(result, &docs))
	require.Len(t, docs, 1)

	// session_id must be absent (omitempty) or empty string — never the real value
	sessionID, hasKey := docs[0]["session_id"]
	if hasKey {
		assert.Empty(t, sessionID, "session_id must be empty/absent in MCP response")
	}
}

// TestHandleSearch_Batch_WithLimit tests batch mode with limit.
func TestHandleSearch_Batch_WithLimit(t *testing.T) {
	resetDB(t)

	putReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{"type": "fact", "project": "acme-corp", "title": "beta one", "content": "beta content one"},
			map[string]interface{}{"type": "fact", "project": "acme-corp", "title": "beta two", "content": "beta content two"},
		},
	})
	_, err := handleKB(db)(context.TODO(), putReq)
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"domain":  "kb",
		"queries": []interface{}{"beta"},
		"limit":   float64(1),
	})
	result, err := handleSearch(db)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var resultSets [][]store.DocumentSummary
	err = unmarshalResult(result, &resultSets)
	require.NoError(t, err)
	require.Len(t, resultSets, 1)
	assert.LessOrEqual(t, len(resultSets[0]), 1)
}

// TestHandleGet_MultiTypes tests types[] array returns docs of all listed types.
func TestHandleGet_MultiTypes(t *testing.T) {
	resetDB(t)

	putReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{"type": "decision", "project": "acme-corp", "title": "A decision", "content": "c1"},
			map[string]interface{}{"type": "fact", "project": "acme-corp", "title": "A fact", "content": "c2"},
			map[string]interface{}{"type": "note", "project": "acme-corp", "title": "A note", "content": "c3"},
		},
	})
	_, err := handleKB(db)(context.TODO(), putReq)
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"domain": "kb",
		"types":  []interface{}{"decision", "fact"},
	})
	result, err := handleGet(db)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var docs []map[string]interface{}
	err = unmarshalResult(result, &docs)
	require.NoError(t, err)
	require.Len(t, docs, 2)

	types := make(map[string]bool)
	for _, d := range docs {
		tp, ok := d["type"].(string)
		require.True(t, ok)
		types[tp] = true
	}
	assert.True(t, types["decision"])
	assert.True(t, types["fact"])
	assert.False(t, types["note"])
}

// TestHandleGet_MultiCategories tests categories[] array returns docs from all listed categories.
func TestHandleGet_MultiCategories(t *testing.T) {
	resetDB(t)

	putReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{"type": "fact", "project": "acme-corp", "category": "architecture", "title": "Arch fact", "content": "c1"},
			map[string]interface{}{"type": "fact", "project": "acme-corp", "category": "config", "title": "Config fact", "content": "c2"},
			map[string]interface{}{"type": "fact", "project": "acme-corp", "category": "ops", "title": "Ops fact", "content": "c3"},
		},
	})
	_, err := handleKB(db)(context.TODO(), putReq)
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"domain":     "kb",
		"categories": []interface{}{"architecture", "config"},
	})
	result, err := handleGet(db)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var docs []map[string]interface{}
	err = unmarshalResult(result, &docs)
	require.NoError(t, err)
	require.Len(t, docs, 2)

	cats := make(map[string]bool)
	for _, d := range docs {
		cat, ok := d["category"].(string)
		require.True(t, ok)
		cats[cat] = true
	}
	assert.True(t, cats["architecture"])
	assert.True(t, cats["config"])
	assert.False(t, cats["ops"])
}

// TestHandleKBBatch_ConflictDetected verifies that overwriting with different content
// returns conflict=true on the affected entry.
func TestHandleKBBatch_ConflictDetected(t *testing.T) {
	resetDB(t)

	putFirst := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"type":    "decision",
				"project": "acme-corp",
				"title":   "conflict-entry",
				"content": "original content",
			},
		},
	})
	_, err := handleKB(db)(context.TODO(), putFirst)
	require.NoError(t, err)

	putSecond := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"type":    "decision",
				"project": "acme-corp",
				"title":   "conflict-entry",
				"content": "changed content",
			},
		},
	})
	result, err := handleKB(db)(context.TODO(), putSecond)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var resp []map[string]interface{}
	require.NoError(t, unmarshalResult(result, &resp))
	require.Len(t, resp, 1)
	assert.Equal(t, true, resp[0]["conflict"])
	assert.NotEmpty(t, resp[0]["previous_excerpt"])
}

// TestHandleKBBatch_NoConflictSameContent verifies conflict is absent when content unchanged.
func TestHandleKBBatch_NoConflictSameContent(t *testing.T) {
	resetDB(t)

	req := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"type":    "fact",
				"project": "acme-corp",
				"title":   "stable-entry",
				"content": "stable content",
			},
		},
	})

	_, err := handleKB(db)(context.TODO(), req)
	require.NoError(t, err)

	result, err := handleKB(db)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var resp []map[string]interface{}
	require.NoError(t, unmarshalResult(result, &resp))
	require.Len(t, resp, 1)
	_, hasConflict := resp[0]["conflict"]
	assert.False(t, hasConflict, "conflict key must be absent (omitempty) when no conflict")
}

// TestHandleSearch_CategoriesFilter tests search with categories[] filter.
func TestHandleSearch_CategoriesFilter(t *testing.T) {
	resetDB(t)

	putReq := makeRequest(map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{"type": "fact", "project": "acme-corp", "category": "arch", "title": "arch fact", "content": "postgres storage"},
			map[string]interface{}{"type": "fact", "project": "acme-corp", "category": "ops", "title": "ops fact", "content": "postgres deployment"},
		},
	})
	_, err := handleKB(db)(context.TODO(), putReq)
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"domain":     "kb",
		"query":      "postgres",
		"categories": []interface{}{"arch"},
	})
	result, err := handleSearch(db)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var summaries []store.DocumentSummary
	err = unmarshalResult(result, &summaries)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "arch", summaries[0].Category)
}

// ---- domain dispatch tests (OU: read/write surface split) ----

// TestHandleGet_MissingDomain_Errors verifies get requires a domain.
func TestHandleGet_MissingDomain_Errors(t *testing.T) {
	resetDB(t)

	result, err := handleGet(db)(context.TODO(), makeRequest(map[string]interface{}{}))
	require.NoError(t, err)
	assert.True(t, result.IsError)
	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, `must be "kb", "backlog", or "roadmap"`)
}

// TestHandleGet_InvalidDomain_Errors verifies get rejects an unrecognized domain.
func TestHandleGet_InvalidDomain_Errors(t *testing.T) {
	resetDB(t)

	result, err := handleGet(db)(context.TODO(), makeRequest(map[string]interface{}{"domain": "bogus"}))
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleSearch_MissingDomain_Errors verifies search requires a domain.
func TestHandleSearch_MissingDomain_Errors(t *testing.T) {
	resetDB(t)

	result, err := handleSearch(db)(context.TODO(), makeRequest(map[string]interface{}{"query": "anything"}))
	require.NoError(t, err)
	assert.True(t, result.IsError)
	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, `must be "kb", "backlog", or "roadmap"`)
}

// TestHandleGet_DomainBacklog_IdsFetch verifies get domain=backlog fetches items by id.
func TestHandleGet_DomainBacklog_IdsFetch(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Domain get task", "desc", "", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"domain": "backlog",
		"ids":    []interface{}{item.ID},
	})
	result, err := handleGet(db)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var items []map[string]interface{}
	require.NoError(t, unmarshalResult(result, &items))
	require.Len(t, items, 1)
	assert.Equal(t, "Domain get task", items[0]["title"])
}

// TestHandleGet_DomainBacklog_FilteredList verifies get domain=backlog lists with filters.
func TestHandleGet_DomainBacklog_FilteredList(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Open task", "", "", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"domain":   "backlog",
		"projects": []interface{}{"acme-corp"},
	})
	result, err := handleGet(db)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "Open task")
}

// TestHandleSearch_DomainBacklog_ReturnsMatches verifies search domain=backlog uses FTS over items.
func TestHandleSearch_DomainBacklog_ReturnsMatches(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "findableterm task", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P2", "unrelated task", "", "", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"domain": "backlog",
		"query":  "findableterm",
	})
	result, err := handleSearch(db)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "findableterm task")
	assert.NotContains(t, textContent.Text, "unrelated task")
}

// TestHandleSearch_DomainBacklog_MissingQuery_Errors verifies search domain=backlog requires query.
func TestHandleSearch_DomainBacklog_MissingQuery_Errors(t *testing.T) {
	resetAllDB(t)

	req := makeRequest(map[string]interface{}{"domain": "backlog"})
	result, err := handleSearch(db)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleGet_DomainBacklog_IdsFetchVerbose tests ids fetch with verbose=true (notes included).
func TestHandleGet_DomainBacklog_IdsFetchVerbose(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Verbose task", "", "secret notes", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"domain":  "backlog",
		"ids":     []interface{}{item.ID},
		"verbose": true,
	})
	result, err := handleGet(db)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var items []map[string]interface{}
	require.NoError(t, unmarshalResult(result, &items))
	require.Len(t, items, 1)
	assert.Equal(t, "secret notes", items[0]["notes"])
}

// TestHandleGet_DomainBacklog_IdsFetchMiss tests ids fetch where an ID doesn't exist returns an error.
func TestHandleGet_DomainBacklog_IdsFetchMiss(t *testing.T) {
	resetAllDB(t)

	req := makeRequest(map[string]interface{}{
		"domain": "backlog",
		"ids":    []interface{}{"AC-9999"},
	})
	result, err := handleGet(db)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError, "fetching nonexistent item should return error result")
}

// TestHandleGet_DomainBacklog_IdsFetchOmitsProjectID tests that ids-fetch response omits project_id/component (omitempty).
func TestHandleGet_DomainBacklog_IdsFetchOmitsProjectID(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "test-project", "TP")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, "TP", "P1", "No-ProjID Task", "desc", "", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"domain": "backlog",
		"ids":    []interface{}{item.ID},
	})
	result, err := handleGet(db)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var items []map[string]interface{}
	require.NoError(t, unmarshalResult(result, &items))
	require.Len(t, items, 1)

	_, hasKey := items[0]["project_id"]
	assert.False(t, hasKey, "project_id must be omitted from MCP response")
	_, hasComponent := items[0]["component"]
	assert.False(t, hasComponent, "component must be omitted when empty")
}

// TestHandleGet_DomainBacklog_NoItems_ReturnsNoItemsText verifies list mode with no matches.
func TestHandleGet_DomainBacklog_NoItems_ReturnsNoItemsText(t *testing.T) {
	resetAllDB(t)

	_, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"domain":   "backlog",
		"projects": []interface{}{"acme-corp"},
	})
	result, err := handleGet(db)(context.TODO(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
}

// TestHandleGet_DomainBacklog_PriorityFilter tests listing with priority_min and priority_max.
func TestHandleGet_DomainBacklog_PriorityFilter(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P0", "Critical", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P2", "Normal", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P5", "Low", "", "", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"domain":       "backlog",
		"projects":     []interface{}{"acme-corp"},
		"priority_min": "P1",
		"priority_max": "P3",
	})
	result, err := handleGet(db)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "Normal")
	assert.NotContains(t, textContent.Text, "Critical")
	assert.NotContains(t, textContent.Text, "Low")
}

// TestHandleGet_DomainBacklog_PriorityMinInvalid_Errors verifies error on bad priority_min.
func TestHandleGet_DomainBacklog_PriorityMinInvalid_Errors(t *testing.T) {
	resetAllDB(t)

	req := makeRequest(map[string]interface{}{
		"domain":       "backlog",
		"priority_min": "X9",
	})
	result, err := handleGet(db)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleGet_DomainBacklog_StatusFilter tests filtering by status.
func TestHandleGet_DomainBacklog_StatusFilter(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	item, err := backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "Open task", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P2", "Another task", "", "", "")
	require.NoError(t, err)
	err = backlog.MarkDone(db, item.ID)
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"domain":   "backlog",
		"projects": []interface{}{"acme-corp"},
		"status":   "open",
	})
	result, err := handleGet(db)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "Another task")
	assert.NotContains(t, textContent.Text, "Open task")
}

// TestHandleGet_DomainBacklog_ComponentFilter tests filtering by component.
func TestHandleGet_DomainBacklog_ComponentFilter(t *testing.T) {
	resetAllDB(t)

	proj, err := backlog.CreateProject(db, "acme-corp", "AC")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P1", "API task", "", "", "api")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj.ID, proj.Prefix, "P2", "UI task", "", "", "ui")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"domain":    "backlog",
		"projects":  []interface{}{"acme-corp"},
		"component": "api",
	})
	result, err := handleGet(db)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "api")
	assert.Contains(t, textContent.Text, "API task")
}

// TestHandleGet_DomainBacklog_NonexistentProject_Errors verifies error when filtering by bad project.
func TestHandleGet_DomainBacklog_NonexistentProject_Errors(t *testing.T) {
	resetAllDB(t)

	req := makeRequest(map[string]interface{}{
		"domain":   "backlog",
		"projects": []interface{}{"no-such-project"},
	})
	result, err := handleGet(db)(context.TODO(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// TestHandleGet_DomainBacklog_MultiProject tests listing across two projects.
func TestHandleGet_DomainBacklog_MultiProject(t *testing.T) {
	resetAllDB(t)

	proj1, err := backlog.CreateProject(db, "project-alpha", "PA")
	require.NoError(t, err)
	proj2, err := backlog.CreateProject(db, "project-beta", "PB")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj1.ID, proj1.Prefix, "P1", "Alpha task", "", "", "")
	require.NoError(t, err)
	_, err = backlog.AddItem(db, proj2.ID, proj2.Prefix, "P2", "Beta task", "", "", "")
	require.NoError(t, err)

	req := makeRequest(map[string]interface{}{
		"domain":   "backlog",
		"projects": []interface{}{"project-alpha", "project-beta"},
	})
	result, err := handleGet(db)(context.TODO(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "Alpha task")
	assert.Contains(t, textContent.Text, "Beta task")
}
