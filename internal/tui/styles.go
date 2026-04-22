package tui

import "github.com/charmbracelet/lipgloss"

// Styles holds all terminal styling.
type Styles struct {
	// Tab styling
	ActiveTab   lipgloss.Style
	InactiveTab lipgloss.Style
	TabBar      lipgloss.Style

	// List styling
	ListTitle    lipgloss.Style
	ListSelected lipgloss.Style
	ListNormal   lipgloss.Style

	// Priority colors
	PriorityP0 lipgloss.Style
	PriorityP1 lipgloss.Style
	PriorityP2 lipgloss.Style
	PriorityP3 lipgloss.Style
	PriorityP4 lipgloss.Style
	PriorityP5 lipgloss.Style
	PriorityP6 lipgloss.Style

	// Status badges
	StatusOpen     lipgloss.Style
	StatusActive   lipgloss.Style
	StatusDone     lipgloss.Style
	StatusCanceled lipgloss.Style
	StatusDraft    lipgloss.Style

	// Viewport
	DetailBorder lipgloss.Style
	DetailTitle  lipgloss.Style

	// Search
	SearchInput lipgloss.Style

	// Footer
	FooterError lipgloss.Style
	FooterInfo  lipgloss.Style

	// Help
	HelpKey   lipgloss.Style
	HelpValue lipgloss.Style
}

// DefaultStyles returns initialized styles with sensible defaults.
func DefaultStyles() Styles {
	return Styles{
		ActiveTab: lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Bold(true).
			Underline(true).
			Padding(0, 2),
		InactiveTab: lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Padding(0, 2),
		TabBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			PaddingBottom(1),

		ListTitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("32")).
			Bold(true),
		ListSelected: lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Background(lipgloss.Color("237")).
			Bold(true).
			PaddingLeft(1),
		ListNormal: lipgloss.NewStyle().
			PaddingLeft(1),

		PriorityP0: lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
		PriorityP1: lipgloss.NewStyle().Foreground(lipgloss.Color("208")),
		PriorityP2: lipgloss.NewStyle().Foreground(lipgloss.Color("226")),
		PriorityP3: lipgloss.NewStyle().Foreground(lipgloss.Color("243")),
		PriorityP4: lipgloss.NewStyle().Foreground(lipgloss.Color("243")),
		PriorityP5: lipgloss.NewStyle().Foreground(lipgloss.Color("243")),
		PriorityP6: lipgloss.NewStyle().Foreground(lipgloss.Color("243")),

		StatusOpen:     lipgloss.NewStyle().Foreground(lipgloss.Color("32")),
		StatusActive:   lipgloss.NewStyle().Foreground(lipgloss.Color("33")),
		StatusDone:     lipgloss.NewStyle().Foreground(lipgloss.Color("242")),
		StatusCanceled: lipgloss.NewStyle().Foreground(lipgloss.Color("243")),
		StatusDraft:    lipgloss.NewStyle().Foreground(lipgloss.Color("243")),

		DetailBorder: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(1),
		DetailTitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("32")).
			Bold(true),

		SearchInput: lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			PaddingLeft(1),

		FooterError: lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true),
		FooterInfo: lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")),

		HelpKey: lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Bold(true),
		HelpValue: lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")),
	}
}

// PriorityStyle returns the style for a given priority.
func (s Styles) PriorityStyle(priority string) lipgloss.Style {
	switch priority {
	case "P0":
		return s.PriorityP0
	case "P1":
		return s.PriorityP1
	case "P2":
		return s.PriorityP2
	case "P3":
		return s.PriorityP3
	case "P4":
		return s.PriorityP4
	case "P5":
		return s.PriorityP5
	case "P6":
		return s.PriorityP6
	default:
		return lipgloss.NewStyle()
	}
}

// StatusStyle returns the style for a given status.
func (s Styles) StatusStyle(status string) lipgloss.Style {
	switch status {
	case "open":
		return s.StatusOpen
	case "active":
		return s.StatusActive
	case "done":
		return s.StatusDone
	case "canceled":
		return s.StatusCanceled
	case "draft":
		return s.StatusDraft
	default:
		return lipgloss.NewStyle()
	}
}
