package tui

import (
	"database/sql"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"dangernoodle.io/ouroboros/internal/store"
)

// kbRow represents a KB document in the list.
type kbRow struct {
	doc *store.DocumentSummary
}

func (kr kbRow) Title() string {
	return "[" + kr.doc.Type + "]"
}

func (kr kbRow) Description() string {
	parts := []string{}
	if kr.doc.Project != "" {
		parts = append(parts, kr.doc.Project)
	}
	if kr.doc.Category != "" {
		parts = append(parts, "/"+kr.doc.Category)
	}
	if len(parts) > 0 {
		parts = append(parts, "—")
	}
	parts = append(parts, truncate(kr.doc.Title, 50))
	return strings.Join(parts, " ")
}

func (kr kbRow) FilterValue() string {
	return kr.doc.Title + " " + kr.doc.Type
}

// KBModel represents the KB tab.
type KBModel struct {
	db       *sql.DB
	list     list.Model
	viewport viewport.Model
	styles   Styles

	docs         []store.DocumentSummary
	selectedDoc  *store.Document
	focusList    bool
	searchMode   bool
	searchInput  textinput.Model
	loading      bool
	error        string
	projectNames []string
}

// NewKBModel creates a new KB tab model.
func NewKBModel(db *sql.DB, styles Styles) *KBModel {
	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.SetShowHelp(false)
	vp := viewport.New(0, 0)

	si := textinput.New()
	si.Placeholder = "Search..."
	si.Focus()

	return &KBModel{
		db:          db,
		list:        l,
		viewport:    vp,
		styles:      styles,
		focusList:   true,
		searchMode:  false,
		searchInput: si,
		loading:     true,
	}
}

// Init implements tea.Model.
func (m *KBModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m *KBModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loadKBMsg:
		m.docs = msg.docs
		m.loading = false
		m.error = ""
		if len(m.docs) > 0 {
			items := make([]list.Item, len(m.docs))
			for i := range m.docs {
				items[i] = kbRow{&m.docs[i]}
			}
			m.list.SetItems(items)
		}
		return m, nil

	case kbErrorMsg:
		m.loading = false
		m.error = msg.err.Error()
		return m, nil

	case fetchKBDetailMsg:
		doc, err := store.GetDocument(m.db, msg.id)
		if err != nil {
			return m, func() tea.Msg {
				return kbErrorMsg{err: err}
			}
		}
		m.selectedDoc = doc
		m.viewport.SetContent(formatKBDetail(doc, m.styles))
		m.focusList = false
		return m, nil

	case tea.KeyMsg:
		if msg.Type == tea.KeyEsc {
			if m.searchMode {
				m.searchMode = false
				m.searchInput.SetValue("")
				m.focusList = true
				return m, nil
			}
			if !m.focusList {
				m.focusList = true
				m.selectedDoc = nil
				m.viewport.SetContent("")
				return m, nil
			}
		}

		if msg.String() == "/" && m.focusList && !m.searchMode {
			m.searchMode = true
			m.focusList = false
			m.searchInput.SetValue("")
			m.searchInput.Focus()
			return m, nil
		}

		if m.searchMode {
			switch msg.Type {
			case tea.KeyEnter:
				query := m.searchInput.Value()
				if query == "" {
					// Empty query: browse mode
					return m, m.LoadDocuments(m.projectNames)
				}
				return m, m.SearchDocuments(query)
			default:
				var cmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				return m, cmd
			}
		}

		if msg.Type == tea.KeyEnter && m.focusList {
			if len(m.docs) > 0 && m.list.Index() < len(m.docs) {
				selectedDoc := m.docs[m.list.Index()]
				return m, func() tea.Msg {
					return fetchKBDetailMsg{id: selectedDoc.ID}
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
func (m *KBModel) View() string {
	if m.searchMode {
		return "Search: " + m.searchInput.View() + "\n"
	}

	if m.loading {
		return "Loading KB documents...\n"
	}

	if m.error != "" {
		return m.styles.FooterError.Render("Error: "+m.error) + "\n"
	}

	if len(m.docs) == 0 {
		return "No KB documents found.\n"
	}

	listView := m.list.View()
	if m.focusList {
		return listView
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, listView, "  ", m.viewport.View())
}

// LoadDocuments dispatches a command to load KB documents.
func (m *KBModel) LoadDocuments(projectNames []string) tea.Cmd {
	m.projectNames = projectNames
	m.loading = true
	m.docs = nil
	m.selectedDoc = nil
	m.focusList = true
	m.searchMode = false
	m.viewport.SetContent("")

	return func() tea.Msg {
		docs, err := store.QueryDocuments(m.db, "", projectNames, "", "", nil, 200)
		if err != nil {
			return kbErrorMsg{err: err}
		}
		return loadKBMsg{docs: docs}
	}
}

// SearchDocuments dispatches a command to search KB documents.
func (m *KBModel) SearchDocuments(query string) tea.Cmd {
	m.loading = true
	m.docs = nil
	m.selectedDoc = nil
	m.focusList = true
	m.searchMode = false
	m.viewport.SetContent("")

	return func() tea.Msg {
		docs, err := store.SearchDocuments(m.db, query, "", m.projectNames, 200)
		if err != nil {
			return kbErrorMsg{err: err}
		}
		return loadKBMsg{docs: docs}
	}
}

// SetSize updates the size of list and viewport.
func (m *KBModel) SetSize(width, height int) {
	m.list.SetSize(width/2-2, height-2)
	m.viewport.Width = width / 2
	m.viewport.Height = height - 2
}

// formatKBDetail formats a full KB document for display.
func formatKBDetail(doc *store.Document, styles Styles) string {
	var buf strings.Builder

	buf.WriteString(styles.DetailTitle.Render(doc.Title) + "\n")
	buf.WriteString("Type: " + doc.Type + "\n")
	if doc.Project != "" {
		buf.WriteString("Project: " + doc.Project + "\n")
	}
	if doc.Category != "" {
		buf.WriteString("Category: " + doc.Category + "\n")
	}
	if len(doc.Tags) > 0 {
		buf.WriteString("Tags: " + strings.Join(doc.Tags, ", ") + "\n")
	}
	buf.WriteString("\n" + styles.DetailTitle.Render("Content") + "\n")
	buf.WriteString(doc.Content + "\n")
	if doc.Notes != "" {
		buf.WriteString("\n" + styles.DetailTitle.Render("Notes") + "\n")
		buf.WriteString(doc.Notes + "\n")
	}

	return buf.String()
}

// Message types.
type loadKBMsg struct {
	docs []store.DocumentSummary
}

type kbErrorMsg struct {
	err error
}

type fetchKBDetailMsg struct {
	id int64
}
