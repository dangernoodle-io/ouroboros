package main

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testDB creates an in-memory database for testing.
func testDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	require.NoError(t, applySchema(db))
	t.Cleanup(func() { db.Close() })
	return db
}

func TestInsertAndQueryDecision(t *testing.T) {
	db := testDB(t)

	id, err := insertDecision(db, "acme-corp", "Use PostgreSQL for main datastore", "Better ACID guarantees", []string{"database", "architecture"})
	require.NoError(t, err)
	require.Greater(t, id, int64(0))

	decisions, err := queryDecisions(db, "acme-corp", []string{}, "", 50)
	require.NoError(t, err)
	require.Len(t, decisions, 1)

	d := decisions[0]
	assert.Equal(t, id, d.ID)
	assert.Equal(t, "acme-corp", d.Project)
	assert.Equal(t, "Use PostgreSQL for main datastore", d.Summary)
	assert.ElementsMatch(t, []string{"database", "architecture"}, d.Tags)
	assert.NotEmpty(t, d.CreatedAt)

	// Verify full decision via getDecision includes rationale.
	full, err := getDecision(db, id)
	require.NoError(t, err)
	require.NotNil(t, full)
	assert.Equal(t, "Better ACID guarantees", full.Rationale)
}

func TestDecisionFTSSearch(t *testing.T) {
	db := testDB(t)

	_, err := insertDecision(db, "acme-corp", "Use PostgreSQL for performance", "ACID compliance", []string{"database"})
	require.NoError(t, err)

	_, err = insertDecision(db, "acme-corp", "Migrate to Kubernetes", "Container orchestration", []string{"infrastructure"})
	require.NoError(t, err)

	// Search by FTS query
	decisions, err := queryDecisions(db, "", []string{}, "PostgreSQL", 50)
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	assert.Contains(t, decisions[0].Summary, "PostgreSQL")
}

func TestDecisionTagFilter(t *testing.T) {
	db := testDB(t)

	_, err := insertDecision(db, "acme-corp", "Decision 1", "", []string{"database", "performance"})
	require.NoError(t, err)

	_, err = insertDecision(db, "acme-corp", "Decision 2", "", []string{"database"})
	require.NoError(t, err)

	_, err = insertDecision(db, "acme-corp", "Decision 3", "", []string{"api", "performance"})
	require.NoError(t, err)

	// Query with tag filter
	decisions, err := queryDecisions(db, "acme-corp", []string{"database", "performance"}, "", 50)
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	assert.Equal(t, "Decision 1", decisions[0].Summary)
}

func TestDeleteDecision(t *testing.T) {
	db := testDB(t)

	id, err := insertDecision(db, "acme-corp", "Decision to delete", "", []string{})
	require.NoError(t, err)

	err = deleteDecision(db, id)
	require.NoError(t, err)

	decisions, err := queryDecisions(db, "acme-corp", []string{}, "", 50)
	require.NoError(t, err)
	require.Len(t, decisions, 0)
}

func TestUpsertFact(t *testing.T) {
	db := testDB(t)

	// Insert initial fact
	id1, err := upsertFact(db, "acme-corp", "config", "database-host", "localhost")
	require.NoError(t, err)
	require.Greater(t, id1, int64(0))

	facts, err := queryFacts(db, "acme-corp", "config", "database-host", "", 50)
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, "localhost", facts[0].Value)
	createdAt1 := facts[0].CreatedAt
	firstUpdatedAt := facts[0].UpdatedAt

	// Upsert same key with new value
	id2, err := upsertFact(db, "acme-corp", "config", "database-host", "prod-db.acme-corp.example.com")
	require.NoError(t, err)

	// Should be same ID
	assert.Equal(t, id1, id2)

	facts, err = queryFacts(db, "acme-corp", "config", "database-host", "", 50)
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, "prod-db.acme-corp.example.com", facts[0].Value)
	// Verify created_at unchanged but updated_at was set
	assert.Equal(t, createdAt1, facts[0].CreatedAt)
	assert.NotEmpty(t, facts[0].UpdatedAt)
	assert.Equal(t, firstUpdatedAt, facts[0].UpdatedAt) // Same time on fast upsert
}

func TestQueryFacts(t *testing.T) {
	db := testDB(t)

	_, err := upsertFact(db, "acme-corp", "config", "app-name", "example-service")
	require.NoError(t, err)

	_, err = upsertFact(db, "acme-corp", "config", "port", "8080")
	require.NoError(t, err)

	_, err = upsertFact(db, "acme-corp", "deployment", "region", "us-west-2")
	require.NoError(t, err)

	_, err = upsertFact(db, "other-proj", "config", "app-name", "other-service")
	require.NoError(t, err)

	// Query by project only
	facts, err := queryFacts(db, "acme-corp", "", "", "", 50)
	require.NoError(t, err)
	require.Len(t, facts, 3)

	// Query by project and category
	facts, err = queryFacts(db, "acme-corp", "config", "", "", 50)
	require.NoError(t, err)
	require.Len(t, facts, 2)

	// Query by project, category, and key
	facts, err = queryFacts(db, "acme-corp", "config", "app-name", "", 50)
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, "example-service", facts[0].Value)
}

