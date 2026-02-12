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

// listMode constants
const (
	modeFlat  = 0
	modeTree  = 1
	modeFocus = 2
)

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
	listOffset  int // first visible row in list viewport
	filterInput textinput.Model
	filterText  string
	storeFilter store.Filter // nil = match all

	// tree view state
	listMode     int             // modeFlat, modeTree, modeFocus
	focusHost    string          // active host in focus mode
	hostExpanded map[string]bool // expand state per host
	treeRows     []treeRow       // flattened visible rows for cursor addressing

	// detail view state
	detailTab          int // 0=request, 1=response
	detailVP           viewport.Model
	detailReady        bool
	detailRaw          bool // false=pretty-print, true=raw
	detailImagePreview bool // true=show image as terminal art

	width, height int
	ready         bool
}

func NewApp(s FlowReader, p ProxyInfo, caTrusted bool) *App {
	ti := textinput.New()
	ti.Placeholder = "/ to filter..."
	ti.CharLimit = 256

	return &App{
		store:        s,
		proxy:        p,
		caTrusted:    caTrusted,
		filterInput:  ti,
		hostExpanded: make(map[string]bool),
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

	case editorFinishedMsg:
		return a, nil

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
	// In tree/focus mode we need all flows for grouping, not just visible count.
	limit := 0
	if a.listMode == modeFlat {
		limit = max(a.height-4, 50) // header + column header + separator + status bar
	}
	a.flows, a.totalFlows = a.store.List(a.storeFilter, 0, limit)

	if a.listMode != modeFlat {
		groups := buildHostGroups(a.flows)
		switch a.listMode {
		case modeTree:
			a.treeRows = flattenTree(groups, a.hostExpanded)
		case modeFocus:
			a.treeRows = flattenFocus(groups, a.focusHost)
			if len(a.treeRows) == 0 {
				// focused host disappeared (evicted or filtered out) — back to tree
				a.listMode = modeTree
				a.treeRows = flattenTree(groups, a.hostExpanded)
			}
		}
	}
}

func (a *App) applyFilter() {
	a.filterText = a.filterInput.Value()
	// Explicit nil assignment avoids non-nil interface wrapping a nil pointer.
	if qf := filter.CompileQuick(a.filterText); qf != nil {
		a.storeFilter = qf
	} else {
		a.storeFilter = nil
	}
	a.selectedIdx = 0
	a.listOffset = 0
}

func (a *App) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Filter input takes priority regardless of mode.
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

	// Global keys (all modes)
	switch msg.String() {
	case "q":
		return a, tea.Quit
	case "ctrl+c":
		return a, tea.Quit
	case "/":
		a.filterInput.Focus()
		return a, a.filterInput.Cursor.BlinkCmd()
	}

	// Common cursor navigation (all modes).
	if a.handleListNav(msg) {
		return a, nil
	}

	// Mode-specific keys.
	switch a.listMode {
	case modeTree:
		return a.updateTreeList(msg)
	case modeFocus:
		return a.updateFocusList(msg)
	default:
		return a.updateFlatList(msg)
	}
}

// visibleLen returns the number of rows in the current list mode.
func (a *App) visibleLen() int {
	if a.listMode == modeFlat {
		return len(a.flows)
	}
	return len(a.treeRows)
}

// pageSize returns the number of visible rows in the list viewport.
func (a *App) pageSize() int {
	return max(a.height-5, 1)
}

// handleListNav handles cursor movement keys shared across all list modes.
// Returns true if the key was consumed.
func (a *App) handleListNav(msg tea.KeyMsg) bool {
	n := a.visibleLen()
	switch msg.String() {
	case "j", "down":
		if a.selectedIdx < n-1 {
			a.selectedIdx++
		}
	case "k", "up":
		if a.selectedIdx > 0 {
			a.selectedIdx--
		}
	case "pgdown", "ctrl+d":
		a.selectedIdx = min(a.selectedIdx+a.pageSize(), n-1)
	case "pgup", "ctrl+u":
		a.selectedIdx = max(a.selectedIdx-a.pageSize(), 0)
	case "g", "home":
		a.selectedIdx = 0
		a.listOffset = 0
	case "G", "end":
		if n > 0 {
			a.selectedIdx = n - 1
		}
	default:
		return false
	}
	return true
}

// openFlowDetail enters detail view for the given flow ID.
func (a *App) openFlowDetail(id store.FlowID) {
	a.selectedID = id
	a.showDetail = true
	a.detailTab = 0
	a.initDetailViewport()
}

func (a *App) updateFlatList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "t":
		a.listMode = modeTree
		a.selectedIdx = 0
		a.listOffset = 0
		a.refreshFlows()
	case "enter":
		if len(a.flows) > 0 && a.selectedIdx < len(a.flows) {
			a.openFlowDetail(a.flows[a.selectedIdx].ID)
		}
	}
	return a, nil
}

