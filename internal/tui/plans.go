package tui

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"dangernoodle.io/ouroboros/internal/backlog"
)

// planRow represents a plan in the list.
type planRow struct {
	plan *backlog.Plan
}

func (pr planRow) Title() string {
	return fmt.Sprintf("#%d", pr.plan.ID)
}

func (pr planRow) Description() string {
	parts := []string{}
	if pr.plan.Status != "" {
		parts = append(parts, "["+pr.plan.Status+"]")
	}
	parts = append(parts, "—")
	parts = append(parts, truncate(pr.plan.Title, 50))
	return strings.Join(parts, " ")
}

func (pr planRow) FilterValue() string {
	return fmt.Sprintf("%d %s", pr.plan.ID, pr.plan.Title)
}

// PlansModel represents the plans tab.
type PlansModel struct {
	db       *sql.DB
	list     list.Model
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
	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.SetShowHelp(false)
	vp := viewport.New(0, 0)

	return &PlansModel{
		db:        db,
		list:      l,
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
			items := make([]list.Item, len(m.plans))
			for i := range m.plans {
				items[i] = planRow{&m.plans[i]}
			}
			m.list.SetItems(items)
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

	case tea.Key:
		if msg.Type == tea.KeyEsc {
			if !m.focusList {
				m.focusList = true
				m.selectedPlan = nil
				m.viewport.SetContent("")
				return m, nil
			}
		}
		if msg.Type == tea.KeyEnter && m.focusList {
			if len(m.plans) > 0 && m.list.Index() < len(m.plans) {
				selectedPlan := m.plans[m.list.Index()]
				return m, func() tea.Msg {
					return fetchPlansDetailMsg{id: selectedPlan.ID}
				}
			}
		}

		if m.focusList {
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
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

	listView := m.list.View()
	if m.focusList {
		return listView
	}

	return fmt.Sprintf("%-40s | %s", listView, m.viewport.View())
}

// LoadPlans dispatches a command to load plans for the given project filter.
func (m *PlansModel) LoadPlans(projIDs []int64) tea.Cmd {
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
		return loadPlansMsg{plans: plans}
	}
}

// SetSize updates the size of list and viewport.
func (m *PlansModel) SetSize(width, height int) {
	m.list.SetSize(width/2-2, height-2)
	m.viewport.Width = width / 2
	m.viewport.Height = height - 2
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
	plans []backlog.Plan
}

type plansErrorMsg struct {
	err error
}

type fetchPlansDetailMsg struct {
	id int64
}
