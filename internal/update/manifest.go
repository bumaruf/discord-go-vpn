// Package update replaces the client with the newest GitHub release before it
// connects. A release carries update.json, which names each platform's binary
// and its SHA-256, and update.json.sig, its ed25519 signature; the client trusts
// only the public key compiled into it, never the server it downloads from.
package update

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/semver"
)

const (
	ManifestName  = "update.json"
	SignatureName = "update.json.sig"
)

type Manifest struct {
	Version string `json:"version"`
	// Assets is keyed by GOOS/GOARCH, such as "windows/arm64".
	Assets map[string]Asset `json:"assets"`
}

type Asset struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

func Platform(goos, goarch string) string { return goos + "/" + goarch }

// BuildManifest hashes each file in dir; files maps a platform to its file name.
func BuildManifest(version, dir string, files map[string]string) (Manifest, error) {
	if !semver.IsValid(version) {
		return Manifest{}, fmt.Errorf("version %q is not semver, such as v1.2.3", version)
	}
	m := Manifest{Version: version, Assets: make(map[string]Asset, len(files))}
	for platform, name := range files {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return Manifest{}, err
		}
		sum := sha256.Sum256(data)
		m.Assets[platform] = Asset{Name: name, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(data))}
	}
	return m, nil
}

// Sign returns the manifest's bytes and their base64 signature, the contents of
// update.json and update.json.sig.
func Sign(m Manifest, privateKey ed25519.PrivateKey) (manifest, signature []byte, err error) {
	manifest, err = json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	sig := ed25519.Sign(privateKey, manifest)
	return manifest, []byte(base64.StdEncoding.EncodeToString(sig)), nil
}

// Verify checks signature against the exact manifest bytes before parsing them.
func Verify(manifest, signature []byte, publicKey ed25519.PublicKey) (Manifest, error) {
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(signature)))
	if err != nil || !ed25519.Verify(publicKey, manifest, sig) {
		return Manifest{}, errors.New("a assinatura da atualização não confere")
	}
	var m Manifest
	if err := json.Unmarshal(manifest, &m); err != nil {
		return Manifest{}, fmt.Errorf("manifesto da atualização inválido: %w", err)
	}
	if !semver.IsValid(m.Version) {
		return Manifest{}, fmt.Errorf("manifesto da atualização com versão inválida %q", m.Version)
	}
	return m, nil
}

// ParsePrivateKey reads the base64 32-byte seed GenerateKey prints.
func ParsePrivateKey(encoded string) (ed25519.PrivateKey, error) {
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("signing key must be the base64 of a %d-byte ed25519 seed", ed25519.SeedSize)
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

func parsePublicKey(encoded string) (ed25519.PublicKey, error) {
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, errors.New("chave pública de atualização inválida")
	}
	return key, nil
}

// GenerateKey prints a new key pair: the seed goes into the release secret, the
// public key into publicKey.
func GenerateKey(w io.Writer) error {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "private (secret UPDATE_SIGNING_KEY): %s\npublic (internal/update publicKey): %s\n",
		base64.StdEncoding.EncodeToString(priv.Seed()), base64.StdEncoding.EncodeToString(pub))
	return err
}

// VerifyRelease checks a signed manifest against the key compiled into the
// client, so a release can't publish one that every client would reject.
func VerifyRelease(manifest, signature []byte) error {
	key, err := parsePublicKey(publicKey)
	if err != nil {
		return err
	}
	_, err = Verify(manifest, signature, key)
	return err
}
