package tui

import (
	"database/sql"
	"fmt"

	"dangernoodle.io/ouroboros/internal/backlog"
)

// ProjectFilter manages project selection state across tabs.
type ProjectFilter struct {
	projects  []backlog.Project
	selected  int // 0 = All, 1..N = specific project
	idSlice   []int64
	nameSlice []string
}

// NewProjectFilter loads all projects and initializes filter to "All".
func NewProjectFilter(db *sql.DB) (*ProjectFilter, error) {
	projects, err := backlog.ListProjects(db)
	if err != nil {
		return nil, fmt.Errorf("load projects: %w", err)
	}
	pf := &ProjectFilter{
		projects: projects,
		selected: 0,
	}
	pf.updateSlices()
	return pf, nil
}

// updateSlices recalculates ID and name slices based on current selection.
func (pf *ProjectFilter) updateSlices() {
	if pf.selected == 0 {
		// All projects
		pf.idSlice = make([]int64, len(pf.projects))
		pf.nameSlice = make([]string, len(pf.projects))
		for i, p := range pf.projects {
			pf.idSlice[i] = p.ID
			pf.nameSlice[i] = p.Name
		}
	} else if pf.selected > 0 && pf.selected <= len(pf.projects) {
		// Single project (1-indexed)
		p := pf.projects[pf.selected-1]
		pf.idSlice = []int64{p.ID}
		pf.nameSlice = []string{p.Name}
	}
}

// Cycle moves to the next project (All → proj1 → proj2 → ... → All).
func (pf *ProjectFilter) Cycle() {
	pf.selected = (pf.selected + 1) % (len(pf.projects) + 1)
	pf.updateSlices()
}

// IDSlice returns the project IDs for filtering backlog/plans.
func (pf *ProjectFilter) IDSlice() []int64 {
	return pf.idSlice
}

// NameSlice returns the project names for filtering KB documents.
func (pf *ProjectFilter) NameSlice() []string {
	return pf.nameSlice
}

// Label returns the user-visible label (e.g., "All" or "acme-corp").
func (pf *ProjectFilter) Label() string {
	if pf.selected == 0 {
		return "All"
	}
	if pf.selected > 0 && pf.selected <= len(pf.projects) {
		return pf.projects[pf.selected-1].Name
	}
	return "All"
}
