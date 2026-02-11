package certutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsInstalled_NonexistentCert(t *testing.T) {
	if IsInstalled("/nonexistent/path/cert.pem") {
		t.Error("IsInstalled should return false for nonexistent cert")
	}
}

func TestIsInstalled_EmptyPath(t *testing.T) {
	if IsInstalled("") {
		t.Error("IsInstalled should return false for empty path")
	}
}

func TestInstall_MissingCertFile(t *testing.T) {
	err := Install("/nonexistent/path/cert.pem")
	if err == nil {
		t.Fatal("expected error for missing cert file")
	}
}

func TestInstall_EmptyPath(t *testing.T) {
	err := Install("")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestInstall_DirectoryInsteadOfFile(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("skipping: requires sudo")
	}
	dir := t.TempDir()
	err := Install(dir)
	if err == nil {
		t.Fatal("expected error when passing directory")
	}
}

func TestInstall_ValidCertFile(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("skipping: requires sudo")
	}
	dir := t.TempDir()
	certPath := filepath.Join(dir, "test-ca.pem")
	if err := os.WriteFile(certPath, []byte("not-a-real-cert"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Install(certPath)
	if err == nil {
		t.Fatal("expected error from platform command")
	}
}
