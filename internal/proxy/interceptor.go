package proxy

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	mp "github.com/lqqyt2423/go-mitmproxy/proxy"

	"github.com/kostyay/httpmon/internal/bodydecoder"
	"github.com/kostyay/httpmon/internal/hostfilter"
	"github.com/kostyay/httpmon/internal/procinfo"
	"github.com/kostyay/httpmon/internal/scripting"
	"github.com/kostyay/httpmon/internal/store"
	"github.com/kostyay/httpmon/internal/throttle"
)

// interceptor is a go-mitmproxy addon that captures flows into a RingBuffer.
type interceptor struct {
	mp.BaseAddon
	store      *store.RingBuffer
	engine     *scripting.Engine      // may be nil
	resolver   *procinfo.Resolver     // may be nil
	decoderReg *bodydecoder.Registry  // may be nil
	hostFilter *hostfilter.HostFilter // may be nil

	mu              sync.RWMutex
	throttleBPS     int64
	throttleLatency time.Duration
}

type interceptorConfig struct {
	Store           *store.RingBuffer
	Engine          *scripting.Engine
	Resolver        *procinfo.Resolver
	DecoderRegistry *bodydecoder.Registry
	HostFilter      *hostfilter.HostFilter
	ThrottleBPS     int64
	ThrottleLatency time.Duration
}

