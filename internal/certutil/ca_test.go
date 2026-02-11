package certutil

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureCA_CreatesFiles(t *testing.T) {
	dir := t.TempDir()

	certPath, err := EnsureCA(dir)
	if err != nil {
		t.Fatalf("EnsureCA: %v", err)
	}

	// Cert path should point to the PEM cert file.
	if filepath.Base(certPath) != caCertFileName {
		t.Errorf("certPath base = %q, want %q", filepath.Base(certPath), caCertFileName)
	}

	// All three files should exist.
	for _, name := range []string{caFileName, caCertFileName, caCertCerName} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("missing file %s: %v", name, err)
		}
	}

	// Verify CN is httpmon-ca.
	data, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatal("no PEM block in cert file")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if cert.Subject.CommonName != CAName {
		t.Errorf("CN = %q, want %q", cert.Subject.CommonName, CAName)
	}
}

func TestEnsureCA_Idempotent(t *testing.T) {
	dir := t.TempDir()

	path1, err := EnsureCA(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Read cert content.
	data1, _ := os.ReadFile(path1)

	// Second call should reuse existing.
	path2, err := EnsureCA(dir)
	if err != nil {
		t.Fatal(err)
	}
	data2, _ := os.ReadFile(path2)

	if string(data1) != string(data2) {
		t.Error("EnsureCA regenerated cert on second call")
	}
}

func TestEnsureCA_RegeneratesWrongCN(t *testing.T) {
	dir := t.TempDir()

	// Write a fake CA file with wrong CN by using a modified name.
	// Simplest: just write garbage that won't parse as correct CN.
	if err := os.WriteFile(filepath.Join(dir, caFileName), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}

	certPath, err := EnsureCA(dir)
	if err != nil {
		t.Fatalf("EnsureCA: %v", err)
	}

	data, _ := os.ReadFile(certPath)
	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatal("no PEM block")
	}
	cert, _ := x509.ParseCertificate(block.Bytes)
	if cert.Subject.CommonName != CAName {
		t.Errorf("CN = %q after regeneration, want %q", cert.Subject.CommonName, CAName)
	}
}
