//go:build linux

package browse

import "fmt"

// setSystemProxy is a no-op on Linux — system proxy configuration varies
// widely across desktop environments and distros.
func setSystemProxy(_ int) (func() error, error) {
	return nil, fmt.Errorf("system proxy configuration not supported on Linux")
}
