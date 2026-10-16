package commands

import "github.com/charmbracelet/lipgloss"

var (
	green  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	red    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	cyan   = lipgloss.NewStyle().Foreground(lipgloss.Color("51"))
	bold   = lipgloss.NewStyle().Bold(true)
	dim    = lipgloss.NewStyle().Faint(true)

	checkMark = green.Render("✓")
	crossMark = red.Render("✗")
	bullet    = dim.Render("·")
)