package tui

import (
	"testing"

	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
	"dangernoodle.io/ouroboros/internal/store"
)

// TestProjectFilterCycle tests the project cycling logic.
func TestProjectFilterCycle(t *testing.T) {
	projects := []backlog.Project{
		{ID: 1, Name: "acme-corp", Prefix: "AC"},
		{ID: 2, Name: "test-proj", Prefix: "TP"},
	}

	pf := &ProjectFilter{
		projects: projects,
		selected: 0,
	}
	pf.updateSlices()

	// Initially at All
	require.Equal(t, "All", pf.Label())
	require.Equal(t, []int64{1, 2}, pf.IDSlice())

	// Cycle to first project
	pf.Cycle()
	require.Equal(t, "acme-corp", pf.Label())
	require.Equal(t, []int64{1}, pf.IDSlice())

	// Cycle to second project
	pf.Cycle()
	require.Equal(t, "test-proj", pf.Label())
	require.Equal(t, []int64{2}, pf.IDSlice())

	// Cycle back to All
	pf.Cycle()
	require.Equal(t, "All", pf.Label())
	require.Equal(t, []int64{1, 2}, pf.IDSlice())
}

// TestProjectFilterNameSlice tests the name slice generation.
func TestProjectFilterNameSlice(t *testing.T) {
	projects := []backlog.Project{
		{ID: 1, Name: "acme-corp", Prefix: "AC"},
		{ID: 2, Name: "example-inc", Prefix: "EX"},
	}

	pf := &ProjectFilter{
		projects: projects,
		selected: 0,
	}
	pf.updateSlices()

	// All projects
	require.Equal(t, []string{"acme-corp", "example-inc"}, pf.NameSlice())

	// First project only
	pf.selected = 1
	pf.updateSlices()
	require.Equal(t, []string{"acme-corp"}, pf.NameSlice())

	// Second project only
	pf.selected = 2
	pf.updateSlices()
	require.Equal(t, []string{"example-inc"}, pf.NameSlice())
}

// TestTruncate tests string truncation.
func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		expect string
	}{
		{"short", 10, "short"},
		{"this is a longer string", 10, "this is a …"},
		{"exact", 5, "exact"},
		{"exact", 4, "exac…"},
		{"", 10, ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			require.Equal(t, tt.expect, got)
		})
	}
}

// TestPriorityStyle tests priority styling.
func TestPriorityStyle(t *testing.T) {
	styles := DefaultStyles()

	tests := []string{"P0", "P1", "P2", "P3", "P4", "P5", "P6"}
	for _, priority := range tests {
		t.Run(priority, func(t *testing.T) {
			style := styles.PriorityStyle(priority)
			require.NotNil(t, style)
		})
	}

	// Test unknown priority
	unknownStyle := styles.PriorityStyle("UNKNOWN")
	require.NotNil(t, unknownStyle)
}

// TestStatusStyle tests status styling.
func TestStatusStyle(t *testing.T) {
	styles := DefaultStyles()

	tests := []string{"open", "active", "done", "canceled", "draft"}
	for _, status := range tests {
		t.Run(status, func(t *testing.T) {
			style := styles.StatusStyle(status)
			require.NotNil(t, style)
		})
	}

	// Test unknown status
	unknownStyle := styles.StatusStyle("UNKNOWN")
	require.NotNil(t, unknownStyle)
}

// TestItemToRow_NoProject tests itemToRow without project column.
func TestItemToRow_NoProject(t *testing.T) {
	item := backlog.Item{
		ID:        "AC-1",
		Priority:  "P0",
		Status:    "open",
		Title:     "example task",
		Component: "backend",
	}
	styles := DefaultStyles()

	row := itemToRow(item, false, "", styles)
	require.Equal(t, 5, len(row)) // ID, P, Status, Title, Component
	require.Equal(t, "AC-1", row[0])
	require.Contains(t, row[1], "P0") // Priority cell contains styled P0
	require.Contains(t, row[2], "open")
	require.Contains(t, row[3], "example task")
	require.Equal(t, "backend", row[4])
}

