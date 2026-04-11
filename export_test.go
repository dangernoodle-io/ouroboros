package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExportMarkdownEmpty(t *testing.T) {
	db := testDB(t)

	markdown, err := exportMarkdown(db, "")
	require.NoError(t, err)

	// Verify header is present
	assert.Contains(t, markdown, "# Knowledge Base Export")
	assert.Contains(t, markdown, "All Projects")
	assert.Contains(t, markdown, "## Decisions")
	assert.Contains(t, markdown, "## Facts")
	assert.Contains(t, markdown, "## Relations")

	// Verify empty section messages
	assert.Contains(t, markdown, "_No decisions found._")
	assert.Contains(t, markdown, "_No facts found._")
	assert.Contains(t, markdown, "_No relations found._")
}

func TestExportMarkdownWithData(t *testing.T) {
	db := testDB(t)

	// Insert test data
	_, err := insertDecision(db, "acme-corp", "Use PostgreSQL", "ACID compliance", []string{"database", "architecture"})
	require.NoError(t, err)

	_, err = insertDecision(db, "acme-corp", "Containerize services", "Deploy with Docker", []string{"infrastructure"})
	require.NoError(t, err)

	_, err = upsertFact(db, "acme-corp", "config", "db-host", "prod.acme-corp.example.com")
	require.NoError(t, err)

	_, err = upsertFact(db, "acme-corp", "deployment", "region", "us-west-2")
	require.NoError(t, err)

	_, err = insertRelation(db, "acme-corp", "app-service", "acme-corp", "database", "depends_on", "app requires database")
	require.NoError(t, err)

	// Export
	markdown, err := exportMarkdown(db, "acme-corp")
	require.NoError(t, err)

	// Verify decisions section
	assert.Contains(t, markdown, "## Decisions")
	assert.Contains(t, markdown, "Use PostgreSQL")
	assert.Contains(t, markdown, "ACID compliance")
	assert.Contains(t, markdown, "database, architecture")
	assert.Contains(t, markdown, "Containerize services")
	assert.Contains(t, markdown, "infrastructure")

	// Verify facts section
	assert.Contains(t, markdown, "## Facts")
	assert.Contains(t, markdown, "config")
	assert.Contains(t, markdown, "db-host")
	assert.Contains(t, markdown, "prod.acme-corp.example.com")
	assert.Contains(t, markdown, "deployment")
	assert.Contains(t, markdown, "region")
	assert.Contains(t, markdown, "us-west-2")

	// Verify relations section
	assert.Contains(t, markdown, "## Relations")
	assert.Contains(t, markdown, "app-service")
	assert.Contains(t, markdown, "database")
	assert.Contains(t, markdown, "depends_on")
	assert.Contains(t, markdown, "app requires database")

	// Verify project label
	assert.Contains(t, markdown, "Project: acme-corp")
}

func TestExportMarkdownProjectFilter(t *testing.T) {
	db := testDB(t)

	// Insert data for two projects
	_, err := insertDecision(db, "acme-corp", "Decision 1", "", []string{})
	require.NoError(t, err)

	_, err = insertDecision(db, "other-proj", "Decision 2", "", []string{})
	require.NoError(t, err)

	// Export for specific project
	markdown, err := exportMarkdown(db, "acme-corp")
	require.NoError(t, err)

	// Verify only acme-corp decision is present
	assert.Contains(t, markdown, "Decision 1")
	assert.NotContains(t, markdown, "Decision 2")
	assert.Contains(t, markdown, "Project: acme-corp")
}

