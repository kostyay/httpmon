//go:build linux

package browse

import (
	"fmt"
	"os/exec"
)

func openURL(url string) error {
	cmd := exec.Command("xdg-open", url) // #nosec G204 -- url is from CLI flag
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", out, err)
	}
	return nil
}
