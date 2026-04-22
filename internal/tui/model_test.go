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

// TestItemRowFormatting tests backlog item row formatting.
func TestItemRowFormatting(t *testing.T) {
	tests := []struct {
		name    string
		item    *backlog.Item
		hasID   bool
		hasDesc bool
	}{
		{
			name: "with component",
			item: &backlog.Item{
				ID:        "AC-1",
				Priority:  "P0",
				Status:    "open",
				Title:     "example task",
				Component: "backend",
			},
			hasID:   true,
			hasDesc: true,
		},
		{
			name: "without component",
			item: &backlog.Item{
				ID:       "AC-1",
				Priority: "P2",
				Status:   "done",
				Title:    "another task",
			},
			hasID:   true,
			hasDesc: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := itemRow{item: tt.item}
			if tt.hasID {
				require.Equal(t, "AC-1", row.Title())
			}
			if tt.hasDesc {
				desc := row.Description()
				require.NotEmpty(t, desc)
			}
		})
	}
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

// TestPlanRowFormatting tests plan row formatting.
func TestPlanRowFormatting(t *testing.T) {
	plan := &backlog.Plan{
		ID:     42,
		Title:  "implementation roadmap",
		Status: "active",
	}

	row := planRow{plan: plan}
	require.NotEmpty(t, row.Title())
	require.Contains(t, row.Title(), "42")
	require.NotEmpty(t, row.Description())
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
