package tui

import (
	"database/sql"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Root model manages the entire TUI state.
type Root struct {
	db       *sql.DB
	styles   Styles
	width    int
	height   int
	tabs     int // 0 = Backlog, 1 = KB, 2 = Plans
	projects *ProjectFilter

	backlog *BacklogModel
	kb      *KBModel
	plans   *PlansModel

	helpMode bool
	lastErr  string
	errTimer int
}

// Run creates and runs the TUI.
func Run(db *sql.DB) error {
	styles := DefaultStyles()
	pf, err := NewProjectFilter(db)
	if err != nil {
		return fmt.Errorf("load projects: %w", err)
	}

	root := &Root{
		db:       db,
		styles:   styles,
		tabs:     0,
		projects: pf,
		backlog:  NewBacklogModel(db, styles),
		kb:       NewKBModel(db, styles),
		plans:    NewPlansModel(db, styles),
	}

	p := tea.NewProgram(root, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}

// Init implements tea.Model.
func (m *Root) Init() tea.Cmd {
	return tea.Batch(
		m.backlog.Init(),
		m.kb.Init(),
		m.plans.Init(),
		m.loadTabData(m.tabs),
	)
}

// Update implements tea.Model.
func (m *Root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.backlog.SetSize(m.width, m.height-3)
		m.kb.SetSize(m.width, m.height-3)
		m.plans.SetSize(m.width, m.height-3)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.helpMode = !m.helpMode
			return m, nil
		case "tab":
			m.tabs = (m.tabs + 1) % 3
			return m, m.loadTabData(m.tabs)
		case "shift+tab":
			m.tabs = (m.tabs - 1 + 3) % 3
			return m, m.loadTabData(m.tabs)
		case "1":
			m.tabs = 0
			return m, m.loadTabData(0)
		case "2":
			m.tabs = 1
			return m, m.loadTabData(1)
		case "3":
			m.tabs = 2
			return m, m.loadTabData(2)
		case "p":
			m.projects.Cycle()
			return m, m.loadTabData(m.tabs)
		}
	}

	// Route to active tab
	var cmd tea.Cmd
	switch m.tabs {
	case 0:
		_, cmd = m.backlog.Update(msg)
	case 1:
		_, cmd = m.kb.Update(msg)
	case 2:
		_, cmd = m.plans.Update(msg)
	}

	return m, cmd
}

// View implements tea.Model.
func (m *Root) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	// Header
	header := m.renderTabBar()

	// Body
	var body string
	switch m.tabs {
	case 0:
		body = m.backlog.View()
	case 1:
		body = m.kb.View()
	case 2:
		body = m.plans.View()
	}

	// Footer
	footer := m.renderFooter()

	// Help overlay
	if m.helpMode {
		help := m.renderHelp()
		return header + "\n" + help + "\n" + footer
	}

	return header + "\n" + body + "\n" + footer
}

// renderTabBar renders the tab bar with project filter.
func (m *Root) renderTabBar() string {
	tabNames := []string{"Backlog", "KB", "Plans"}
	tabs := []string{}
	for i, name := range tabNames {
		if i == m.tabs {
			tabs = append(tabs, m.styles.ActiveTab.Render(name))
		} else {
			tabs = append(tabs, m.styles.InactiveTab.Render(name))
		}
	}
	tabBar := strings.Join(tabs, " ")
	projectLabel := " | Project: " + m.projects.Label()
	return tabBar + projectLabel
}

// renderFooter renders the footer with status and help hint.
func (m *Root) renderFooter() string {
	hints := "q: quit | tab: switch | p: project | /: search (KB) | ?: help"
	if m.lastErr != "" && m.errTimer > 0 {
		return m.styles.FooterError.Render(m.lastErr)
	}
	return m.styles.FooterInfo.Render(hints)
}

// renderHelp renders the help overlay.
func (m *Root) renderHelp() string {
	help := `Keybindings:
  tab / shift+tab, 1/2/3   Switch tabs
  ↑/↓, j/k                 Move in list
  enter                    Load detail
  /                        Search (KB tab)
  esc                      Clear search / return to list
  p                        Cycle project filter
  ?                        Toggle help
  q, ctrl+c                Quit
`
	return help
}

// loadTabData dispatches a command to load data for the active tab.
func (m *Root) loadTabData(tab int) tea.Cmd {
	projIDs := m.projects.IDSlice()
	projNames := m.projects.NameSlice()

	switch tab {
	case 0:
		return m.backlog.LoadItems(projIDs)
	case 1:
		return m.kb.LoadDocuments(projNames)
	case 2:
		return m.plans.LoadPlans(projIDs)
	}
	return nil
}
