package tui

import (
	"database/sql"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"dangernoodle.io/ouroboros/internal/backlog"
)

// BacklogModel represents the backlog tab.
type BacklogModel struct {
	db       *sql.DB
	table    table.Model
	viewport viewport.Model
	styles   Styles

	items        []backlog.Item
	selectedItem *backlog.Item
	focusList    bool
	loading      bool
	error        string
}

// NewBacklogModel creates a new backlog tab model.
func NewBacklogModel(db *sql.DB, styles Styles) *BacklogModel {
	tbl := table.New(table.WithFocused(true), table.WithHeight(0))
	vp := viewport.New(0, 0)

	return &BacklogModel{
		db:        db,
		table:     tbl,
		viewport:  vp,
		styles:    styles,
		focusList: true,
		loading:   true,
	}
}

// Init implements tea.Model.
func (m *BacklogModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m *BacklogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loadBacklogMsg:
		m.items = msg.items
		m.loading = false
		m.error = ""
		if len(m.items) > 0 {
			if msg.showProject {
				sort.SliceStable(m.items, func(i, j int) bool {
					if m.items[i].ProjectID != m.items[j].ProjectID {
						return m.items[i].ProjectID < m.items[j].ProjectID
					}
					return m.items[i].Priority < m.items[j].Priority
				})
			}
			cols := backlogColumns(msg.showProject, 30) // placeholder width
			rows := []table.Row{}
			for i := range m.items {
				projectName := ""
				if msg.showProject {
					projectName = msg.projectNames[m.items[i].ProjectID]
				}
				row := itemToRow(m.items[i], msg.showProject, projectName, m.styles)
				rows = append(rows, row)
			}
			m.table.SetColumns(cols)
			m.table.SetRows(rows)
		}
		return m, nil

	case backlogErrorMsg:
		m.loading = false
		m.error = msg.err.Error()
		return m, nil

	case fetchBacklogDetailMsg:
		item, err := backlog.GetItem(m.db, msg.id)
		if err != nil {
			return m, func() tea.Msg {
				return backlogErrorMsg{err: err}
			}
		}
		m.selectedItem = item
		m.viewport.SetContent(formatBacklogDetail(item, m.styles))
		m.focusList = false
		return m, nil

	case tea.KeyMsg:
		if msg.Type == tea.KeyEsc {
			if !m.focusList {
				m.focusList = true
				m.selectedItem = nil
				m.viewport.SetContent("")
				return m, nil
			}
		}
		if msg.Type == tea.KeyEnter && m.focusList {
			if len(m.items) > 0 && m.table.Cursor() < len(m.items) {
				selectedItem := m.items[m.table.Cursor()]
				return m, func() tea.Msg {
					return fetchBacklogDetailMsg{id: selectedItem.ID}
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
func (m *BacklogModel) View() string {
	if m.loading {
		return "Loading backlog...\n"
	}

	if m.error != "" {
		return m.styles.FooterError.Render("Error: "+m.error) + "\n"
	}

	if len(m.items) == 0 {
		return "No backlog items found.\n"
	}

	tableView := m.table.View()
	if m.focusList {
		return tableView
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, tableView, "  ", m.viewport.View())
}

// LoadItems dispatches a command to load backlog items for the given project filter.
// When showProject is true, rows display their project name and results are
// clustered by project; projectNames maps ProjectID to display name.
func (m *BacklogModel) LoadItems(projIDs []int64, showProject bool, projectNames map[int64]string) tea.Cmd {
	m.loading = true
	m.items = nil
	m.selectedItem = nil
	m.focusList = true
	m.viewport.SetContent("")

	return func() tea.Msg {
		items, err := backlog.ListItems(m.db, backlog.ItemFilter{ProjectIDs: projIDs})
		if err != nil {
			return backlogErrorMsg{err: err}
		}
		return loadBacklogMsg{items: items, showProject: showProject, projectNames: projectNames}
	}
}

// SetSize updates the size of table and viewport.
func (m *BacklogModel) SetSize(width, height int) {
	tableWidth := width/2 - 2
	tableHeight := height - 2

	// Compute title width based on showProject
	showProject := len(m.table.Columns()) > 5 // 6 cols when All, 5 when single project
	titleWidth := computeTitleWidth(tableWidth, showProject)

	// Rebuild columns with correct width
	cols := backlogColumns(showProject, titleWidth)
	m.table.SetColumns(cols)
	m.table.SetWidth(tableWidth)
	m.table.SetHeight(tableHeight)

	m.viewport.Width = width / 2
	m.viewport.Height = tableHeight
}

// computeTitleWidth calculates the title column width based on total table width and other columns.
func computeTitleWidth(totalWidth int, showProject bool) int {
	idWidth := 8
	pWidth := 4
	statusWidth := 10
	componentWidth := 12
	padding := 2 * 5 // 2 per column (5 columns base, 6 with project)

	used := idWidth + pWidth + statusWidth + componentWidth + padding
	if showProject {
		projectWidth := 14
		used = idWidth + pWidth + statusWidth + projectWidth + componentWidth + padding
	}

	titleWidth := totalWidth - used
	if titleWidth < 20 {
		titleWidth = 20
	}
	return titleWidth
}

// backlogColumns builds column definitions for the backlog table.
func backlogColumns(showProject bool, titleWidth int) []table.Column {
	cols := []table.Column{
		{Title: "ID", Width: 8},
		{Title: "P", Width: 4},
		{Title: "Status", Width: 10},
	}
	if showProject {
		cols = append(cols, table.Column{Title: "Project", Width: 14})
	}
	cols = append(cols, []table.Column{
		{Title: "Title", Width: titleWidth},
		{Title: "Component", Width: 12},
	}...)
	return cols
}

// itemToRow converts a backlog item to a table row.
func itemToRow(item backlog.Item, showProject bool, projectName string, styles Styles) table.Row {
	// Format priority with color
	priority := styles.PriorityStyle(item.Priority).Render(item.Priority)

	// Format status with color
	status := styles.StatusStyle(item.Status).Render(item.Status)

	row := table.Row{
		item.ID,
		priority,
		status,
	}

	if showProject {
		row = append(row, projectName)
	}

	row = append(row, truncate(item.Title, 50))
	row = append(row, item.Component)

	return row
}

// formatBacklogDetail formats a full backlog item for display.
func formatBacklogDetail(item *backlog.Item, styles Styles) string {
	var buf strings.Builder

	buf.WriteString(styles.DetailTitle.Render(item.ID) + "\n")
	buf.WriteString("Priority: " + styles.PriorityStyle(item.Priority).Render(item.Priority) + "\n")
	buf.WriteString("Status: " + styles.StatusStyle(item.Status).Render(item.Status) + "\n")
	if item.Component != "" {
		buf.WriteString("Component: " + item.Component + "\n")
	}
	buf.WriteString("\n" + styles.DetailTitle.Render("Title") + "\n")
	buf.WriteString(item.Title + "\n\n")
	buf.WriteString(styles.DetailTitle.Render("Description") + "\n")
	buf.WriteString(item.Description + "\n")
	if item.Notes != "" {
		buf.WriteString("\n" + styles.DetailTitle.Render("Notes") + "\n")
		buf.WriteString(item.Notes + "\n")
	}

	return buf.String()
}

// Message types.
type loadBacklogMsg struct {
	items        []backlog.Item
	showProject  bool
	projectNames map[int64]string
}

type backlogErrorMsg struct {
	err error
}

type fetchBacklogDetailMsg struct {
	id string
}

// truncate truncates a string to a maximum length.
func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "…"
	}
	return s
}
