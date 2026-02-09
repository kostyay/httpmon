package certutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Install adds the CA certificate at certPath to the system trust store.
// Requires root/sudo on both darwin and linux.
func Install(certPath string) error {
	if _, err := os.Stat(certPath); err != nil {
		return fmt.Errorf("cert file: %w", err)
	}

	switch runtime.GOOS {
	case "darwin":
		return installDarwin(certPath)
	case "linux":
		return installLinux(certPath)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func installDarwin(certPath string) error {
	cmd := exec.Command("security", "add-trusted-cert",
		"-d", "-r", "trustRoot",
		"-k", "/Library/Keychains/System.keychain",
		certPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("security add-trusted-cert: %s: %w", out, err)
	}
	return nil
}

func installLinux(certPath string) error {
	dest := "/usr/local/share/ca-certificates/httpmon.crt"

	data, err := os.ReadFile(filepath.Clean(certPath))
	if err != nil {
		return fmt.Errorf("read cert: %w", err)
	}
	if err := os.WriteFile(dest, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", dest, err)
	}

	cmd := exec.Command("update-ca-certificates")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("update-ca-certificates: %s: %w", out, err)
	}
	return nil
}
