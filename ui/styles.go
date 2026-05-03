package ui

import "github.com/charmbracelet/lipgloss"

var (
	fitnessColor = lipgloss.Color("#00FFFF")
	fatigueColor = lipgloss.Color("#FF00FF")
	formColor    = lipgloss.Color("#FFFF00")

	appStyle = lipgloss.NewStyle().
			Padding(1, 2)

	navItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	navItemActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("62")).
				Bold(true)

	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("87"))

	subtleBoxStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238"))

	cardStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))

	tabStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(lipgloss.Color("249"))

	activeTabStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Bold(true).
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("62"))

	headerBarStyle = lipgloss.NewStyle().
			BorderBottom(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("238")).
			Padding(0, 0).
			MarginBottom(0)

	footerStyle = lipgloss.NewStyle().
			BorderTop(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("238")).
			Padding(0, 1)

	goodStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	warnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	badStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

func statStyle(c lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().
		Width(20).
		Padding(1, 2).
		MarginRight(1).
		Bold(true).
		Foreground(c).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(c)
}