// TestItemToRow_WithProject tests itemToRow with project column.
func TestItemToRow_WithProject(t *testing.T) {
	item := backlog.Item{
		ID:        "AC-1",
		Priority:  "P2",
		Status:    "done",
		Title:     "another task",
		Component: "api",
	}
	styles := DefaultStyles()

	row := itemToRow(item, true, "acme-corp", styles)
	require.Equal(t, 6, len(row)) // ID, P, Status, Project, Title, Component
	require.Equal(t, "AC-1", row[0])
	require.Contains(t, row[1], "P2")
	require.Contains(t, row[2], "done")
	require.Equal(t, "acme-corp", row[3])
	require.Contains(t, row[4], "another task")
	require.Equal(t, "api", row[5])
}

// TestBacklogColumns_Shape tests backlog column structure.
func TestBacklogColumns_Shape(t *testing.T) {
	// Single project: 5 columns (ID, P, Status, Title, Component)
	colsSingle := backlogColumns(false, 30)
	require.Equal(t, 5, len(colsSingle))
	require.Equal(t, "ID", colsSingle[0].Title)
	require.Equal(t, "P", colsSingle[1].Title)
	require.Equal(t, "Status", colsSingle[2].Title)
	require.Equal(t, "Title", colsSingle[3].Title)
	require.Equal(t, "Component", colsSingle[4].Title)

	// All projects: 6 columns (ID, P, Status, Project, Title, Component)
	colsAll := backlogColumns(true, 30)
	require.Equal(t, 6, len(colsAll))
	require.Equal(t, "ID", colsAll[0].Title)
	require.Equal(t, "P", colsAll[1].Title)
	require.Equal(t, "Status", colsAll[2].Title)
	require.Equal(t, "Project", colsAll[3].Title)
	require.Equal(t, "Title", colsAll[4].Title)
	require.Equal(t, "Component", colsAll[5].Title)
}

// TestKBRowFormatting tests KB row formatting.
func TestKBRowFormatting(t *testing.T) {
	tests := []struct {
		name     string
		doc      *store.DocumentSummary
		hasTitle bool
	}{
		{
			name: "with category",
			doc: &store.DocumentSummary{
				ID:       1,
				Type:     "decision",
				Project:  "acme-corp",
				Category: "arch",
				Title:    "microservices design",
			},
			hasTitle: true,
		},
		{
			name: "without category",
			doc: &store.DocumentSummary{
				ID:      2,
				Type:    "note",
				Project: "test-proj",
				Title:   "quick note",
			},
			hasTitle: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := kbRow{doc: tt.doc}
			if tt.hasTitle {
				require.NotEmpty(t, row.Title())
			}
			desc := row.Description()
			require.NotEmpty(t, desc)
		})
	}
}

// TestPlanToRow_NoProject tests planToRow without project column.
func TestPlanToRow_NoProject(t *testing.T) {
	plan := backlog.Plan{
		ID:     42,
		Title:  "implementation roadmap",
		Status: "active",
	}
	styles := DefaultStyles()

	row := planToRow(plan, false, "", styles)
	require.Equal(t, 3, len(row)) // #, Status, Title
	require.Equal(t, "42", row[0])
	require.Contains(t, row[1], "active")
	require.Contains(t, row[2], "implementation roadmap")
}

// TestPlanToRow_WithProject tests planToRow with project column.
func TestPlanToRow_WithProject(t *testing.T) {
	plan := backlog.Plan{
		ID:     99,
		Title:  "migration plan",
		Status: "draft",
	}
	styles := DefaultStyles()

	row := planToRow(plan, true, "acme-corp", styles)
	require.Equal(t, 4, len(row)) // #, Status, Project, Title
	require.Equal(t, "99", row[0])
	require.Contains(t, row[1], "draft")
	require.Equal(t, "acme-corp", row[2])
	require.Contains(t, row[3], "migration plan")
}

