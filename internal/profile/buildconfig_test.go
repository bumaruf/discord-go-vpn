package profile

import (
	"net/netip"
	"reflect"
	"testing"

	"github.com/bumaruf/discord-go-vpn/internal/wg"
)

func TestBuildConfig(t *testing.T) {
	cfg, err := buildConfig("priv", "serverpub", "10.8.0.7/32", "10.8.0.1", "203.0.113.10:51820")
	if err != nil {
		t.Fatalf("buildConfig: %v", err)
	}
	want := &wg.Config{
		PrivateKey: "priv",
		Addresses:  []netip.Addr{netip.MustParseAddr("10.8.0.7")},
		DNS:        []netip.Addr{netip.MustParseAddr("10.8.0.1")},
		MTU:        wg.DefaultMTU,
		Peer: wg.Peer{
			PublicKey:  "serverpub",
			Endpoint:   "203.0.113.10:51820",
			AllowedIPs: []string{"0.0.0.0/0", "::/0"},
			Keepalive:  25,
		},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("buildConfig =\n%+v\nwant\n%+v", cfg, want)
	}
}

func TestBuildConfigKeepsOnlyTheAddressOfTheIPv6Prefix(t *testing.T) {
	cfg, err := buildConfig("priv", "pub", "fd00::7/128", "1.1.1.1", "[2001:db8::1]:51820")
	if err != nil {
		t.Fatalf("buildConfig: %v", err)
	}
	if got := cfg.Addresses; len(got) != 1 || got[0] != netip.MustParseAddr("fd00::7") {
		t.Errorf("Addresses = %v, want [fd00::7]", got)
	}
}

func TestBuildConfigRejectsAddressWithoutPrefix(t *testing.T) {
	for _, address := range []string{"", "10.8.0.7", "not-an-address/32"} {
		if _, err := buildConfig("priv", "pub", address, "10.8.0.1", "203.0.113.10:51820"); err == nil {
			t.Errorf("buildConfig accepted address %q", address)
		}
	}
}

func TestBuildConfigFallsBackToPublicDNS(t *testing.T) {
	for _, dns := range []string{"", "dns.example", "10.8.0.1/32"} {
		cfg, err := buildConfig("priv", "pub", "10.8.0.7/32", dns, "203.0.113.10:51820")
		if err != nil {
			t.Fatalf("buildConfig with dns %q: %v", dns, err)
		}
		if got := cfg.DNS; len(got) != 1 || got[0] != netip.MustParseAddr("1.1.1.1") {
			t.Errorf("dns %q: DNS = %v, want [1.1.1.1]", dns, got)
		}
	}
}
