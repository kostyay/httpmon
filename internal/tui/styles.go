package tui

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

var (
	colorGreen  = compat.AdaptiveColor{Light: lipgloss.Color("#22863a"), Dark: lipgloss.Color("#85e89d")}
	colorYellow = compat.AdaptiveColor{Light: lipgloss.Color("#b08800"), Dark: lipgloss.Color("#ffea7f")}
	colorRed    = compat.AdaptiveColor{Light: lipgloss.Color("#cb2431"), Dark: lipgloss.Color("#f97583")}
	colorOrange = compat.AdaptiveColor{Light: lipgloss.Color("#e36209"), Dark: lipgloss.Color("#ffab70")}
	colorBlue   = compat.AdaptiveColor{Light: lipgloss.Color("#005cc5"), Dark: lipgloss.Color("#79b8ff")}
	colorMuted  = compat.AdaptiveColor{Light: lipgloss.Color("#6a737d"), Dark: lipgloss.Color("#6a737d")}
	colorFg     = compat.AdaptiveColor{Light: lipgloss.Color("#24292e"), Dark: lipgloss.Color("#e1e4e8")}

	styleStatus2xx = lipgloss.NewStyle().Foreground(colorGreen).Bold(true)
	styleStatus3xx = lipgloss.NewStyle().Foreground(colorBlue)
	styleStatus4xx = lipgloss.NewStyle().Foreground(colorOrange).Bold(true)
	styleStatus5xx = lipgloss.NewStyle().Foreground(colorRed).Bold(true)

	styleMethod   = lipgloss.NewStyle().Foreground(colorBlue).Bold(true)
	styleSelected = lipgloss.NewStyle().Reverse(true)
	styleMuted    = lipgloss.NewStyle().Foreground(colorMuted)
	styleHeader   = lipgloss.NewStyle().Foreground(colorFg).Bold(true)

	styleStatusBar = lipgloss.NewStyle().
			Background(compat.AdaptiveColor{Light: lipgloss.Color("#d1d5da"), Dark: lipgloss.Color("#2f363d")}).
			Foreground(compat.AdaptiveColor{Light: lipgloss.Color("#24292e"), Dark: lipgloss.Color("#e1e4e8")}).
			Padding(0, 1)

	styleActiveTab = lipgloss.NewStyle().
			Foreground(colorBlue).
			Bold(true).
			Underline(true)

	styleInactiveTab = lipgloss.NewStyle().
				Foreground(colorMuted)

	styleWarning = lipgloss.NewStyle().
			Foreground(compat.AdaptiveColor{Light: lipgloss.Color("#cb2431"), Dark: lipgloss.Color("#ff6670")}).
			Bold(true)

	styleDetailHeader = lipgloss.NewStyle().Bold(true)
	styleSection      = lipgloss.NewStyle().Foreground(colorYellow).Bold(true)

	styleMenuBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBlue).
			Padding(0, 1)

	styleMenuTitle = lipgloss.NewStyle().
			Foreground(colorBlue).
			Bold(true)

	styleMenuSelected = lipgloss.NewStyle().
				Reverse(true)
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
