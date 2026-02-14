//go:build e2e

package e2e

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kostyay/httpmon/internal/maplocal"
	"github.com/kostyay/httpmon/internal/throttle"
)

func TestE2E_MapLocal(t *testing.T) {
	t.Parallel()
	h := newHarness(t, multiHandler())

	// Create a local file to serve.
	localFile := t.TempDir() + "/local.json"
	if err := os.WriteFile(localFile, []byte(`{"local":true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Pattern uses wildcard for host (interceptor stores hostname without port).
	h.proxy.MapLocal.AddRule(maplocal.Rule{
		Pattern:   "*/mapped",
		LocalPath: localFile,
	})

	// Make a request to the mapped path and wait for completion.
	h.doGet(t, "/mapped")
	h.waitForText(t, "200")
}

func TestE2E_MapLocal_Indicator(t *testing.T) {
	t.Parallel()
	h := newHarness(t, multiHandler())

	localFile := t.TempDir() + "/indicator.txt"
	if err := os.WriteFile(localFile, []byte("local response"), 0o644); err != nil {
		t.Fatal(err)
	}

	h.proxy.MapLocal.AddRule(maplocal.Rule{
		Pattern:   "*/local-indicator",
		LocalPath: localFile,
	})

	h.doGet(t, "/local-indicator")
	h.waitForText(t, "/local-indicator")

	view := h.view()
	if !strings.Contains(view, "[L]") {
		t.Errorf("maplocal flow should show [L] indicator, got:\n%s", view)
	}
}

func TestE2E_Throttle(t *testing.T) {
	t.Parallel()
	h := newHarness(t, multiHandler())

	// Baseline: unthrottled request.
	start := time.Now()
	h.doGet(t, "/json")
	baseline := time.Since(start)

	h.proxy.SetThrottle(throttle.PresetBandwidth("3g"), 100*time.Millisecond)
	t.Cleanup(func() {
		h.proxy.SetThrottle(0, 0)
	})

	// Throttled request should take noticeably longer.
	start = time.Now()
	h.doGet(t, "/large")
	throttled := time.Since(start)

	// The throttled request with 100ms added latency should be at least
	// 80ms slower than baseline (allowing some margin).
	if throttled < baseline+80*time.Millisecond {
		t.Errorf("throttled request (%v) should be slower than baseline (%v)",
			throttled, baseline)
	}
}
