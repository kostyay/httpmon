package tui

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

var composeMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

func (a *App) initCompose() {
	a.showCompose = true
	a.composeMethod = "GET"
	a.composeFocus = 0

	a.composeURL = textinput.New()
	a.composeURL.Placeholder = "https://api.example.com/v1/resource"
	a.composeURL.CharLimit = 1024
	a.composeURL.Focus()

	a.composeHeaders = textinput.New()
	a.composeHeaders.Placeholder = "Header: value, Header2: value2"
	a.composeHeaders.CharLimit = 2048

	a.composeBody = textinput.New()
	a.composeBody.Placeholder = `{"key":"value"}`
	a.composeBody.CharLimit = 4096
}

func (a *App) updateCompose(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.showCompose = false
		return a, nil
	case "tab":
		// Cycle method.
		for i, m := range composeMethods {
			if m == a.composeMethod {
				a.composeMethod = composeMethods[(i+1)%len(composeMethods)]
				break
			}
		}
		return a, nil
	case "ctrl+j":
		// Move focus down.
		a.composeFocus = (a.composeFocus + 1) % 3
		a.focusComposeField()
		return a, nil
	case "ctrl+k":
		// Move focus up.
		a.composeFocus = (a.composeFocus + 2) % 3
		a.focusComposeField()
		return a, nil
	case "ctrl+s":
		return a.sendCompose()
	}

	// Pass to focused input.
	var cmd tea.Cmd
	switch a.composeFocus {
	case 0:
		a.composeURL, cmd = a.composeURL.Update(msg)
	case 1:
		a.composeHeaders, cmd = a.composeHeaders.Update(msg)
	case 2:
		a.composeBody, cmd = a.composeBody.Update(msg)
	}
	return a, cmd
}

func (a *App) focusComposeField() {
	a.composeURL.Blur()
	a.composeHeaders.Blur()
	a.composeBody.Blur()
	switch a.composeFocus {
	case 0:
		a.composeURL.Focus()
	case 1:
		a.composeHeaders.Focus()
	case 2:
		a.composeBody.Focus()
	}
}

func (a *App) sendCompose() (tea.Model, tea.Cmd) {
	rawURL := a.composeURL.Value()
	if rawURL == "" {
		return a, tea.Printf("URL is required")
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}

	body := []byte(a.composeBody.Value())
	var bodyReader *bytes.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	} else {
		bodyReader = bytes.NewReader(nil)
	}

	req, err := http.NewRequest(a.composeMethod, rawURL, bodyReader)
	if err != nil {
		return a, tea.Printf("Invalid request: %v", err)
	}

	// Parse headers.
	hdrs := a.composeHeaders.Value()
	if hdrs != "" {
		for _, h := range strings.Split(hdrs, ",") {
			h = strings.TrimSpace(h)
			parts := strings.SplitN(h, ":", 2)
			if len(parts) == 2 {
				req.Header.Add(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
			}
		}
	}

	proxyAddr := a.proxyAddr()
	a.showCompose = false

	return a, func() tea.Msg {
		proxyURL, _ := url.Parse(fmt.Sprintf("http://localhost%s", proxyAddr))
		client := &http.Client{
			Transport: &http.Transport{
				Proxy:           http.ProxyURL(proxyURL),
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // #nosec G402 -- routes through our own MITM proxy CA
			},
		}
		resp, doErr := client.Do(req) // #nosec G704 -- user-composed request sent through local proxy
		if doErr != nil {
			return tea.Printf("Send failed: %v", doErr)
		}
		_ = resp.Body.Close()
		return tea.Printf("Sent %s %s → %d", req.Method, rawURL, resp.StatusCode)
	}
}

func (a *App) viewCompose() string {
	var b strings.Builder

	b.WriteString(styleHeader.Render("Compose Request"))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", a.width))
	b.WriteString("\n\n")

	// Method.
	b.WriteString(styleSection.Render("Method"))
	b.WriteString(fmt.Sprintf("  %s  (Tab to cycle)\n\n", a.composeMethod))

	// URL.
	label := "URL"
	if a.composeFocus == 0 {
		label = "▸ URL"
	}
	b.WriteString(styleSection.Render(label))
	b.WriteString("\n  ")
	b.WriteString(a.composeURL.View())
	b.WriteString("\n\n")

	// Headers.
	label = "Headers"
	if a.composeFocus == 1 {
		label = "▸ Headers"
	}
	b.WriteString(styleSection.Render(label))
	b.WriteString("\n  ")
	b.WriteString(a.composeHeaders.View())
	b.WriteString("\n\n")

	// Body.
	label = "Body"
	if a.composeFocus == 2 {
		label = "▸ Body"
	}
	b.WriteString(styleSection.Render(label))
	b.WriteString("\n  ")
	b.WriteString(a.composeBody.View())
	b.WriteString("\n\n")

	// Fill remaining space.
	used := 12 // approx lines used
	for used < a.height-1 {
		b.WriteString("\n")
		used++
	}

	bar := "Ctrl+S:send  Ctrl+J/K:navigate  Tab:method  Esc:cancel"
	b.WriteString(styleStatusBar.Width(a.width).Render(bar))

	return b.String()
}
