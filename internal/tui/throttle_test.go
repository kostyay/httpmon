package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kostyay/httpmon/internal/throttle"
)

type mockThrottleController struct {
	bps     int64
	latency time.Duration
}

func (m *mockThrottleController) SetThrottle(bps int64, latency time.Duration) {
	m.bps = bps
	m.latency = latency
}

func (m *mockThrottleController) GetThrottleBPS() int64             { return m.bps }
func (m *mockThrottleController) GetThrottleLatency() time.Duration { return m.latency }

func newAppWithThrottle(tc ThrottleController) *App {
	m := seedMock(3)
	app := NewApp(AppConfig{
		Store:     m,
		Proxy:     &mockProxyInfo{addr: ":9999"},
		CATrusted: true,
		Throttle:  tc,
	})
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app.Update(TickMsg(time.Now()))
	return app
}

func TestThrottleOpenClose(t *testing.T) {
	tc := &mockThrottleController{}
	app := newAppWithThrottle(tc)

	sendKey(app, "T")
	if !app.showThrottle {
		t.Fatal("T should open throttle modal")
	}

	app.updateThrottle(tea.KeyPressMsg{Code: tea.KeyEscape})
	if app.showThrottle {
		t.Error("Esc should close throttle modal")
	}
}

func TestThrottleTKeyCloses(t *testing.T) {
	tc := &mockThrottleController{}
	app := newAppWithThrottle(tc)

	sendKey(app, "T")
	app.updateThrottle(tea.KeyPressMsg{Code: 'T', Text: "T"})
	if app.showThrottle {
		t.Error("T should close throttle modal")
	}
}

func TestThrottleNavigate(t *testing.T) {
	tc := &mockThrottleController{}
	app := newAppWithThrottle(tc)

	sendKey(app, "T")

	app.updateThrottle(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if app.throttleCursor != 1 {
		t.Errorf("after j: cursor = %d, want 1", app.throttleCursor)
	}

	app.updateThrottle(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if app.throttleCursor != 0 {
		t.Errorf("after k: cursor = %d, want 0", app.throttleCursor)
	}

	// Clamp at 0.
	app.updateThrottle(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if app.throttleCursor != 0 {
		t.Errorf("clamp at 0: cursor = %d", app.throttleCursor)
	}
}

func TestThrottleApplyPreset(t *testing.T) {
	tc := &mockThrottleController{}
	app := newAppWithThrottle(tc)

	sendKey(app, "T")

	// Navigate to "3G" preset (index 1).
	app.updateThrottle(tea.KeyPressMsg{Code: 'j', Text: "j"})
	app.updateThrottle(tea.KeyPressMsg{Code: tea.KeyEnter})

	if app.showThrottle {
		t.Error("Enter should close modal")
	}
	bps3g := throttle.PresetBandwidth("3g")
	if tc.bps != bps3g {
		t.Errorf("bps = %d, want %d (3G)", tc.bps, bps3g)
	}
	if tc.latency != 100*time.Millisecond {
		t.Errorf("latency = %v, want 100ms", tc.latency)
	}
}

func TestThrottleApplyNone(t *testing.T) {
	tc := &mockThrottleController{bps: throttle.PresetBandwidth("3g")}
	app := newAppWithThrottle(tc)

	sendKey(app, "T")

	// Cursor starts on current preset (3G=1). Navigate to None (0).
	app.updateThrottle(tea.KeyPressMsg{Code: 'k', Text: "k"})
	app.updateThrottle(tea.KeyPressMsg{Code: tea.KeyEnter})

	if tc.bps != 0 {
		t.Errorf("bps = %d, want 0 (None)", tc.bps)
	}
}

func TestThrottleStatusLabel(t *testing.T) {
	tc := &mockThrottleController{}
	app := newAppWithThrottle(tc)

	if label := app.throttleStatusLabel(); label != "" {
		t.Errorf("label = %q, want empty when throttle off", label)
	}

	tc.bps = throttle.PresetBandwidth("3g")
	if label := app.throttleStatusLabel(); label == "" {
		t.Error("label should be non-empty when throttle active")
	}
}

func TestThrottleViewContent(t *testing.T) {
	tc := &mockThrottleController{}
	app := newAppWithThrottle(tc)

	sendKey(app, "T")
	view := ansi.Strip(app.viewThrottle())

	if view == "" {
		t.Fatal("viewThrottle should return non-empty")
	}
	for _, want := range []string{"Throttle", "None", "3G", "4G", "WiFi"} {
		if !strings.Contains(view, want) {
			t.Errorf("view should contain %q", want)
		}
	}
}
