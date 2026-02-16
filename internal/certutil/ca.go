package certutil

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

const (
	// CAName is the CN and Organization used for the httpmon root CA.
	CAName = "httpmon-ca"

	caFileName     = "mitmproxy-ca.pem"      // key + cert (what go-mitmproxy loads)
	caCertFileName = "mitmproxy-ca-cert.pem" // cert only (PEM)
	caCertCerName  = "mitmproxy-ca-cert.cer" // cert only (DER-compat)
)

// EnsureCA creates the CA key+cert files in dir if they don't already exist
// with the correct CN. Returns the path to the PEM cert file.
func EnsureCA(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	caPath := filepath.Join(dir, caFileName)
	certPath := filepath.Join(dir, caCertFileName)

	// If files exist and have correct CN, nothing to do.
	if hasCorrectCN(caPath) {
		return certPath, nil
	}

	// Generate fresh CA.
	key, certDER, err := generateCA()
	if err != nil {
		return "", err
	}

	keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", err
	}

	// Write combined key+cert (what go-mitmproxy loads).
	combo, err := os.Create(caPath) // #nosec G304 -- path derived from internal dir constant
	if err != nil {
		return "", err
	}
	defer combo.Close()

	if err := pem.Encode(combo, &pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}); err != nil {
		return "", err
	}
	if err := pem.Encode(combo, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return "", err
	}

	// Write cert-only files.
	certBlock := &pem.Block{Type: "CERTIFICATE", Bytes: certDER}
	for _, name := range []string{caCertFileName, caCertCerName} {
		f, err := os.Create(filepath.Join(dir, name)) // #nosec G304 -- name is a package constant
		if err != nil {
			return "", err
		}
		if err := pem.Encode(f, certBlock); err != nil {
			_ = f.Close()
			return "", err
		}
		_ = f.Close()
	}

	return certPath, nil
}

func generateCA() (*rsa.PrivateKey, []byte, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano() / 100000),
		Subject: pkix.Name{
			CommonName:   CAName,
			Organization: []string{CAName},
		},
		NotBefore:             time.Now().Add(-48 * time.Hour),
		NotAfter:              time.Now().Add(3 * 365 * 24 * time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  true,
		SignatureAlgorithm:    x509.SHA256WithRSA,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageEmailProtection,
			x509.ExtKeyUsageTimeStamping,
			x509.ExtKeyUsageCodeSigning,
			x509.ExtKeyUsageMicrosoftCommercialCodeSigning,
			x509.ExtKeyUsageMicrosoftServerGatedCrypto,
			x509.ExtKeyUsageNetscapeServerGatedCrypto,
		},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	return key, der, nil
}

// hasCorrectCN checks whether the CA file exists and has CN=httpmon-ca.
func hasCorrectCN(caPath string) bool {
	data, err := os.ReadFile(caPath) // #nosec G304 -- internal path from EnsureCA
	if err != nil {
		return false
	}
	// Skip first PEM block (private key), parse second (certificate).
	var block *pem.Block
	for {
		block, data = pem.Decode(data)
		if block == nil {
			return false
		}
		if block.Type == "CERTIFICATE" {
			break
		}
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}
	return cert.Subject.CommonName == CAName
}
