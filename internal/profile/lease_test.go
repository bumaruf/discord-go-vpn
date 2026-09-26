package profile

import (
	"encoding/base64"
	"testing"
)

// A generated key must pass the validator that reads profiles from disk, or the
// tunnel would only fail when coming up.
func TestKeyPairHasWireGuardFormat(t *testing.T) {
	priv, pub, err := keyPair()
	if err != nil {
		t.Fatalf("keyPair: %v", err)
	}
	for name, key := range map[string]string{"private": priv, "public": pub} {
		raw, err := base64.StdEncoding.DecodeString(key)
		if err != nil {
			t.Errorf("%s key is not base64: %v", name, err)
		}
		if len(raw) != 32 {
			t.Errorf("%s key has %d bytes, want 32", name, len(raw))
		}
	}
	if priv == pub {
		t.Error("the public key equals the private key")
	}
}

func TestKeyPairIsUnique(t *testing.T) {
	a, _, _ := keyPair()
	b, _, _ := keyPair()
	if a == b {
		t.Error("two calls generated the same private key")
	}
}
