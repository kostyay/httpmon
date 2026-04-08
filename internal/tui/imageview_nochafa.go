//go:build !chafa

package tui

import "errors"

// renderImage is a stub that returns an error when chafa support is not compiled in.
func renderImage(body []byte, width, height int) (string, error) {
	return "", errors.New("image rendering unavailable (build without chafa support)")
}