func TestImportJSON(t *testing.T) {
	db := testDB(t)

	// Create import payload
	payload := ImportData{
		Decisions: []ImportDecision{
			{
				Project:   "acme-corp",
				Summary:   "Use PostgreSQL",
				Rationale: "ACID compliance",
				Tags:      []string{"database", "architecture"},
			},
		},
		Facts: []ImportFact{
			{
				Project:  "acme-corp",
				Category: "config",
				Key:      "db-host",
				Value:    "prod.acme-corp.example.com",
			},
		},
		Relations: []ImportRelation{
			{
				SourceProject: "acme-corp",
				Source:        "app-service",
				TargetProject: "acme-corp",
				Target:        "database",
				RelationType:  "depends_on",
				Description:   "app requires database",
			},
		},
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	// Import
	err = importJSON(db, "", data)
	require.NoError(t, err)

	// Verify decision imported (summaries don't include rationale).
	decisions, err := queryDecisions(db, "acme-corp", nil, "", 50)
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	assert.Equal(t, "Use PostgreSQL", decisions[0].Summary)
	assert.ElementsMatch(t, []string{"database", "architecture"}, decisions[0].Tags)

	// Verify rationale via getDecision.
	full, err := getDecision(db, decisions[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "ACID compliance", full.Rationale)

	// Verify fact imported
	facts, err := queryFacts(db, "acme-corp", "config", "db-host", "", 50)
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, "prod.acme-corp.example.com", facts[0].Value)

	// Verify relation imported
	relations, err := queryRelations(db, "acme-corp", "", "", 50)
	require.NoError(t, err)
	require.Len(t, relations, 1)
	assert.Equal(t, "app-service", relations[0].Source)
	assert.Equal(t, "database", relations[0].Target)
	assert.Equal(t, "depends_on", relations[0].RelationType)
}

func TestImportJSONDefaultProject(t *testing.T) {
	db := testDB(t)

	// Create import payload with items missing project field
	payload := ImportData{
		Decisions: []ImportDecision{
			{
				Summary: "Decision 1",
			},
		},
		Facts: []ImportFact{
			{
				Category: "config",
				Key:      "setting",
				Value:    "value",
			},
		},
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	// Import with default project
	err = importJSON(db, "acme-corp", data)
	require.NoError(t, err)

	// Verify decision used default project
	decisions, err := queryDecisions(db, "acme-corp", nil, "", 50)
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	assert.Equal(t, "acme-corp", decisions[0].Project)

	// Verify fact used default project
	facts, err := queryFacts(db, "acme-corp", "", "", "", 50)
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, "acme-corp", facts[0].Project)
}

func TestImportJSONMissingProject(t *testing.T) {
	db := testDB(t)

	// Create payload with missing project and no default
	payload := ImportData{
		Decisions: []ImportDecision{
			{
				Summary: "Decision without project",
			},
		},
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	// Import should fail
	err = importJSON(db, "", data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing project")
}

func TestImportDataAutoDetectJSON(t *testing.T) {
	db := testDB(t)

	// Create JSON string
	payload := ImportData{
		Decisions: []ImportDecision{
			{
				Project: "acme-corp",
				Summary: "Decision 1",
			},
		},
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)

	// Import with auto-detection
	err = importData(db, "", string(data))
	require.NoError(t, err)

	// Verify imported
	decisions, err := queryDecisions(db, "acme-corp", nil, "", 50)
	require.NoError(t, err)
	require.Len(t, decisions, 1)
}

func TestImportDataAutoDetectJSONArray(t *testing.T) {
	db := testDB(t)

	// Create JSON array string
	jsonStr := `[{"project": "acme-corp", "summary": "Decision"}]`

	// Import with auto-detection (should fail because format is not ImportData)
	err := importData(db, "", jsonStr)
	require.Error(t, err)
}

func TestImportDataUnsupportedFormat(t *testing.T) {
	db := testDB(t)

	// Try to import markdown
	markdown := `# Decisions

## Decision 1
Summary: Test decision`

	err := importData(db, "", markdown)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
	assert.Contains(t, err.Error(), "JSON")
}

func TestImportDataEmpty(t *testing.T) {
	db := testDB(t)

	err := importData(db, "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestRoundTripExportAndImport(t *testing.T) {
	db1 := testDB(t)
	db2 := testDB(t)

	// Insert data into db1
	_, err := insertDecision(db1, "acme-corp", "Use PostgreSQL", "ACID compliance", []string{"database"})
	require.NoError(t, err)

	_, err = insertDecision(db1, "acme-corp", "Docker deployment", "Container orchestration", []string{"infrastructure"})
	require.NoError(t, err)

	_, err = upsertFact(db1, "acme-corp", "config", "db-host", "prod.acme-corp.example.com")
	require.NoError(t, err)

	_, err = upsertFact(db1, "acme-corp", "deployment", "region", "us-west-2")
	require.NoError(t, err)

	_, err = insertRelation(db1, "acme-corp", "app", "acme-corp", "database", "depends_on", "")
	require.NoError(t, err)

	// Export markdown from db1
	markdown, err := exportMarkdown(db1, "acme-corp")
	require.NoError(t, err)
	assert.NotEmpty(t, markdown)
	assert.Contains(t, markdown, "Use PostgreSQL")
	assert.Contains(t, markdown, "Docker deployment")

	// Manually create JSON with same data for import into db2
	importPayload := ImportData{
		Decisions: []ImportDecision{
			{
				Project:   "acme-corp",
				Summary:   "Use PostgreSQL",
				Rationale: "ACID compliance",
				Tags:      []string{"database"},
			},
			{
				Project:   "acme-corp",
				Summary:   "Docker deployment",
				Rationale: "Container orchestration",
				Tags:      []string{"infrastructure"},
			},
		},
		Facts: []ImportFact{
			{
				Project:  "acme-corp",
				Category: "config",
				Key:      "db-host",
				Value:    "prod.acme-corp.example.com",
			},
			{
				Project:  "acme-corp",
				Category: "deployment",
				Key:      "region",
				Value:    "us-west-2",
			},
		},
		Relations: []ImportRelation{
			{
				SourceProject: "acme-corp",
				Source:        "app",
				TargetProject: "acme-corp",
				Target:        "database",
				RelationType:  "depends_on",
			},
		},
	}

	data, err := json.Marshal(importPayload)
	require.NoError(t, err)

	// Import into db2
	err = importJSON(db2, "", data)
	require.NoError(t, err)

	// Verify counts match between databases
	decisions1, err := queryDecisions(db1, "acme-corp", nil, "", 500)
	require.NoError(t, err)

	decisions2, err := queryDecisions(db2, "acme-corp", nil, "", 500)
	require.NoError(t, err)

	assert.Equal(t, len(decisions1), len(decisions2))
	assert.Equal(t, 2, len(decisions2))

	facts1, err := queryFacts(db1, "acme-corp", "", "", "", 500)
	require.NoError(t, err)

	facts2, err := queryFacts(db2, "acme-corp", "", "", "", 500)
	require.NoError(t, err)

	assert.Equal(t, len(facts1), len(facts2))
	assert.Equal(t, 2, len(facts2))

	relations1, err := queryRelations(db1, "acme-corp", "", "", 500)
	require.NoError(t, err)

	relations2, err := queryRelations(db2, "acme-corp", "", "", 500)
	require.NoError(t, err)

	assert.Equal(t, len(relations1), len(relations2))
	assert.Equal(t, 1, len(relations2))
}

func TestImportJSONWhitespace(t *testing.T) {
	db := testDB(t)

	// JSON with extra whitespace
	jsonStr := `
	{
		"decisions": [
			{
				"project": "acme-corp",
				"summary": "Test Decision"
			}
		],
		"facts": [],
		"relations": []
	}`

	err := importData(db, "", jsonStr)
	require.NoError(t, err)

	decisions, err := queryDecisions(db, "acme-corp", nil, "", 50)
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	assert.Equal(t, "Test Decision", decisions[0].Summary)
}

func TestExportMarkdownEscaping(t *testing.T) {
	db := testDB(t)

	// Insert data with special markdown characters
	_, err := insertDecision(db, "acme-corp", "Use | pipe symbol", "Test & ampersand", []string{"tag1", "tag2"})
	require.NoError(t, err)

	markdown, err := exportMarkdown(db, "acme-corp")
	require.NoError(t, err)

	// Verify the data is present (markdown tables will handle pipes naturally)
	assert.Contains(t, markdown, "Use | pipe symbol")
	assert.Contains(t, markdown, "Test & ampersand")
}

func TestImportMultipleProjects(t *testing.T) {
	db := testDB(t)

	// Create import payload with multiple projects
	payload := ImportData{
		Decisions: []ImportDecision{
			{
				Project: "acme-corp",
				Summary: "Decision 1",
			},
			{
				Project: "other-proj",
				Summary: "Decision 2",
			},
		},
		Facts: []ImportFact{
			{
				Project:  "acme-corp",
				Category: "config",
				Key:      "key1",
				Value:    "value1",
			},
			{
				Project:  "other-proj",
				Category: "config",
				Key:      "key2",
				Value:    "value2",
			},
		},
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	err = importJSON(db, "", data)
	require.NoError(t, err)

	// Verify both projects have data
	decisions1, err := queryDecisions(db, "acme-corp", nil, "", 50)
	require.NoError(t, err)
	assert.Len(t, decisions1, 1)

	decisions2, err := queryDecisions(db, "other-proj", nil, "", 50)
	require.NoError(t, err)
	assert.Len(t, decisions2, 1)

	facts1, err := queryFacts(db, "acme-corp", "", "", "", 50)
	require.NoError(t, err)
	assert.Len(t, facts1, 1)

	facts2, err := queryFacts(db, "other-proj", "", "", "", 50)
	require.NoError(t, err)
	assert.Len(t, facts2, 1)
}
