package tui

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"

	tea "charm.land/bubbletea/v2"
)

// buildRepeatRequest constructs an *http.Request from the currently selected flow.
func (a *App) buildRepeatRequest() *http.Request {
	meta, data, err := a.store.Get(a.selectedID)
	if err != nil || meta == nil {
		return nil
	}

	u := fmt.Sprintf("%s://%s%s", meta.Scheme, meta.Host, meta.Path)

	var body *bytes.Reader
	if data != nil && len(data.RequestBody) > 0 {
		body = bytes.NewReader(data.RequestBody)
	} else {
		body = bytes.NewReader(nil)
	}

	req, reqErr := http.NewRequest(meta.Method, u, body)
	if reqErr != nil {
		return nil
	}

	if data != nil {
		for k, vals := range data.RequestHeaders {
			for _, v := range vals {
				req.Header.Add(k, v)
			}
		}
	}

	return req
}

// repeatRequest sends the selected flow's request through the self-proxy.
func (a *App) repeatRequest() tea.Cmd {
	req := a.buildRepeatRequest()
	if req == nil {
		return tea.Printf("Cannot repeat: flow data unavailable")
	}

	proxyAddr := a.proxyAddr()

	return func() tea.Msg {
		proxyURL, _ := url.Parse(fmt.Sprintf("http://localhost%s", proxyAddr))
		client := &http.Client{
			Transport: &http.Transport{
				Proxy:           http.ProxyURL(proxyURL),
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // #nosec G402 -- routes through our own MITM proxy CA
			},
		}

		resp, err := client.Do(req)
		if err != nil {
			return tea.Printf("Repeat failed: %v", err)
		}
		_ = resp.Body.Close()
		return tea.Printf("Request sent (%d)", resp.StatusCode)
	}
}
