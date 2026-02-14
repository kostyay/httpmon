package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kostyay/httpmon/internal/throttle"
)

type throttlePreset struct {
	name    string
	bps     int64
	latency time.Duration
}

var throttlePresets = []throttlePreset{
	{name: "None", bps: 0, latency: 0},
	{name: "3G (750 kbps, 100ms)", bps: throttle.PresetBandwidth("3g"), latency: 100 * time.Millisecond},
	{name: "4G (4 Mbps, 50ms)", bps: throttle.PresetBandwidth("4g"), latency: 50 * time.Millisecond},
	{name: "WiFi (30 Mbps, 5ms)", bps: throttle.PresetBandwidth("wifi"), latency: 5 * time.Millisecond},
}

func (a *App) initThrottle() {
	a.showThrottle = true
	a.throttleCursor = a.activeThrottlePreset()
}

func (a *App) activeThrottlePreset() int {
	if a.throttle == nil {
		return 0
	}
	bps := a.throttle.GetThrottleBPS()
	for i, p := range throttlePresets {
		if p.bps == bps {
			return i
		}
	}
	return 0
}

func (a *App) updateThrottle(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "T":
		a.showThrottle = false
		return a, nil
	case "j", "down":
		if a.throttleCursor < len(throttlePresets)-1 {
			a.throttleCursor++
		}
	case "k", "up":
		if a.throttleCursor > 0 {
			a.throttleCursor--
		}
	case "enter":
		if a.throttle != nil && a.throttleCursor < len(throttlePresets) {
			p := throttlePresets[a.throttleCursor]
			a.throttle.SetThrottle(p.bps, p.latency)
		}
		a.showThrottle = false
		return a, nil
	}
	return a, nil
}

func (a *App) viewThrottle() string {
	var b strings.Builder

	b.WriteString(styleMenuTitle.Render(" Throttle "))
	b.WriteString("\n")

	active := a.activeThrottlePreset()
	for i, p := range throttlePresets {
		marker := "  "
		if i == active {
			marker = lipgloss.NewStyle().Foreground(colorGreen).Render("● ")
		}

		line := fmt.Sprintf(" %s%-28s", marker, p.name)
		if i == a.throttleCursor {
			line = styleMenuSelected.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styleMuted.Render(" j/k navigate  Enter apply  Esc close "))

	popup := styleMenuBorder.Render(b.String())

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		popup,
	)
}

func (a *App) throttleStatusLabel() string {
	if a.throttle == nil {
		return ""
	}
	bps := a.throttle.GetThrottleBPS()
	if bps <= 0 {
		return ""
	}
	for _, p := range throttlePresets[1:] {
		if p.bps == bps {
			return p.name
		}
	}
	return fmt.Sprintf("%d B/s", bps)
}
