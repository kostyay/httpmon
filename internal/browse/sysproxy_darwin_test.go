//go:build darwin

package browse

import (
	"testing"
)

func TestActiveNetworkService(t *testing.T) {
	svc, err := activeNetworkService()
	if err != nil {
		t.Skipf("no active network service: %v", err)
	}
	if svc == "" {
		t.Fatal("empty service name")
	}
	t.Logf("active service: %s", svc)
}

func TestGetProxyState(t *testing.T) {
	svc, err := activeNetworkService()
	if err != nil {
		t.Skipf("no active network service: %v", err)
	}

	ps, err := getProxyState(svc, "getwebproxy")
	if err != nil {
		t.Fatalf("getwebproxy: %v", err)
	}
	t.Logf("HTTP proxy enabled=%v server=%q port=%d", ps.enabled, ps.server, ps.port)

	ps, err = getProxyState(svc, "getsecurewebproxy")
	if err != nil {
		t.Fatalf("getsecurewebproxy: %v", err)
	}
	t.Logf("HTTPS proxy enabled=%v server=%q port=%d", ps.enabled, ps.server, ps.port)
}
