// Package browse launches the default browser with httpmon's proxy configured
// via system proxy settings, and restores them on cleanup.
package browse

import (
	"fmt"
	"os/exec"
)

// Session holds state needed to restore proxy settings on cleanup.
type Session struct {
	cleanup func() error
}

// Start configures the system HTTP/HTTPS proxy, then opens url in the default
// browser. The returned Session must be Stop'd to restore proxy settings.
func Start(proxyPort int, url string) (*Session, error) {
	cleanup, err := setSystemProxy(proxyPort)
	if err != nil {
		return nil, fmt.Errorf("set system proxy: %w", err)
	}

	if err := openURL(url); err != nil {
		// Best-effort restore before returning error.
		_ = cleanup()
		return nil, fmt.Errorf("open browser: %w", err)
	}

	return &Session{cleanup: cleanup}, nil
}

// Stop restores the original system proxy settings.
func (s *Session) Stop() error {
	if s == nil || s.cleanup == nil {
		return nil
	}
	return s.cleanup()
}

func openURL(url string) error {
	cmd := exec.Command("open", url) // #nosec G204 -- url is from CLI flag
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", out, err)
	}
	return nil
}
