package certutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// IsInstalled checks whether the CA certificate is trusted by the system.
func IsInstalled(certPath string) bool {
	if _, err := os.Stat(certPath); err != nil {
		return false
	}
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("security", "find-certificate",
			"-c", CAName, "-p", "/Library/Keychains/System.keychain").CombinedOutput()
		return err == nil && strings.Contains(string(out), "BEGIN CERTIFICATE")
	case "linux":
		_, err := os.Stat("/usr/local/share/ca-certificates/httpmon.crt")
		return err == nil
	default:
		return false
	}
}

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
	cmd := exec.Command("sudo", "security", "add-trusted-cert", // #nosec G204 -- certPath is an internal file path, not user input
		"-d", "-r", "trustRoot",
		"-k", "/Library/Keychains/System.keychain",
		certPath,
	)
	cmd.Stdin = os.Stdin
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("security add-trusted-cert: %s: %w", out, err)
	}
	return nil
}

func installLinux(certPath string) error {
	dest := "/usr/local/share/ca-certificates/httpmon.crt"

	// Use sudo tee to write cert to privileged path.
	data, err := os.ReadFile(filepath.Clean(certPath))
	if err != nil {
		return fmt.Errorf("read cert: %w", err)
	}

	tee := exec.Command("sudo", "tee", dest) // #nosec G204 -- dest is a constant
	tee.Stdin = strings.NewReader(string(data))
	tee.Stdout = nil // suppress tee stdout
	if out, err := tee.CombinedOutput(); err != nil {
		return fmt.Errorf("write %s: %s: %w", dest, out, err)
	}

	cmd := exec.Command("sudo", "update-ca-certificates")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("update-ca-certificates: %s: %w", out, err)
	}
	return nil
}
