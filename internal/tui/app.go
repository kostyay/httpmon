package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kostyay/httpmon/internal/filter"
	"github.com/kostyay/httpmon/internal/store"
)

type tickMsg time.Time

type App struct {
	store FlowReader
	proxy ProxyInfo

	caTrusted bool

	// view state
	showDetail  bool
	selectedID  store.FlowID
	selectedIdx int

	// flow list state
	flows       []store.FlowMeta
	totalFlows  int
	filterInput textinput.Model
	filterText  string
	storeFilter store.Filter // nil = match all

	// detail view state
	detailTab   int // 0=request, 1=response
	detailVP    viewport.Model
	detailReady bool
	detailRaw   bool // false=pretty-print, true=raw

	width, height int
	ready         bool
}

func NewApp(s FlowReader, p ProxyInfo, caTrusted bool) *App {
	ti := textinput.New()
	ti.Placeholder = "/ to filter..."
	ti.CharLimit = 256

	return &App{
		store:       s,
		proxy:       p,
		caTrusted:   caTrusted,
		filterInput: ti,
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (a *App) Init() tea.Cmd {
	return tickCmd()
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.ready = true
		if a.showDetail {
			a.detailVP.Width = a.width
			a.detailVP.Height = a.height - 3 // header + tab + status
		}
		return a, nil

	case tickMsg:
		a.refreshFlows()
		return a, tickCmd()

	case tea.KeyMsg:
		if a.showDetail {
			return a.updateDetail(msg)
		}
		return a.updateList(msg)
	}

	if a.filterInput.Focused() {
		var cmd tea.Cmd
		a.filterInput, cmd = a.filterInput.Update(msg)
		return a, cmd
	}

	return a, nil
}

func (a *App) View() string {
	if !a.ready {
		return "Loading..."
	}
	if a.showDetail {
		return a.viewDetail()
	}
	return a.viewList()
}

func (a *App) refreshFlows() {
	maxVisible := a.height - 4 // header + column header + separator + status bar
	if maxVisible < 1 {
		maxVisible = 50
	}
	a.flows, a.totalFlows = a.store.List(a.storeFilter, 0, maxVisible)
}

func (a *App) applyFilter() {
	text := a.filterInput.Value()
	a.filterText = text
	qf := filter.CompileQuick(text)
	if qf == nil {
		a.storeFilter = nil
	} else {
		a.storeFilter = qf
	}
	a.selectedIdx = 0
}

func (a *App) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.filterInput.Focused() {
		switch msg.String() {
		case "esc":
			a.filterInput.Blur()
			return a, nil
		case "enter":
			a.applyFilter()
			a.filterInput.Blur()
			return a, nil
		}
		var cmd tea.Cmd
		a.filterInput, cmd = a.filterInput.Update(msg)
		a.applyFilter()
		return a, cmd
	}

	switch msg.String() {
	case "q":
		return a, tea.Quit
	case "ctrl+c":
		return a, tea.Quit
	case "/":
		a.filterInput.Focus()
		return a, a.filterInput.Cursor.BlinkCmd()
	case "j", "down":
		if a.selectedIdx < len(a.flows)-1 {
			a.selectedIdx++
		}
	case "k", "up":
		if a.selectedIdx > 0 {
			a.selectedIdx--
		}
	case "g", "home":
		a.selectedIdx = 0
	case "G", "end":
		if len(a.flows) > 0 {
			a.selectedIdx = len(a.flows) - 1
		}
	case "enter":
		if len(a.flows) > 0 && a.selectedIdx < len(a.flows) {
			a.selectedID = a.flows[a.selectedIdx].ID
			a.showDetail = true
			a.detailTab = 0
			a.initDetailViewport()
		}
	}
	return a, nil
}

func (a *App) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.showDetail = false
		return a, nil
	case "ctrl+c":
		return a, tea.Quit
	case "q":
		a.showDetail = false
		return a, nil
	case "1", "left":
		a.detailTab = 0
		a.updateDetailContent()
	case "2", "right":
		a.detailTab = 1
		a.updateDetailContent()
	case "j", "down":
		a.detailVP.ScrollDown(1)
	case "k", "up":
		a.detailVP.ScrollUp(1)
	case "d":
		a.detailVP.HalfPageDown()
	case "u":
		a.detailVP.HalfPageUp()
	case "p":
		a.detailRaw = !a.detailRaw
		a.updateDetailContent()
	case "n":
		a.nextFlow(1)
	case "N":
		a.nextFlow(-1)
	}
	return a, nil
}

func (a *App) nextFlow(delta int) {
	if len(a.flows) == 0 {
		return
	}
	// Find current flow in list
	for i, f := range a.flows {
		if f.ID == a.selectedID {
			next := i + delta
			if next >= 0 && next < len(a.flows) {
				a.selectedIdx = next
				a.selectedID = a.flows[next].ID
				a.updateDetailContent()
			}
			return
		}
	}
}

func (a *App) initDetailViewport() {
	a.detailVP = viewport.New(a.width, a.height-3)
	a.detailVP.YPosition = 0
	a.detailReady = true
	a.updateDetailContent()
}

func (a *App) updateDetailContent() {
	meta, data, err := a.store.Get(a.selectedID)
	if err != nil {
		a.detailVP.SetContent("Flow no longer available. Press Esc to return.")
		return
	}
	darkBg := lipgloss.HasDarkBackground()
	a.detailVP.SetContent(renderDetailBody(meta, data, a.detailTab, a.width, darkBg, !a.detailRaw))
}

func (a *App) proxyAddr() string {
	if a.proxy != nil {
		return a.proxy.Addr()
	}
	return ":8080"
}

func (a *App) statusText() string {
	addr := a.proxyAddr()
	var warning string
	if !a.caTrusted {
		warning = styleWarning.Render("CA NOT TRUSTED") + "  "
	}
	info := fmt.Sprintf("%d flows | Proxy %s", a.totalFlows, addr)
	help := "? help  / filter"
	gap := a.width - len(warning) - len(info) - len(help)
	if gap < 2 {
		gap = 2
	}
	return styleStatusBar.Width(a.width).Render(
		warning + info + fmt.Sprintf("%*s", gap, help),
	)
}