func TestQueryFactsFTS(t *testing.T) {
	db := testDB(t)

	_, err := upsertFact(db, "acme-corp", "config", "database-url", "postgresql://prod.acme-corp.example.com")
	require.NoError(t, err)

	_, err = upsertFact(db, "acme-corp", "config", "api-key", "secret123")
	require.NoError(t, err)

	// FTS search
	facts, err := queryFacts(db, "", "", "", "postgresql", 50)
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, "database-url", facts[0].Key)
}

func TestDeleteFact(t *testing.T) {
	db := testDB(t)

	_, err := upsertFact(db, "acme-corp", "config", "setting1", "value1")
	require.NoError(t, err)

	_, err = upsertFact(db, "acme-corp", "config", "setting2", "value2")
	require.NoError(t, err)

	err = deleteFact(db, "acme-corp", "config", "setting1")
	require.NoError(t, err)

	facts, err := queryFacts(db, "acme-corp", "config", "", "", 50)
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, "setting2", facts[0].Key)
}

func TestInsertAndQueryRelation(t *testing.T) {
	db := testDB(t)

	id, err := insertRelation(db, "acme-corp", "example-service", "acme-corp", "database", "depends_on", "example-service depends on database")
	require.NoError(t, err)
	require.Greater(t, id, int64(0))

	relations, err := queryRelations(db, "acme-corp", "", "", 50)
	require.NoError(t, err)
	require.Len(t, relations, 1)

	r := relations[0]
	assert.Equal(t, id, r.ID)
	assert.Equal(t, "acme-corp", r.SourceProject)
	assert.Equal(t, "example-service", r.Source)
	assert.Equal(t, "acme-corp", r.TargetProject)
	assert.Equal(t, "database", r.Target)
	assert.Equal(t, "depends_on", r.RelationType)
	assert.Equal(t, "example-service depends on database", r.Description)
	assert.NotEmpty(t, r.CreatedAt)
}

func TestQueryRelationsByEntity(t *testing.T) {
	db := testDB(t)

	// Insert relations where entity is both source and target
	id1, err := insertRelation(db, "acme-corp", "service-a", "acme-corp", "service-b", "depends_on", "")
	require.NoError(t, err)

	id2, err := insertRelation(db, "acme-corp", "service-c", "acme-corp", "service-a", "depends_on", "")
	require.NoError(t, err)

	// Query for service-a should return both relations (as source and as target)
	relations, err := queryRelations(db, "acme-corp", "service-a", "", 50)
	require.NoError(t, err)
	require.Len(t, relations, 2)

	ids := []int64{relations[0].ID, relations[1].ID}
	assert.ElementsMatch(t, []int64{id1, id2}, ids)
}

func TestDeleteRelation(t *testing.T) {
	db := testDB(t)

	id, err := insertRelation(db, "acme-corp", "service-a", "acme-corp", "service-b", "depends_on", "")
	require.NoError(t, err)

	err = deleteRelation(db, id)
	require.NoError(t, err)

	relations, err := queryRelations(db, "acme-corp", "", "", 50)
	require.NoError(t, err)
	require.Len(t, relations, 0)
}

func TestSearchAll(t *testing.T) {
	db := testDB(t)

	// Insert decisions and facts
	_, err := insertDecision(db, "acme-corp", "Use PostgreSQL", "ACID compliance", []string{"database"})
	require.NoError(t, err)

	_, err = upsertFact(db, "acme-corp", "config", "db-type", "postgresql")
	require.NoError(t, err)

	// Search
	result, err := searchAll(db, "postgresql", "", 50)
	require.NoError(t, err)

	assert.Len(t, result.Decisions, 1)
	assert.Equal(t, "Use PostgreSQL", result.Decisions[0].Summary)

	assert.Len(t, result.Facts, 1)
	assert.Equal(t, "db-type", result.Facts[0].Key)
}

func TestSearchAllWithProjectFilter(t *testing.T) {
	db := testDB(t)

	_, err := insertDecision(db, "acme-corp", "Use PostgreSQL", "", []string{})
	require.NoError(t, err)

	_, err = insertDecision(db, "other-proj", "Use MySQL", "", []string{})
	require.NoError(t, err)

	// Search in specific project only
	result, err := searchAll(db, "postgresql", "acme-corp", 50)
	require.NoError(t, err)

	assert.Len(t, result.Decisions, 1)
	assert.Equal(t, "acme-corp", result.Decisions[0].Project)
}