func (a *App) updateTreeList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "t":
		a.listMode = modeFlat
		a.selectedIdx = 0
		a.listOffset = 0
	case "right", "l":
		if a.selectedIdx < len(a.treeRows) {
			row := a.treeRows[a.selectedIdx]
			if row.IsHost {
				a.hostExpanded[row.Host] = true
				a.refreshFlows()
			}
		}
	case "left", "h":
		if a.selectedIdx < len(a.treeRows) {
			row := a.treeRows[a.selectedIdx]
			if row.IsHost {
				a.hostExpanded[row.Host] = false
				a.refreshFlows()
			} else {
				a.jumpToParentHost(row.Host)
			}
		}
	case "f":
		if a.selectedIdx < len(a.treeRows) {
			row := a.treeRows[a.selectedIdx]
			a.focusHost = row.Host
			a.listMode = modeFocus
			a.selectedIdx = 0
			a.refreshFlows()
		}
	case "enter":
		if a.selectedIdx < len(a.treeRows) {
			row := a.treeRows[a.selectedIdx]
			if row.IsHost {
				a.hostExpanded[row.Host] = !a.hostExpanded[row.Host]
				a.refreshFlows()
			} else {
				a.openFlowDetail(row.Flow.ID)
			}
		}
	}
	return a, nil
}

func (a *App) updateFocusList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.listMode = modeTree
		a.selectedIdx = 0
		a.listOffset = 0
		a.refreshFlows()
	case "t":
		a.listMode = modeFlat
		a.selectedIdx = 0
		a.listOffset = 0
	case "enter":
		if a.selectedIdx < len(a.treeRows) {
			a.openFlowDetail(a.treeRows[a.selectedIdx].Flow.ID)
		}
	}
	return a, nil
}

// jumpToParentHost moves cursor to the host node for the given host.
func (a *App) jumpToParentHost(host string) {
	for i, row := range a.treeRows {
		if row.IsHost && row.Host == host {
			a.selectedIdx = i
			return
		}
	}
}

func (a *App) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		a.showDetail = false
		a.detailImagePreview = false
		return a, nil
	case "ctrl+c":
		return a, tea.Quit
	case "1", "left":
		a.detailTab = 0
		a.detailImagePreview = false
		a.updateDetailContent()
	case "2", "right":
		a.detailTab = 1
		a.detailImagePreview = false
		a.updateDetailContent()
	case "i":
		a.toggleImagePreview()
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
	case "e":
		return a.editBody()
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
				a.detailImagePreview = false
				a.updateDetailContent()
			}
			return
		}
	}
}

func (a *App) editBody() (tea.Model, tea.Cmd) {
	_, data, err := a.store.Get(a.selectedID)
	if err != nil || data == nil {
		return a, nil
	}

	var body []byte
	var ct string
	if a.detailTab == 0 {
		body = data.RequestBody
		ct = data.RequestHeaders.Get("Content-Type")
	} else {
		body = data.ResponseBody
		ct = data.ResponseHeaders.Get("Content-Type")
	}

	return a, openInEditor(body, ct)
}

func (a *App) initDetailViewport() {
	a.detailVP = viewport.New(a.width, a.height-3)
	a.detailVP.YPosition = 0
	a.detailReady = true
	a.updateDetailContent()
}

func (a *App) toggleImagePreview() {
	_, data, err := a.store.Get(a.selectedID)
	if err != nil || data == nil {
		return
	}
	var ct string
	if a.detailTab == 0 {
		ct = data.RequestHeaders.Get("Content-Type")
	} else {
		ct = data.ResponseHeaders.Get("Content-Type")
	}
	if isRenderableImage(ct) {
		a.detailImagePreview = !a.detailImagePreview
	}
}

func (a *App) updateDetailContent() {
	meta, data, err := a.store.Get(a.selectedID)
	if err != nil {
		a.detailVP.SetContent("Flow no longer available. Press Esc to return.")
		return
	}

	if a.detailImagePreview && data != nil {
		var body []byte
		if a.detailTab == 0 {
			body = data.RequestBody
		} else {
			body = data.ResponseBody
		}
		vpH := a.height - 3 // header + tabs + status
		out, renderErr := renderImage(body, a.width, vpH)
		if renderErr == nil {
			a.detailVP.SetContent(out)
			return
		}
		// Fall through to normal rendering on error.
		a.detailImagePreview = false
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

	var help string
	switch a.listMode {
	case modeTree:
		help = "t:flat  f:focus  / filter"
	case modeFocus:
		help = "t:flat  Esc:unfocus  / filter"
	default:
		help = "t:tree  / filter"
	}

	gap := max(a.width-len(warning)-len(info)-len(help), 2)
	return styleStatusBar.Width(a.width).Render(
		warning + info + fmt.Sprintf("%*s", gap, help),
	)
}
