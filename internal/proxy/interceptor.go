package proxy

import (
	"log"
	"time"

	mp "github.com/lqqyt2423/go-mitmproxy/proxy"

	"github.com/kostyay/httpmon/internal/store"
)

// interceptor is a go-mitmproxy addon that captures flows into a RingBuffer.
type interceptor struct {
	mp.BaseAddon
	store *store.RingBuffer
}

func newInterceptor(s *store.RingBuffer) *interceptor {
	return &interceptor{store: s}
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

	// Merge response data with existing request data
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
