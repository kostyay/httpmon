package proxy

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/kostyay/httpmon/internal/store"
	"github.com/kostyay/httpmon/internal/throttle"
)

func TestStreamResponseModifierNoThrottle(t *testing.T) {
	s := store.New(100)
	ic := newInterceptor(interceptorConfig{Store: s})

	in := strings.NewReader("hello")
	out := ic.StreamResponseModifier(nil, in)

	if out != in {
		t.Error("should return original reader when no throttle")
	}
}

func TestStreamResponseModifierWithThrottle(t *testing.T) {
	s := store.New(100)
	ic := newInterceptor(interceptorConfig{
		Store:       s,
		ThrottleBPS: throttle.PresetBandwidth("wifi"),
	})

	in := strings.NewReader("hello world")
	out := ic.StreamResponseModifier(nil, in)

	if out == in {
		t.Error("should wrap reader when throttle active")
	}

	data, err := io.ReadAll(out)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("data = %q, want %q", string(data), "hello world")
	}
}

func TestStreamResponseModifierLatencyOnly(t *testing.T) {
	s := store.New(100)
	ic := newInterceptor(interceptorConfig{
		Store:           s,
		ThrottleLatency: 1 * time.Millisecond,
	})

	in := strings.NewReader("test")
	out := ic.StreamResponseModifier(nil, in)

	if out == in {
		t.Error("should wrap reader when latency > 0")
	}
}

func TestSetThrottleRuntimeChange(t *testing.T) {
	s := store.New(100)
	ic := newInterceptor(interceptorConfig{Store: s})

	if ic.ThrottleBPS() != 0 {
		t.Errorf("initial BPS = %d, want 0", ic.ThrottleBPS())
	}

	bps3g := throttle.PresetBandwidth("3g")
	ic.SetThrottle(bps3g, 100*time.Millisecond)

	if ic.ThrottleBPS() != bps3g {
		t.Errorf("BPS = %d, want %d", ic.ThrottleBPS(), bps3g)
	}
	if ic.ThrottleLatency() != 100*time.Millisecond {
		t.Errorf("Latency = %v, want 100ms", ic.ThrottleLatency())
	}

	// After runtime change, StreamResponseModifier should wrap.
	in := strings.NewReader("data")
	out := ic.StreamResponseModifier(nil, in)
	if out == in {
		t.Error("should wrap after SetThrottle")
	}
}
