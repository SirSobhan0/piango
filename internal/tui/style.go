package tui

import "github.com/charmbracelet/lipgloss"

// The palette keeps the original teal-on-dark look.
const (
	colAccent = "#00E6C3"
	colDim    = "#666666"
	colFaint  = "#333333"
	colText   = "#AAAAAA"
	colBright = "#FFFFFF"
	colBlue   = "#8BE9FD"
	colPurple = "#6272A4"
	colGreen  = "#50FA7B"
	colYellow = "#F1FA8C"
	colRed    = "#FF5555"
	colBack   = "#111111"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colBright)).
			Padding(0, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colAccent))

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colFaint)).
			Padding(0, 1)

	panelTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colPurple)).
			Bold(true)

	instStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colAccent)).
			Background(lipgloss.Color(colBack)).
			Bold(true).
			Padding(0, 1)

	formulaStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colYellow)).
			Italic(true)

	aboutStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colDim))

	waveStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colAccent))

	specStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colBlue))

	lissStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colGreen))

	keyStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colFaint)).
			Foreground(lipgloss.Color(colText)).
			Width(7).
			Align(lipgloss.Center)

	activeKeyStyle = keyStyle.
			BorderForeground(lipgloss.Color(colAccent)).
			Foreground(lipgloss.Color("#000000")).
			Background(lipgloss.Color(colAccent)).
			Bold(true)

	activeParamStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colBack)).
			Background(lipgloss.Color(colAccent)).
			Bold(true)

	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colPurple))
	valueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colBlue))

	okStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colBack)).
		Background(lipgloss.Color(colGreen)).
		Bold(true).
		Padding(0, 1)

	errStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colBack)).
			Background(lipgloss.Color(colRed)).
			Bold(true).
			Padding(0, 1)

	helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colDim))

	editorStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color(colAccent)).
			Padding(1, 2)
)
