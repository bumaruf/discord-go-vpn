package tlscert

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"time"
)

// Ensure creates a self-signed certificate on the first run. With no domain there is no
// public certificate, so the client pins this certificate's fingerprint instead.
func Ensure(certFile, keyFile string) error {
	if _, err := os.Stat(certFile); err == nil {
		if _, err := os.Stat(keyFile); err == nil {
			return nil
		}
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	template := x509.Certificate{
		Subject:               pkix.Name{CommonName: "discord-go-vpn provisiond"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("0.0.0.0")},
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return err
	}

	if err := writePEM(certFile, "CERTIFICATE", der, 0o644); err != nil {
		return err
	}
	return writePEM(keyFile, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key), 0o600)
}

func writePEM(path, blockType string, der []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer file.Close()
	return pem.Encode(file, &pem.Block{Type: blockType, Bytes: der})
}

// Fingerprint returns the certificate's SHA-256 in the format the client embeds.
func Fingerprint(certFile string) (string, error) {
	raw, err := os.ReadFile(certFile)
	if err != nil {
		return "", err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return "", fmt.Errorf("%s is not a valid PEM file", certFile)
	}

	sum := sha256.Sum256(block.Bytes)
	return base64.StdEncoding.EncodeToString(sum[:]), nil
}
