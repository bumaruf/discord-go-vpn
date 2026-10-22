package wg

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func key(fill byte) string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{fill}, 32))
}

func sampleConf(priv, pub string) string {
	// A literal IP endpoint keeps ResolveUDPAddr off DNS, so the test needs no
	// network.
	return fmt.Sprintf(`
# sample profile
[Interface]
PrivateKey = %s
Address = 10.2.0.2/32, fd00::2/128
DNS = 10.2.0.1, 1.1.1.1
MTU = 1380

[Peer]
PublicKey = %s
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = 203.0.113.10:51820
PersistentKeepalive = 25
`, priv, pub)
}

func TestParseReadsEssentialFields(t *testing.T) {
	priv, pub := key(1), key(2)

	cfg, err := Parse(strings.NewReader(sampleConf(priv, pub)))
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}

	if cfg.PrivateKey != priv {
		t.Errorf("PrivateKey = %q, want %q", cfg.PrivateKey, priv)
	}
	if len(cfg.Addresses) != 2 {
		t.Fatalf("Addresses has %d entries, want 2", len(cfg.Addresses))
	}
	if got := cfg.Addresses[0].String(); got != "10.2.0.2" {
		t.Errorf("Addresses[0] = %q, want 10.2.0.2", got)
	}
	if len(cfg.DNS) != 2 {
		t.Errorf("DNS has %d entries, want 2", len(cfg.DNS))
	}
	if cfg.MTU != 1380 {
		t.Errorf("MTU = %d, want 1380", cfg.MTU)
	}
	if cfg.Peer.Keepalive != 25 {
		t.Errorf("Keepalive = %d, want 25", cfg.Peer.Keepalive)
	}
}

func TestMTUUnsetWhenOmitted(t *testing.T) {
	conf := fmt.Sprintf("[Interface]\nPrivateKey=%s\nAddress=10.2.0.2/32\n[Peer]\nPublicKey=%s\nEndpoint=203.0.113.10:51820\n", key(1), key(2))

	cfg, err := Parse(strings.NewReader(conf))
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}
	if cfg.MTU != 0 {
		t.Errorf("MTU = %d, want 0 for the transport's default", cfg.MTU)
	}
	if len(cfg.DNS) != 1 {
		t.Fatalf("DNS has %d entries, want the single default", len(cfg.DNS))
	}
}

func TestUAPIEncodesKeysAsHex(t *testing.T) {
	priv, pub := key(1), key(2)

	cfg, err := Parse(strings.NewReader(sampleConf(priv, pub)))
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}

	uapi, err := cfg.UAPI()
	if err != nil {
		t.Fatalf("UAPI returned an error: %v", err)
	}

	wantPriv := "private_key=" + hex.EncodeToString(bytes.Repeat([]byte{1}, 32))
	wantPub := "public_key=" + hex.EncodeToString(bytes.Repeat([]byte{2}, 32))

	for _, want := range []string{
		wantPriv,
		wantPub,
		"endpoint=203.0.113.10:51820",
		"allowed_ip=0.0.0.0/0",
		"allowed_ip=::/0",
		"persistent_keepalive_interval=25",
	} {
		if !strings.Contains(uapi, want) {
			t.Errorf("UAPI lacks %q\noutput:\n%s", want, uapi)
		}
	}

	// wireguard-go would take an all-zero preshared key as a real one.
	if strings.Contains(uapi, "preshared_key=") {
		t.Errorf("UAPI has preshared_key although the file has none:\n%s", uapi)
	}
}

func TestDefaultAllowedIPsWhenOmitted(t *testing.T) {
	conf := fmt.Sprintf("[Interface]\nPrivateKey=%s\nAddress=10.2.0.2/32\n[Peer]\nPublicKey=%s\nEndpoint=203.0.113.10:51820\n", key(1), key(2))

	cfg, err := Parse(strings.NewReader(conf))
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}
	uapi, err := cfg.UAPI()
	if err != nil {
		t.Fatalf("UAPI returned an error: %v", err)
	}
	// Without any allowed_ip the peer accepts no packets and the tunnel is silent.
	if !strings.Contains(uapi, "allowed_ip=0.0.0.0/0") {
		t.Errorf("UAPI did not fall back to the default allowed_ip:\n%s", uapi)
	}
}

func TestConfRoundTripsThroughParse(t *testing.T) {
	cfg, err := Parse(strings.NewReader(sampleConf(key(1), key(2))))
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}
	cfg.Peer.PresharedKey = key(3)

	again, err := Parse(strings.NewReader(cfg.Conf()))
	if err != nil {
		t.Fatalf("Parse(Conf()) returned an error: %v\nconf:\n%s", err, cfg.Conf())
	}

	want := *cfg
	want.MTU = 0
	if !reflect.DeepEqual(again, &want) {
		t.Errorf("Parse(Conf()) =\n%+v\nwant\n%+v\nconf:\n%s", again, &want, cfg.Conf())
	}
}

func TestConfFallsBackToDefaultAllowedIPs(t *testing.T) {
	cfg, err := Parse(strings.NewReader(fmt.Sprintf("[Interface]\nPrivateKey=%s\nAddress=10.2.0.2/32\n[Peer]\nPublicKey=%s\nEndpoint=203.0.113.10:51820\n", key(1), key(2))))
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}
	if conf := cfg.Conf(); !strings.Contains(conf, "AllowedIPs = 0.0.0.0/0, ::/0\n") {
		t.Errorf("Conf did not fall back to the default AllowedIPs:\n%s", conf)
	}
}

func TestParseRejectsIncompleteOrInvalidFile(t *testing.T) {
	cases := map[string]string{
		"no PrivateKey":  fmt.Sprintf("[Interface]\nAddress=10.2.0.2/32\n[Peer]\nPublicKey=%s\nEndpoint=203.0.113.10:51820\n", key(2)),
		"no Address":     fmt.Sprintf("[Interface]\nPrivateKey=%s\n[Peer]\nPublicKey=%s\nEndpoint=203.0.113.10:51820\n", key(1), key(2)),
		"no PublicKey":   fmt.Sprintf("[Interface]\nPrivateKey=%s\nAddress=10.2.0.2/32\n[Peer]\nEndpoint=203.0.113.10:51820\n", key(1)),
		"no Endpoint":    fmt.Sprintf("[Interface]\nPrivateKey=%s\nAddress=10.2.0.2/32\n[Peer]\nPublicKey=%s\n", key(1), key(2)),
		"short key":      fmt.Sprintf("[Interface]\nPrivateKey=%s\nAddress=10.2.0.2/32\n[Peer]\nPublicKey=%s\nEndpoint=203.0.113.10:51820\n", base64.StdEncoding.EncodeToString([]byte("short")), key(2)),
		"key not base64": fmt.Sprintf("[Interface]\nPrivateKey=not!!base64\nAddress=10.2.0.2/32\n[Peer]\nPublicKey=%s\nEndpoint=203.0.113.10:51820\n", key(2)),
	}

	for name, conf := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(conf)); err == nil {
				t.Errorf("Parse accepted an invalid file (%s)", name)
			}
		})
	}
}

func TestCommentsAndSpacesAreIgnored(t *testing.T) {
	conf := fmt.Sprintf("  [Interface]  \n PrivateKey = %s  # the key\n Address = 10.2.0.2/32 ; end\n\n[Peer]\nPublicKey=%s\nEndpoint=203.0.113.10:51820\n", key(1), key(2))

	cfg, err := Parse(strings.NewReader(conf))
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}
	if cfg.PrivateKey != key(1) {
		t.Errorf("a trailing comment leaked into the value: %q", cfg.PrivateKey)
	}
}
