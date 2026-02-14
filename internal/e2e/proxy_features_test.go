//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/kostyay/httpmon/internal/throttle"
)

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
