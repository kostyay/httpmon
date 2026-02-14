package throttle

import (
	"bytes"
	"io"
	"testing"
	"time"
)

func TestThrottleSlowsTransfer(t *testing.T) {
	// 1KB/s bandwidth, read 512 bytes → should take ~500ms.
	data := bytes.Repeat([]byte("x"), 512)
	r := NewReader(bytes.NewReader(data), 1024, 0)

	start := time.Now()
	_, err := io.ReadAll(r)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatal(err)
	}
	if elapsed < 400*time.Millisecond {
		t.Errorf("expected >= 400ms, got %v", elapsed)
	}
}

func TestThrottlePresets(t *testing.T) {
	presets := map[string]int64{
		"3g":   93750,     // 750kbps = 93750 B/s
		"4g":   500000,    // 4Mbps = 500000 B/s
		"wifi": 3750000,   // 30Mbps
	}
	for name, expected := range presets {
		p := PresetBandwidth(name)
		if p != expected {
			t.Errorf("preset %q = %d, want %d", name, p, expected)
		}
	}
}

func TestThrottleLatency(t *testing.T) {
	data := []byte("hello")
	r := NewReader(bytes.NewReader(data), 0, 200*time.Millisecond)

	start := time.Now()
	_, err := io.ReadAll(r)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatal(err)
	}
	if elapsed < 150*time.Millisecond {
		t.Errorf("expected >= 150ms latency, got %v", elapsed)
	}
}

func TestThrottleOff(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 1024)
	r := NewReader(bytes.NewReader(data), 0, 0)

	start := time.Now()
	out, err := io.ReadAll(r)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1024 {
		t.Errorf("got %d bytes, want 1024", len(out))
	}
	if elapsed > 50*time.Millisecond {
		t.Errorf("throttle off should be near-instant, got %v", elapsed)
	}
}
