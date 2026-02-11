package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorGreen  = lipgloss.AdaptiveColor{Light: "#22863a", Dark: "#85e89d"}
	colorYellow = lipgloss.AdaptiveColor{Light: "#b08800", Dark: "#ffea7f"}
	colorRed    = lipgloss.AdaptiveColor{Light: "#cb2431", Dark: "#f97583"}
	colorOrange = lipgloss.AdaptiveColor{Light: "#e36209", Dark: "#ffab70"}
	colorBlue   = lipgloss.AdaptiveColor{Light: "#005cc5", Dark: "#79b8ff"}
	colorMuted  = lipgloss.AdaptiveColor{Light: "#6a737d", Dark: "#6a737d"}
	colorFg     = lipgloss.AdaptiveColor{Light: "#24292e", Dark: "#e1e4e8"}

	styleStatus2xx = lipgloss.NewStyle().Foreground(colorGreen).Bold(true)
	styleStatus3xx = lipgloss.NewStyle().Foreground(colorBlue)
	styleStatus4xx = lipgloss.NewStyle().Foreground(colorOrange).Bold(true)
	styleStatus5xx = lipgloss.NewStyle().Foreground(colorRed).Bold(true)

	styleMethod   = lipgloss.NewStyle().Foreground(colorBlue).Bold(true)
	styleSelected = lipgloss.NewStyle().Reverse(true)
	styleMuted    = lipgloss.NewStyle().Foreground(colorMuted)
	styleHeader   = lipgloss.NewStyle().Foreground(colorFg).Bold(true)

	styleStatusBar = lipgloss.NewStyle().
			Background(lipgloss.AdaptiveColor{Light: "#d1d5da", Dark: "#2f363d"}).
			Foreground(lipgloss.AdaptiveColor{Light: "#24292e", Dark: "#e1e4e8"}).
			Padding(0, 1)

	styleActiveTab = lipgloss.NewStyle().
			Foreground(colorBlue).
			Bold(true).
			Underline(true)

	styleInactiveTab = lipgloss.NewStyle().
				Foreground(colorMuted)

	styleWarning = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#cb2431", Dark: "#ff6670"}).
			Bold(true)

	styleDetailHeader = lipgloss.NewStyle().Bold(true)
	styleSection      = lipgloss.NewStyle().Foreground(colorYellow).Bold(true)
)

func statusStyle(code int) lipgloss.Style {
	switch {
	case code >= 500:
		return styleStatus5xx
	case code >= 400:
		return styleStatus4xx
	case code >= 300:
		return styleStatus3xx
	case code >= 200:
		return styleStatus2xx
	default:
		return styleMuted
	}
}
