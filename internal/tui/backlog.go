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

// itemRow represents a backlog item in the list.
type itemRow struct {
	item *backlog.Item
}

func (ir itemRow) Title() string {
	return ir.item.ID
}

func (ir itemRow) Description() string {
	parts := []string{}
	if ir.item.Priority != "" {
		parts = append(parts, ir.item.Priority)
	}
	if ir.item.Status != "" {
		parts = append(parts, "["+ir.item.Status+"]")
	}
	parts = append(parts, truncate(ir.item.Title, 50))
	if ir.item.Component != "" {
		parts = append(parts, "("+ir.item.Component+")")
	}
	return strings.Join(parts, " ")
}

func (ir itemRow) FilterValue() string {
	return ir.item.ID + " " + ir.item.Title
}

// BacklogModel represents the backlog tab.
type BacklogModel struct {
	db       *sql.DB
	list     list.Model
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
	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.SetShowHelp(false)
	vp := viewport.New(0, 0)

	return &BacklogModel{
		db:        db,
		list:      l,
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
			items := make([]list.Item, len(m.items))
			for i := range m.items {
				items[i] = itemRow{&m.items[i]}
			}
			m.list.SetItems(items)
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

	case tea.Key:
		if msg.Type == tea.KeyEsc {
			if !m.focusList {
				m.focusList = true
				m.selectedItem = nil
				m.viewport.SetContent("")
				return m, nil
			}
		}
		if msg.Type == tea.KeyEnter && m.focusList {
			if len(m.items) > 0 && m.list.Index() < len(m.items) {
				selectedItem := m.items[m.list.Index()]
				return m, func() tea.Msg {
					return fetchBacklogDetailMsg{id: selectedItem.ID}
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

	listView := m.list.View()
	if m.focusList {
		return listView
	}

	// Split layout: list on left, detail on right
	return fmt.Sprintf("%-40s | %s", listView, m.viewport.View())
}

// LoadItems dispatches a command to load backlog items for the given project filter.
func (m *BacklogModel) LoadItems(projIDs []int64) tea.Cmd {
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
		return loadBacklogMsg{items: items}
	}
}

// SetSize updates the size of list and viewport.
func (m *BacklogModel) SetSize(width, height int) {
	m.list.SetSize(width/2-2, height-2)
	m.viewport.Width = width / 2
	m.viewport.Height = height - 2
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
	items []backlog.Item
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