func newInterceptor(cfg interceptorConfig) *interceptor {
	return &interceptor{
		store:           cfg.Store,
		engine:          cfg.Engine,
		resolver:        cfg.Resolver,
		decoderReg:      cfg.DecoderRegistry,
		hostFilter:      cfg.HostFilter,
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
	runRaw := func() []byte { result, _ := run(body); return result }

	if i.decoderReg == nil {
		return runRaw()
	}
	decoded, _, err := i.decoderReg.Decode(body, contentType, meta)
	if err != nil {
		return runRaw()
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

	// Skip flows for hosts the filter excludes (e.g. --allow / --block).
	// go-mitmproxy calls Requestheaders for CONNECT before checking
	// ShouldInterceptRule, so without this guard tunneled hosts leak
	// into the store.
	if i.hostFilter != nil && !i.hostFilter.ShouldIntercept(f.Request.URL.Host) {
		return
	}

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
	reqBody := f.Request.Body
	if len(reqBody) > maxBodySize {
		reqBody = reqBody[:maxBodySize]
	}

	i.store.SetData(id, &store.FlowData{
		RequestHeaders: f.Request.Header.Clone(),
		RequestBody:    reqBody,
	})

	if i.engine != nil && i.applyRequestScripts(f, id, reqBody) {
		return
	}
}

// applyRequestScripts runs the script engine on the outbound request.
// It returns true when a synthetic or blocked response has been set and
// the caller should return immediately without forwarding the request.
func (i *interceptor) applyRequestScripts(f *mp.Flow, id string, reqBody []byte) bool {
	reqURL := f.Request.URL.String()
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
	ctx.Body = i.runScriptsWithCodec(f.Request.Body, f.Request.Header.Get("Content-Type"), meta,
		func(scriptBody []byte) ([]byte, bool) {
			ctx.Body = scriptBody
			i.engine.RunOnRequest(ctx)
			return ctx.Body, !bytes.Equal(scriptBody, ctx.Body)
		},
	)

	if ctx.Responded {
		i.buildSyntheticResponse(f, id, reqBody, ctx.ResponseStatus, ctx.ResponseHeaders, ctx.ResponseBody)
		return true
	}
	if ctx.Blocked {
		f.Response = &mp.Response{
			StatusCode: http.StatusForbidden,
			Header:     http.Header{"Content-Type": {"text/plain"}},
			Body:       []byte("Blocked by httpmon script"),
		}
		return true
	}

	f.Request.Method = ctx.Method
	if ctx.URL != reqURL {
		if u, err := url.Parse(ctx.URL); err == nil {
			f.Request.URL = u
		}
	}
	f.Request.Header = ctx.Headers
	f.Request.Body = ctx.Body
	return false
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
	if i.engine != nil {
		respBody = i.applyResponseScripts(f, id, respBody)
	}

	// Truncate only the ring-buffer copy; the actual response body is unaffected.
	storedBody := respBody
	if len(storedBody) > maxBodySize {
		storedBody = storedBody[:maxBodySize]
	}

	// Merge response fields into existing FlowData (preserves
	// request headers/body and process info set earlier).
	respHeaders := f.Response.Header.Clone()
	i.store.UpdateData(id, func(d *store.FlowData) {
		d.ResponseHeaders = respHeaders
		d.ResponseBody = storedBody
	})

	i.store.Update(id, func(m *store.FlowMeta) {
		m.StatusCode = f.Response.StatusCode
		m.ContentType = f.Response.Header.Get("Content-Type")
		m.SizeBytes = int64(len(respBody))
		m.Duration = time.Since(m.StartedAt)
		m.State = store.StateCompleted
	})
}

// recompress compresses data with the given encoding (gzip, br, deflate, zstd).
// Returns nil on unsupported encoding or error.
func recompress(data []byte, encoding string) []byte {
	var buf bytes.Buffer
	switch encoding {
	case "gzip":
		w := gzip.NewWriter(&buf)
		if _, err := w.Write(data); err != nil {
			return nil
		}
		if err := w.Close(); err != nil {
			return nil
		}
	case "br":
		w := brotli.NewWriter(&buf)
		if _, err := w.Write(data); err != nil {
			return nil
		}
		if err := w.Close(); err != nil {
			return nil
		}
	case "deflate":
		w, _ := flate.NewWriter(&buf, flate.DefaultCompression)
		if _, err := w.Write(data); err != nil {
			return nil
		}
		if err := w.Close(); err != nil {
			return nil
		}
	case "zstd":
		w, err := zstd.NewWriter(&buf)
		if err != nil {
			return nil
		}
		if _, err := w.Write(data); err != nil {
			return nil
		}
		if err := w.Close(); err != nil {
			return nil
		}
	default:
		return nil
	}
	return buf.Bytes()
}

// applyResponseScripts runs the script engine on the inbound response,
// updating f.Response in-place and returning the final (decoded) body.
// The caller must not truncate decoded before passing it: scripts need the
// full content, and f.Response.Body is set to the complete result.
//
// When the original response was compressed, the body is re-compressed
// after script processing to preserve the encoding for the client and
// avoid inflating the response beyond the StreamLargeBodies threshold.
func (i *interceptor) applyResponseScripts(f *mp.Flow, id string, decoded []byte) []byte {
	origEncoding := f.Response.Header.Get("Content-Encoding")

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
	ctx.Body = i.runScriptsWithCodec(decoded, f.Response.Header.Get("Content-Type"), meta,
		func(scriptBody []byte) ([]byte, bool) {
			ctx.Body = scriptBody
			i.engine.RunOnResponse(ctx, f.Request.URL.String())
			return ctx.Body, !bytes.Equal(scriptBody, ctx.Body)
		},
	)

	var finalDecoded []byte
	if ctx.Responded {
		f.Response.StatusCode = ctx.ResponseStatus
		h := http.Header{}
		for k, v := range ctx.ResponseHeaders {
			h.Set(k, v)
		}
		f.Response.Header = h
		f.Response.Body = ctx.ResponseBody
		finalDecoded = ctx.ResponseBody
	} else {
		f.Response.StatusCode = ctx.Status
		f.Response.Header = ctx.Headers
		finalDecoded = ctx.Body

		// Re-compress the body if the original response was compressed.
		// This keeps the wire size small and avoids exceeding the
		// StreamLargeBodies threshold with the decoded body.
		if origEncoding != "" {
			if compressed := recompress(finalDecoded, origEncoding); compressed != nil {
				f.Response.Body = compressed
				f.Response.Header.Set("Content-Encoding", origEncoding)
				f.Response.Header.Set("Content-Length", strconv.Itoa(len(compressed)))
				f.Response.Header.Del("Transfer-Encoding")
				return finalDecoded
			}
		}

		// No original encoding or re-compression failed: send decoded.
		f.Response.Body = finalDecoded
	}
	f.Response.Header.Del("Content-Encoding")
	f.Response.Header.Del("Transfer-Encoding")
	f.Response.Header.Set("Content-Length", strconv.Itoa(len(f.Response.Body)))
	return finalDecoded
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
	i.store.Update(id, func(m *store.FlowMeta) {
		m.StatusCode = status
		m.ContentType = h.Get("Content-Type")
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
