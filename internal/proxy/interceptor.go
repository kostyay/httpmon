package proxy

import (
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"sync"
	"time"

	mp "github.com/lqqyt2423/go-mitmproxy/proxy"

	"github.com/kostyay/httpmon/internal/maplocal"
	"github.com/kostyay/httpmon/internal/scripting"
	"github.com/kostyay/httpmon/internal/store"
	"github.com/kostyay/httpmon/internal/throttle"
)

// interceptor is a go-mitmproxy addon that captures flows into a RingBuffer.
type interceptor struct {
	mp.BaseAddon
	store    *store.RingBuffer
	engine   *scripting.Engine  // may be nil
	mapLocal *maplocal.MapLocal // may be nil

	mu              sync.RWMutex
	throttleBPS     int64
	throttleLatency time.Duration
}

type interceptorConfig struct {
	Store           *store.RingBuffer
	Engine          *scripting.Engine
	MapLocal        *maplocal.MapLocal
	ThrottleBPS     int64
	ThrottleLatency time.Duration
}

func newInterceptor(cfg interceptorConfig) *interceptor {
	return &interceptor{
		store:           cfg.Store,
		engine:          cfg.Engine,
		mapLocal:        cfg.MapLocal,
		throttleBPS:     cfg.ThrottleBPS,
		throttleLatency: cfg.ThrottleLatency,
	}
}

// SetThrottle updates throttle settings at runtime (thread-safe).
func (i *interceptor) SetThrottle(bps int64, latency time.Duration) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.throttleBPS = bps
	i.throttleLatency = latency
}

// ThrottleBPS returns current bandwidth limit.
func (i *interceptor) ThrottleBPS() int64 {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.throttleBPS
}

// ThrottleLatency returns current latency setting.
func (i *interceptor) ThrottleLatency() time.Duration {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.throttleLatency
}

func (i *interceptor) Requestheaders(f *mp.Flow) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("interceptor.Requestheaders panic: %v", r)
		}
	}()

	scheme := "http"
	if f.ConnContext != nil && f.ConnContext.ClientConn.Tls {
		scheme = "https"
	}

	meta := store.FlowMeta{
		ID:        f.Id.String(),
		Method:    f.Request.Method,
		Host:      f.Request.URL.Hostname(),
		Path:      f.Request.URL.RequestURI(),
		Scheme:    scheme,
		StartedAt: time.Now(),
		State:     store.StateInProgress,
	}
	i.store.Add(meta)
}

func (i *interceptor) Request(f *mp.Flow) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("interceptor.Request panic: %v", r)
		}
	}()

	id := f.Id.String()
	body := f.Request.Body
	if len(body) > maxBodySize {
		body = body[:maxBodySize]
	}

	i.store.SetData(id, &store.FlowData{
		RequestHeaders: f.Request.Header.Clone(),
		RequestBody:    body,
	})

	// MapLocal: serve local file instead of forwarding to upstream.
	if i.mapLocal != nil {
		host := f.Request.URL.Hostname()
		path := f.Request.URL.RequestURI()
		if respBody, status, matched := i.mapLocal.Match(host, path); matched {
			ct := mime.TypeByExtension(filepath.Ext(path))
			if ct == "" {
				ct = http.DetectContentType(respBody)
			}
			f.Response = &mp.Response{
				StatusCode: status,
				Header:     http.Header{"Content-Type": {ct}},
				Body:       respBody,
			}
			i.store.SetData(id, &store.FlowData{
				RequestHeaders:  f.Request.Header.Clone(),
				RequestBody:     body,
				ResponseHeaders: f.Response.Header.Clone(),
				ResponseBody:    respBody,
			})
			i.store.Update(id, func(m *store.FlowMeta) {
				m.MapLocal = true
				m.StatusCode = status
				m.ContentType = ct
				m.SizeBytes = int64(len(respBody))
				m.Duration = time.Since(m.StartedAt)
				m.State = store.StateCompleted
			})
			return
		}
	}

	// Run scripts on request (before forwarding to upstream).
	if i.engine != nil {
		reqURL := f.Request.URL.String()
		ctx := &scripting.RequestContext{
			Method:  f.Request.Method,
			URL:     reqURL,
			Headers: f.Request.Header.Clone(),
			Body:    f.Request.Body,
		}
		i.engine.RunOnRequest(ctx)

		if ctx.Blocked {
			f.Response = &mp.Response{
				StatusCode: http.StatusForbidden,
				Header:     http.Header{"Content-Type": {"text/plain"}},
				Body:       []byte("Blocked by httpmon script"),
			}
			return
		}

		f.Request.Method = ctx.Method
		if ctx.URL != reqURL {
			if u, err := url.Parse(ctx.URL); err == nil {
				f.Request.URL = u
			}
		}
		f.Request.Header = ctx.Headers
		f.Request.Body = ctx.Body
	}
}

func (i *interceptor) Responseheaders(f *mp.Flow) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("interceptor.Responseheaders panic: %v", r)
		}
	}()

	if f.Response == nil {
		return
	}
	id := f.Id.String()
	i.store.Update(id, func(m *store.FlowMeta) {
		m.StatusCode = f.Response.StatusCode
		m.ContentType = f.Response.Header.Get("Content-Type")
	})
}

// StreamResponseModifier wraps the response body reader with throttling.
func (i *interceptor) StreamResponseModifier(_ *mp.Flow, in io.Reader) io.Reader {
	i.mu.RLock()
	bps := i.throttleBPS
	lat := i.throttleLatency
	i.mu.RUnlock()

	if bps <= 0 && lat <= 0 {
		return in
	}
	return throttle.NewReader(in, bps, lat)
}

func (i *interceptor) Response(f *mp.Flow) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("interceptor.Response panic: %v", r)
			i.markFailed(f)
		}
	}()

	if f.Response == nil {
		i.markFailed(f)
		return
	}

	id := f.Id.String()

	respBody, err := f.Response.DecodedBody()
	if err != nil {
		respBody = f.Response.Body
	}
	if len(respBody) > maxBodySize {
		respBody = respBody[:maxBodySize]
	}

	// Run scripts on response.
	if i.engine != nil {
		reqURL := f.Request.URL.String()
		ctx := &scripting.ResponseContext{
			Status:  f.Response.StatusCode,
			Headers: f.Response.Header.Clone(),
			Body:    respBody,
		}
		i.engine.RunOnResponse(ctx, reqURL)

		f.Response.StatusCode = ctx.Status
		f.Response.Header = ctx.Headers
		respBody = ctx.Body
		f.Response.Body = ctx.Body
	}

	// Merge response data with existing request data.
	_, existingData, _ := i.store.Get(id)
	data := &store.FlowData{
		ResponseHeaders: f.Response.Header.Clone(),
		ResponseBody:    respBody,
	}
	if existingData != nil {
		data.RequestHeaders = existingData.RequestHeaders
		data.RequestBody = existingData.RequestBody
	}
	i.store.SetData(id, data)

	i.store.Update(id, func(m *store.FlowMeta) {
		m.StatusCode = f.Response.StatusCode
		m.ContentType = f.Response.Header.Get("Content-Type")
		m.SizeBytes = int64(len(respBody))
		m.Duration = time.Since(m.StartedAt)
		m.State = store.StateCompleted
	})
}

func (i *interceptor) markFailed(f *mp.Flow) {
	i.store.Update(f.Id.String(), func(m *store.FlowMeta) {
		m.State = store.StateFailed
		m.Duration = time.Since(m.StartedAt)
	})
}
