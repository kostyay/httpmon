//go:build darwin

package browse

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// proxyState captures the HTTP or HTTPS proxy config for one network service.
type proxyState struct {
	enabled bool
	server  string
	port    int
}

// setSystemProxy configures macOS HTTP+HTTPS proxy on the active network
// service. It returns a cleanup function that restores the original settings.
func setSystemProxy(proxyPort int) (func() error, error) {
	svc, err := activeNetworkService()
	if err != nil {
		return nil, err
	}

	// Snapshot current state.
	origHTTP, err := getProxyState(svc, "getwebproxy")
	if err != nil {
		return nil, fmt.Errorf("read http proxy: %w", err)
	}
	origHTTPS, err := getProxyState(svc, "getsecurewebproxy")
	if err != nil {
		return nil, fmt.Errorf("read https proxy: %w", err)
	}

	portStr := strconv.Itoa(proxyPort)
	if err := networksetup("-setwebproxy", svc, "127.0.0.1", portStr); err != nil {
		return nil, fmt.Errorf("set http proxy: %w", err)
	}
	if err := networksetup("-setsecurewebproxy", svc, "127.0.0.1", portStr); err != nil {
		return nil, fmt.Errorf("set https proxy: %w", err)
	}

	return func() error { return restoreProxy(svc, origHTTP, origHTTPS) }, nil
}

// restoreProxy puts back the original proxy settings for the given service.
func restoreProxy(svc string, http, https proxyState) error {
	var errs []error

	if http.enabled {
		if err := networksetup("-setwebproxy", svc, http.server, strconv.Itoa(http.port)); err != nil {
			errs = append(errs, fmt.Errorf("restore http proxy: %w", err))
		}
	} else {
		if err := networksetup("-setwebproxystate", svc, "off"); err != nil {
			errs = append(errs, fmt.Errorf("disable http proxy: %w", err))
		}
	}

	if https.enabled {
		if err := networksetup("-setsecurewebproxy", svc, https.server, strconv.Itoa(https.port)); err != nil {
			errs = append(errs, fmt.Errorf("restore https proxy: %w", err))
		}
	} else {
		if err := networksetup("-setsecurewebproxystate", svc, "off"); err != nil {
			errs = append(errs, fmt.Errorf("disable https proxy: %w", err))
		}
	}

	return errors.Join(errs...)
}

// activeNetworkService returns the macOS network service name (e.g. "Wi-Fi")
// for the default route's interface.
func activeNetworkService() (string, error) {
	// Get default route interface (e.g. "en0").
	out, err := exec.Command("route", "-n", "get", "default").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("route get default: %s: %w", out, err)
	}
	var iface string
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if after, ok := strings.CutPrefix(line, "interface:"); ok {
			iface = strings.TrimSpace(after)
			break
		}
	}
	if iface == "" {
		return "", fmt.Errorf("no default route interface found")
	}

	// Map interface to network service name via hardware ports.
	out, err = exec.Command("networksetup", "-listallhardwareports").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("listallhardwareports: %s: %w", out, err)
	}
	var currentService string
	sc = bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if after, ok := strings.CutPrefix(line, "Hardware Port:"); ok {
			currentService = strings.TrimSpace(after)
		}
		if after, ok := strings.CutPrefix(line, "Device:"); ok {
			dev := strings.TrimSpace(after)
			if dev == iface {
				return currentService, nil
			}
		}
	}

	return "", fmt.Errorf("no network service found for interface %s", iface)
}

// getProxyState reads the current proxy config for a network service.
// verb is "getwebproxy" or "getsecurewebproxy".
func getProxyState(service, verb string) (proxyState, error) {
	out, err := exec.Command("networksetup", "-"+verb, service).CombinedOutput() // #nosec G204
	if err != nil {
		return proxyState{}, fmt.Errorf("%s %s: %s: %w", verb, service, out, err)
	}

	var ps proxyState
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if after, ok := strings.CutPrefix(line, "Enabled:"); ok {
			ps.enabled = strings.TrimSpace(after) == "Yes"
		}
		if after, ok := strings.CutPrefix(line, "Server:"); ok {
			ps.server = strings.TrimSpace(after)
		}
		if after, ok := strings.CutPrefix(line, "Port:"); ok {
			ps.port, _ = strconv.Atoi(strings.TrimSpace(after))
		}
	}
	return ps, nil
}

// networksetup runs networksetup with the given args.
func networksetup(args ...string) error {
	cmd := exec.Command("networksetup", args...) // #nosec G204 -- args are constructed internally
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("networksetup %s: %s: %w", args[0], out, err)
	}
	return nil
}