func TestClampLimit(t *testing.T) {
	tests := []struct {
		name       string
		limit      int
		defaultVal int
		maxVal     int
		expected   int
	}{
		{"zero returns default", 0, 50, 500, 50},
		{"negative returns default", -1, 50, 500, 50},
		{"within range", 25, 50, 500, 25},
		{"at max", 500, 50, 500, 500},
		{"exceeds max", 600, 50, 500, 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := clampLimit(tt.limit, tt.defaultVal, tt.maxVal)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUpsertNote(t *testing.T) {
	db := testDB(t)

	id1, err := upsertNote(db, "acme-corp", "procedure", "release-process", "1. Tag\n2. Push\n3. Monitor", []string{"release", "ci"})
	require.NoError(t, err)
	require.Greater(t, id1, int64(0))

	note, err := getNote(db, id1)
	require.NoError(t, err)
	require.NotNil(t, note)
	assert.Equal(t, "acme-corp", note.Project)
	assert.Equal(t, "procedure", note.Category)
	assert.Equal(t, "release-process", note.Title)
	assert.Equal(t, "1. Tag\n2. Push\n3. Monitor", note.Body)
	assert.ElementsMatch(t, []string{"release", "ci"}, note.Tags)

	// Upsert same key with new body
	id2, err := upsertNote(db, "acme-corp", "procedure", "release-process", "Updated steps", []string{"release"})
	require.NoError(t, err)
	assert.Equal(t, id1, id2)

	note, err = getNote(db, id1)
	require.NoError(t, err)
	assert.Equal(t, "Updated steps", note.Body)
	assert.ElementsMatch(t, []string{"release"}, note.Tags)
}

func TestQueryNotes(t *testing.T) {
	db := testDB(t)

	_, err := upsertNote(db, "acme-corp", "procedure", "release-process", "steps here", []string{"release"})
	require.NoError(t, err)
	_, err = upsertNote(db, "acme-corp", "guide", "onboarding", "welcome guide", []string{"team"})
	require.NoError(t, err)
	_, err = upsertNote(db, "other-proj", "procedure", "deploy", "deploy steps", nil)
	require.NoError(t, err)

	// Query by project
	notes, err := queryNotes(db, "acme-corp", "", "", nil, 50)
	require.NoError(t, err)
	assert.Len(t, notes, 2)
	// Verify no body in summaries
	for _, n := range notes {
		assert.NotEmpty(t, n.Title)
	}

	// Query by category
	notes, err = queryNotes(db, "", "procedure", "", nil, 50)
	require.NoError(t, err)
	assert.Len(t, notes, 2)

	// Query by project + category
	notes, err = queryNotes(db, "acme-corp", "procedure", "", nil, 50)
	require.NoError(t, err)
	assert.Len(t, notes, 1)
	assert.Equal(t, "release-process", notes[0].Title)
}

func TestQueryNotesFTS(t *testing.T) {
	db := testDB(t)

	_, err := upsertNote(db, "acme-corp", "procedure", "release-process", "Tag and push to trigger goreleaser", nil)
	require.NoError(t, err)
	_, err = upsertNote(db, "acme-corp", "guide", "onboarding", "Welcome to the team", nil)
	require.NoError(t, err)

	notes, err := queryNotes(db, "", "", "goreleaser", nil, 50)
	require.NoError(t, err)
	assert.Len(t, notes, 1)
	assert.Equal(t, "release-process", notes[0].Title)
}

func TestQueryNotesTagFilter(t *testing.T) {
	db := testDB(t)

	_, err := upsertNote(db, "acme-corp", "procedure", "release", "body", []string{"ci", "release"})
	require.NoError(t, err)
	_, err = upsertNote(db, "acme-corp", "procedure", "deploy", "body", []string{"ci"})
	require.NoError(t, err)

	notes, err := queryNotes(db, "", "", "", []string{"ci", "release"}, 50)
	require.NoError(t, err)
	assert.Len(t, notes, 1)
	assert.Equal(t, "release", notes[0].Title)
}

func TestGetNoteNotFound(t *testing.T) {
	db := testDB(t)

	note, err := getNote(db, 999)
	require.NoError(t, err)
	assert.Nil(t, note)
}

func TestDeleteNote(t *testing.T) {
	db := testDB(t)

	id, err := upsertNote(db, "acme-corp", "procedure", "release", "body", nil)
	require.NoError(t, err)

	err = deleteNote(db, id)
	require.NoError(t, err)

	note, err := getNote(db, id)
	require.NoError(t, err)
	assert.Nil(t, note)
}
