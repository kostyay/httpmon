package proxy

import (
	"bytes"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	mp "github.com/lqqyt2423/go-mitmproxy/proxy"

	"github.com/kostyay/httpmon/internal/bodydecoder"
	"github.com/kostyay/httpmon/internal/procinfo"
	"github.com/kostyay/httpmon/internal/scripting"
	"github.com/kostyay/httpmon/internal/store"
	"github.com/kostyay/httpmon/internal/throttle"
)

// interceptor is a go-mitmproxy addon that captures flows into a RingBuffer.
type interceptor struct {
	mp.BaseAddon
	store      *store.RingBuffer
	engine     *scripting.Engine     // may be nil
	resolver   *procinfo.Resolver    // may be nil
	decoderReg *bodydecoder.Registry // may be nil

	mu              sync.RWMutex
	throttleBPS     int64
	throttleLatency time.Duration
}

type interceptorConfig struct {
	Store           *store.RingBuffer
	Engine          *scripting.Engine
	Resolver        *procinfo.Resolver
	DecoderRegistry *bodydecoder.Registry
	ThrottleBPS     int64
	ThrottleLatency time.Duration
}

func newInterceptor(cfg interceptorConfig) *interceptor {
	return &interceptor{
		store:           cfg.Store,
		engine:          cfg.Engine,
		resolver:        cfg.Resolver,
		decoderReg:      cfg.DecoderRegistry,
		throttleBPS:     cfg.ThrottleBPS,
		throttleLatency: cfg.ThrottleLatency,
	}
}

// runScriptsWithCodec decodes a binary body (e.g. protobuf) into JSON before
// the script callback, then re-encodes to wire format if the script modified it.
// Falls back to raw bytes when decoding is unavailable or re-encoding fails.
func (i *interceptor) runScriptsWithCodec(
	body []byte, contentType string, meta bodydecoder.DecoderMetadata,
	run func(body []byte) (modified []byte, changed bool),
) []byte {
	// No registry or content type not decodable -- run scripts on raw bytes.
	if i.decoderReg == nil {
		result, _ := run(body)
		return result
	}
	decoded, _, err := i.decoderReg.Decode(body, contentType, meta)
	if err != nil {
		result, _ := run(body)
		return result
	}

	// Scripts see decoded JSON.
	result, changed := run([]byte(decoded))
	if !changed {
		return body // unchanged — return original wire bytes
	}

	// Re-encode modified JSON back to wire format.
	meta.OriginalBody = body
	encoded, err := i.decoderReg.Encode(result, contentType, meta)
	if err != nil {
		log.Printf("codec: encode failed, using original body: %v", err)
		return body
	}
	return encoded
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

	flowID := f.Id.String()
	meta := store.FlowMeta{
		ID:        flowID,
		Method:    f.Request.Method,
		Host:      f.Request.URL.Hostname(),
		Path:      f.Request.URL.RequestURI(),
		Scheme:    scheme,
		StartedAt: time.Now(),
		State:     store.StateInProgress,
	}
	i.store.Add(meta)

	if i.resolver != nil && f.ConnContext != nil {
		if port := clientPort(f.ConnContext.ClientConn.Conn); port > 0 {
			i.resolver.Resolve(flowID, port)
		}
	}
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

	// Run scripts on request (before forwarding to upstream).
	if i.engine != nil {
		reqURL := f.Request.URL.String()
		contentType := f.Request.Header.Get("Content-Type")
		meta := bodydecoder.DecoderMetadata{
			RequestPath: f.Request.URL.Path,
			Host:        f.Request.URL.Hostname(),
			IsRequest:   true,
		}

		ctx := &scripting.RequestContext{
			Method:  f.Request.Method,
			URL:     reqURL,
			Headers: f.Request.Header.Clone(),
			Meta:    store.FlowMeta{ID: id, Method: f.Request.Method, Host: f.Request.URL.Hostname()},
		}

		// Transparent codec: decode binary body → JSON for scripts, re-encode after.
		ctx.Body = i.runScriptsWithCodec(f.Request.Body, contentType, meta,
			func(scriptBody []byte) ([]byte, bool) {
				ctx.Body = scriptBody
				i.engine.RunOnRequest(ctx)
				return ctx.Body, !bytes.Equal(scriptBody, ctx.Body)
			},
		)

		if ctx.Responded {
			i.buildSyntheticResponse(f, id, body, ctx.ResponseStatus,
				ctx.ResponseHeaders, ctx.ResponseBody)
			return
		}

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
		contentType := f.Response.Header.Get("Content-Type")
		meta := bodydecoder.DecoderMetadata{
			RequestPath: f.Request.URL.Path,
			Host:        f.Request.URL.Hostname(),
			IsRequest:   false,
		}

		ctx := &scripting.ResponseContext{
			Status:  f.Response.StatusCode,
			Headers: f.Response.Header.Clone(),
			Meta:    store.FlowMeta{ID: id, Method: f.Request.Method, Host: f.Request.URL.Hostname()},
		}

		// Transparent codec: decode binary body → JSON for scripts, re-encode after.
		ctx.Body = i.runScriptsWithCodec(respBody, contentType, meta,
			func(scriptBody []byte) ([]byte, bool) {
				ctx.Body = scriptBody
				i.engine.RunOnResponse(ctx, reqURL)
				return ctx.Body, !bytes.Equal(scriptBody, ctx.Body)
			},
		)

		if ctx.Responded {
			f.Response.StatusCode = ctx.ResponseStatus
			if ctx.ResponseHeaders != nil {
				for k, v := range ctx.ResponseHeaders {
					f.Response.Header.Set(k, v)
				}
			}
			respBody = ctx.ResponseBody
			f.Response.Body = ctx.ResponseBody
		} else {
			f.Response.StatusCode = ctx.Status
			f.Response.Header = ctx.Headers
			respBody = ctx.Body
			f.Response.Body = ctx.Body
		}
	}

	// Merge response fields into existing FlowData (preserves
	// request headers/body and process info set earlier).
	respHeaders := f.Response.Header.Clone()
	i.store.UpdateData(id, func(d *store.FlowData) {
		d.ResponseHeaders = respHeaders
		d.ResponseBody = respBody
	})

	i.store.Update(id, func(m *store.FlowMeta) {
		m.StatusCode = f.Response.StatusCode
		m.ContentType = f.Response.Header.Get("Content-Type")
		m.SizeBytes = int64(len(respBody))
		m.Duration = time.Since(m.StartedAt)
		m.State = store.StateCompleted
	})
}

func (i *interceptor) buildSyntheticResponse(
	f *mp.Flow, id string, reqBody []byte,
	status int, headers map[string]string, body []byte,
) {
	h := http.Header{}
	for k, v := range headers {
		h.Set(k, v)
	}
	f.Response = &mp.Response{
		StatusCode: status,
		Header:     h,
		Body:       body,
	}
	i.store.SetData(id, &store.FlowData{
		RequestHeaders:  f.Request.Header.Clone(),
		RequestBody:     reqBody,
		ResponseHeaders: h.Clone(),
		ResponseBody:    body,
	})
	ct := h.Get("Content-Type")
	i.store.Update(id, func(m *store.FlowMeta) {
		m.StatusCode = status
		m.ContentType = ct
		m.SizeBytes = int64(len(body))
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

// clientPort extracts the ephemeral port from a net.Conn's RemoteAddr.
func clientPort(c net.Conn) uint16 {
	if c == nil {
		return 0
	}
	_, portStr, err := net.SplitHostPort(c.RemoteAddr().String())
	if err != nil {
		return 0
	}
	p, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return 0
	}
	return uint16(p)
}
