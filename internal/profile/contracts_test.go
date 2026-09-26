package profile

import (
	"net/netip"
	"testing"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/contracttest"
	"github.com/bumaruf/discord-go-vpn/internal/profile/profiletest"
)

func TestLeaseRequestMatchesFixture(t *testing.T) {
	contracttest.AssertRoundTrip[leaseRequest](t, "lease-request.json")
}

// The client decodes /lease into an unexported anonymous struct, so the fixture is
// checked through what Acquire builds from it: every key has an observable effect.
func TestAcquireReadsLeaseResponseFixture(t *testing.T) {
	f := profiletest.NewProvisiond(t)
	f.LeaseBody = contracttest.Read(t, "lease-response.json")
	ticker := newManualTicker()
	c := newClient(f)
	c.newTicker = ticker.newTicker

	lease, err := c.Acquire()
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	// ttl_seconds 180 renews every 60s; a missing TTL would fall back to 20s.
	if got := receive(t, ticker.intervals, "renew interval"); got != 60*time.Second {
		t.Errorf("renew interval = %v, want 60s", got)
	}

	cfg := lease.Config
	if got := cfg.Addresses; len(got) != 1 || got[0] != netip.MustParseAddr("10.9.0.10") {
		t.Errorf("Addresses = %v, want [10.9.0.10]", got)
	}
	// An unparsable dns falls back to 1.1.1.1.
	if got := cfg.DNS; len(got) != 1 || got[0] != netip.MustParseAddr("10.9.0.1") {
		t.Errorf("DNS = %v, want [10.9.0.1]", got)
	}
	if cfg.Peer.PublicKey != "bXlzZXJ2ZXJwdWJsaWNrZXl0aGF0aXMzMmJ5dGVzISE=" {
		t.Errorf("Peer.PublicKey = %q", cfg.Peer.PublicKey)
	}
	if cfg.Peer.Endpoint != "203.0.113.1:51820" {
		t.Errorf("Peer.Endpoint = %q", cfg.Peer.Endpoint)
	}

	lease.Release()
	if calls := f.Calls("/release"); len(calls) != 1 || calls[0]["id"] != "9f86d081884c7d65" {
		t.Errorf("/release calls = %v, want one with id 9f86d081884c7d65", calls)
	}
}