// TestPlanToRow_NilProjectID tests planToRow with nil ProjectID.
func TestPlanToRow_NilProjectID(t *testing.T) {
	plan := backlog.Plan{
		ID:        77,
		Title:     "orphaned plan",
		Status:    "draft",
		ProjectID: nil, // nil ProjectID
	}
	styles := DefaultStyles()

	row := planToRow(plan, true, "", styles)
	require.Equal(t, 4, len(row))
	require.Equal(t, "77", row[0])
	require.Contains(t, row[1], "draft")
	require.Equal(t, "", row[2]) // empty project cell
	require.Contains(t, row[3], "orphaned plan")
}

// TestPlanColumns_Shape tests plan column structure.
func TestPlanColumns_Shape(t *testing.T) {
	// Single project: 3 columns (#, Status, Title)
	colsSingle := planColumns(false, 30)
	require.Equal(t, 3, len(colsSingle))
	require.Equal(t, "#", colsSingle[0].Title)
	require.Equal(t, "Status", colsSingle[1].Title)
	require.Equal(t, "Title", colsSingle[2].Title)

	// All projects: 4 columns (#, Status, Project, Title)
	colsAll := planColumns(true, 30)
	require.Equal(t, 4, len(colsAll))
	require.Equal(t, "#", colsAll[0].Title)
	require.Equal(t, "Status", colsAll[1].Title)
	require.Equal(t, "Project", colsAll[2].Title)
	require.Equal(t, "Title", colsAll[3].Title)
}

// TestProjectFilterIsAll tests IsAll method.
func TestProjectFilterIsAll(t *testing.T) {
	projects := []backlog.Project{
		{ID: 1, Name: "acme-corp", Prefix: "AC"},
		{ID: 2, Name: "other-project", Prefix: "OP"},
	}

	pf := &ProjectFilter{
		projects: projects,
		selected: 0,
	}
	pf.updateSlices()

	// Should be true when selected is 0 (All)
	require.True(t, pf.IsAll())

	// Should be false when a specific project is selected
	pf.selected = 1
	require.False(t, pf.IsAll())

	pf.selected = 2
	require.False(t, pf.IsAll())
}

// TestProjectFilterNameByID tests NameByID method.
func TestProjectFilterNameByID(t *testing.T) {
	projects := []backlog.Project{
		{ID: 1, Name: "acme-corp", Prefix: "AC"},
		{ID: 2, Name: "other-project", Prefix: "OP"},
	}

	pf := &ProjectFilter{
		projects: projects,
		selected: 0,
	}

	// Test hit
	require.Equal(t, "acme-corp", pf.NameByID(1))
	require.Equal(t, "other-project", pf.NameByID(2))

	// Test miss
	require.Equal(t, "", pf.NameByID(999))
}

// TestProjectFilterAllByID tests AllByID method.
func TestProjectFilterAllByID(t *testing.T) {
	projects := []backlog.Project{
		{ID: 1, Name: "acme-corp", Prefix: "AC"},
		{ID: 2, Name: "other-project", Prefix: "OP"},
	}

	pf := &ProjectFilter{
		projects: projects,
		selected: 0,
	}

	allByID := pf.AllByID()
	require.Equal(t, 2, len(allByID))
	require.Equal(t, "acme-corp", allByID[1])
	require.Equal(t, "other-project", allByID[2])

	// Test that it returns all projects even when filter is on specific project
	pf.selected = 1
	allByID = pf.AllByID()
	require.Equal(t, 2, len(allByID))
	require.Equal(t, "acme-corp", allByID[1])
	require.Equal(t, "other-project", allByID[2])
}

// TestDefaultStyles tests that all styles are initialized.
func TestDefaultStyles(t *testing.T) {
	styles := DefaultStyles()
	require.NotNil(t, styles.ActiveTab)
	require.NotNil(t, styles.InactiveTab)
	require.NotNil(t, styles.ListTitle)
	require.NotNil(t, styles.PriorityP0)
	require.NotNil(t, styles.StatusOpen)
	require.NotNil(t, styles.DetailBorder)
	require.NotNil(t, styles.FooterError)
}
