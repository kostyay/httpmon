package throttle

import (
	"io"
	"strings"
	"time"
)

// PresetBandwidth returns bytes/sec for named presets.
func PresetBandwidth(name string) int64 {
	switch strings.ToLower(name) {
	case "3g":
		return 93750 // 750 kbps
	case "4g":
		return 500000 // 4 Mbps
	case "wifi":
		return 3750000 // 30 Mbps
	default:
		return 0 // no throttle
	}
}

// Reader wraps an io.Reader with bandwidth and latency throttling.
type Reader struct {
	inner     io.Reader
	bps       int64         // bytes per second (0 = unlimited)
	latency   time.Duration // one-time initial delay
	started   bool
	bytesRead int64
	startTime time.Time
}

// NewReader wraps r with optional bandwidth (bytes/sec) and latency.
// Pass 0 for bps or latency to skip that aspect.
func NewReader(r io.Reader, bps int64, latency time.Duration) *Reader {
	return &Reader{
		inner:   r,
		bps:     bps,
		latency: latency,
	}
}

func (r *Reader) Read(p []byte) (int, error) {
	if !r.started {
		r.started = true
		r.startTime = time.Now()
		if r.latency > 0 {
			time.Sleep(r.latency)
		}
	}

	// No bandwidth limit → passthrough.
	if r.bps <= 0 {
		return r.inner.Read(p)
	}

	// Limit read size to ~100ms worth of data for smoother throttling.
	chunk := int(r.bps / 10)
	if chunk < 1 {
		chunk = 1
	}
	if len(p) > chunk {
		p = p[:chunk]
	}

	n, err := r.inner.Read(p)
	r.bytesRead += int64(n)

	// Calculate expected time for bytes read so far.
	expectedDur := time.Duration(float64(r.bytesRead) / float64(r.bps) * float64(time.Second))
	actualDur := time.Since(r.startTime)
	if expectedDur > actualDur {
		time.Sleep(expectedDur - actualDur)
	}

	return n, err
}
