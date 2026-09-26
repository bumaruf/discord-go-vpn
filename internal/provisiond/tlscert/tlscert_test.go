package tlscert

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureCertificate(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "provisiond.crt"), filepath.Join(dir, "provisiond.key")
	if err := Ensure(certFile, keyFile); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	raw, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatalf("reading the certificate: %v", err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		t.Fatal("the certificate is not PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parsing the certificate: %v", err)
	}
	if cert.SerialNumber == nil || cert.SerialNumber.Sign() <= 0 {
		t.Errorf("serial number = %v, want a positive one", cert.SerialNumber)
	}
	if _, err := os.Stat(keyFile); err != nil {
		t.Errorf("key file: %v", err)
	}
}
