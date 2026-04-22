package tui

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"dangernoodle.io/ouroboros/internal/backlog"
)

// PlansModel represents the plans tab.
type PlansModel struct {
	db       *sql.DB
	table    table.Model
	viewport viewport.Model
	styles   Styles

	plans        []backlog.Plan
	selectedPlan *backlog.Plan
	focusList    bool
	loading      bool
	error        string
}

// NewPlansModel creates a new plans tab model.
func NewPlansModel(db *sql.DB, styles Styles) *PlansModel {
	tbl := table.New(table.WithFocused(true), table.WithHeight(0))
	vp := viewport.New(0, 0)

	return &PlansModel{
		db:        db,
		table:     tbl,
		viewport:  vp,
		styles:    styles,
		focusList: true,
		loading:   true,
	}
}

// Init implements tea.Model.
func (m *PlansModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m *PlansModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loadPlansMsg:
		m.plans = msg.plans
		m.loading = false
		m.error = ""
		if len(m.plans) > 0 {
			if msg.showProject {
				sort.SliceStable(m.plans, func(i, j int) bool {
					iProj := int64(0)
					if m.plans[i].ProjectID != nil {
						iProj = *m.plans[i].ProjectID
					}
					jProj := int64(0)
					if m.plans[j].ProjectID != nil {
						jProj = *m.plans[j].ProjectID
					}
					if iProj != jProj {
						return iProj < jProj
					}
					return m.plans[i].ID < m.plans[j].ID
				})
			}
			cols := planColumns(msg.showProject, 30) // placeholder width
			rows := []table.Row{}
			for i := range m.plans {
				projectName := ""
				if msg.showProject && m.plans[i].ProjectID != nil {
					projectName = msg.projectNames[*m.plans[i].ProjectID]
				}
				row := planToRow(m.plans[i], msg.showProject, projectName, m.styles)
				rows = append(rows, row)
			}
			m.table.SetColumns(cols)
			m.table.SetRows(rows)
		}
		return m, nil

	case plansErrorMsg:
		m.loading = false
		m.error = msg.err.Error()
		return m, nil

	case fetchPlansDetailMsg:
		plan, err := backlog.GetPlan(m.db, msg.id)
		if err != nil {
			return m, func() tea.Msg {
				return plansErrorMsg{err: err}
			}
		}
		m.selectedPlan = plan
		m.viewport.SetContent(formatPlanDetail(plan, m.styles))
		m.focusList = false
		return m, nil

	case tea.KeyMsg:
		if msg.Type == tea.KeyEsc {
			if !m.focusList {
				m.focusList = true
				m.selectedPlan = nil
				m.viewport.SetContent("")
				return m, nil
			}
		}
		if msg.Type == tea.KeyEnter && m.focusList {
			if len(m.plans) > 0 && m.table.Cursor() < len(m.plans) {
				selectedPlan := m.plans[m.table.Cursor()]
				return m, func() tea.Msg {
					return fetchPlansDetailMsg{id: selectedPlan.ID}
				}
			}
		}

		if m.focusList {
			var cmd tea.Cmd
			m.table, cmd = m.table.Update(msg)
			return m, cmd
		}
		if !m.focusList {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m *PlansModel) View() string {
	if m.loading {
		return "Loading plans...\n"
	}

	if m.error != "" {
		return m.styles.FooterError.Render("Error: "+m.error) + "\n"
	}

	if len(m.plans) == 0 {
		return "No plans found.\n"
	}

	tableView := m.table.View()
	if m.focusList {
		return tableView
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, tableView, "  ", m.viewport.View())
}

// LoadPlans dispatches a command to load plans for the given project filter.
// When showProject is true, rows display their project name and results are
// clustered by project; projectNames maps ProjectID to display name.
func (m *PlansModel) LoadPlans(projIDs []int64, showProject bool, projectNames map[int64]string) tea.Cmd {
	m.loading = true
	m.plans = nil
	m.selectedPlan = nil
	m.focusList = true
	m.viewport.SetContent("")

	return func() tea.Msg {
		plans, err := backlog.ListPlans(m.db, backlog.PlanFilter{ProjectIDs: projIDs})
		if err != nil {
			return plansErrorMsg{err: err}
		}
		return loadPlansMsg{plans: plans, showProject: showProject, projectNames: projectNames}
	}
}

// SetSize updates the size of table and viewport.
func (m *PlansModel) SetSize(width, height int) {
	tableWidth := width/2 - 2
	tableHeight := height - 2

	// Compute title width based on showProject
	showProject := len(m.table.Columns()) > 3 // 4 cols when All, 3 when single project
	titleWidth := computePlanTitleWidth(tableWidth, showProject)

	// Rebuild columns with correct width
	cols := planColumns(showProject, titleWidth)
	m.table.SetColumns(cols)
	m.table.SetWidth(tableWidth)
	m.table.SetHeight(tableHeight)

	m.viewport.Width = width / 2
	m.viewport.Height = tableHeight
}

// computePlanTitleWidth calculates the title column width for plans.
func computePlanTitleWidth(totalWidth int, showProject bool) int {
	idWidth := 8
	statusWidth := 10
	padding := 2 * 3 // 2 per column (3 columns base, 4 with project)

	used := idWidth + statusWidth + padding
	if showProject {
		projectWidth := 14
		used = idWidth + statusWidth + projectWidth + padding
	}

	titleWidth := totalWidth - used
	if titleWidth < 20 {
		titleWidth = 20
	}
	return titleWidth
}

// planColumns builds column definitions for the plans table.
func planColumns(showProject bool, titleWidth int) []table.Column {
	cols := []table.Column{
		{Title: "#", Width: 8},
		{Title: "Status", Width: 10},
	}
	if showProject {
		cols = append(cols, table.Column{Title: "Project", Width: 14})
	}
	cols = append(cols, table.Column{Title: "Title", Width: titleWidth})
	return cols
}

// planToRow converts a plan to a table row.
func planToRow(plan backlog.Plan, showProject bool, projectName string, styles Styles) table.Row {
	// Format status with color
	status := styles.StatusStyle(plan.Status).Render(plan.Status)

	row := table.Row{
		fmt.Sprintf("%d", plan.ID),
		status,
	}

	if showProject {
		row = append(row, projectName)
	}

	row = append(row, truncate(plan.Title, 50))

	return row
}

// formatPlanDetail formats a full plan for display.
func formatPlanDetail(plan *backlog.Plan, styles Styles) string {
	var buf strings.Builder

	buf.WriteString(styles.DetailTitle.Render(fmt.Sprintf("Plan #%d", plan.ID)) + "\n")
	buf.WriteString("Title: " + plan.Title + "\n")
	buf.WriteString("Status: " + styles.StatusStyle(plan.Status).Render(plan.Status) + "\n")
	if plan.ItemID != nil && *plan.ItemID != "" {
		buf.WriteString("Item: " + *plan.ItemID + "\n")
	}
	buf.WriteString("\n" + styles.DetailTitle.Render("Content") + "\n")
	buf.WriteString(plan.Content + "\n")

	return buf.String()
}

// Message types.
type loadPlansMsg struct {
	plans        []backlog.Plan
	showProject  bool
	projectNames map[int64]string
}

type plansErrorMsg struct {
	err error
}

type fetchPlansDetailMsg struct {
	id int64
}
