package lease

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/netip"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/config"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/wireguard"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/wireguard/wireguardtest"
)

type testRegistry struct {
	*Registry
	wg *wireguardtest.Fake
}

// newTestRegistry builds a registry with fake wg and a fixed clock.
func newTestRegistry(t *testing.T) *testRegistry {
	t.Helper()
	wg := wireguardtest.NewFake()
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	return &testRegistry{Registry: NewRegistry(wg, config.DefaultWGSubnet, func() time.Time { return now }, 50), wg: wg}
}

// captureLog redirects the standard logger for the rest of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

// fillAddressRange stores an active lease on every tunnel address.
func (ts *testRegistry) fillAddressRange() {
	for i := FirstIP; i <= LastIP; i++ {
		ts.leases[fmt.Sprintf("lease-%d", i)] = &Lease{IP: fmt.Sprintf("10.9.0.%d", i), ExpiresAt: ts.now().Add(TTL)}
	}
}

func TestFreeIP(t *testing.T) {
	ts := newTestRegistry(t)

	if ip, err := ts.freeIP(nil); err != nil || ip != "10.9.0.10" {
		t.Errorf("empty pool: freeIP() = %q, %v; want 10.9.0.10", ip, err)
	}

	ts.leases["a"] = &Lease{IP: "10.9.0.10"}
	ts.leases["c"] = &Lease{IP: "10.9.0.12"}
	if ip, err := ts.freeIP(nil); err != nil || ip != "10.9.0.11" {
		t.Errorf("with a gap: freeIP() = %q, %v; want 10.9.0.11", ip, err)
	}

	if ip, err := ts.freeIP(map[string]bool{"10.9.0.11": true}); err != nil || ip != "10.9.0.13" {
		t.Errorf("with 10.9.0.11 on the interface: freeIP() = %q, %v; want 10.9.0.13", ip, err)
	}

	ts.fillAddressRange()
	if ip, err := ts.freeIP(nil); err == nil {
		t.Errorf("full pool: freeIP() = %q, want an error", ip)
	}
}

func TestReserveSkipsPeersOnInterface(t *testing.T) {
	ts := newTestRegistry(t)
	// Left on wg0 by a provisiond that restarted, with its client still connected.
	ts.wg.Stats = map[string]wireguard.PeerStat{
		"orphan-key": {PubKey: "orphan-key", AllowedIPs: []netip.Prefix{netip.MustParsePrefix("10.9.0.10/32")}},
	}

	ip, err := ts.Reserve("new-key", "", 0)
	if err != nil || ip != "10.9.0.11" {
		t.Fatalf("Reserve() = %q, %v; want 10.9.0.11", ip, err)
	}
	if len(ts.wg.Removed) != 0 {
		t.Errorf("removed peers = %v, want none", ts.wg.Removed)
	}
}

func TestReserveFailsWithoutDump(t *testing.T) {
	ts := newTestRegistry(t)
	ts.wg.DumpErr = errors.New("wg show failed")

	if ip, err := ts.Reserve("new-key", "", 0); err == nil {
		t.Fatalf("Reserve() = %q, want an error", ip)
	}
	if len(ts.wg.Peers) != 0 {
		t.Errorf("peers = %v, want none added", ts.wg.Peers)
	}
	if len(ts.reservedIPs) != 0 {
		t.Errorf("reserved addresses = %v, want none", ts.reservedIPs)
	}
}

func TestExpireStale(t *testing.T) {
	ts := newTestRegistry(t)
	now := ts.now()
	ts.leases["expired"] = &Lease{PubKey: "expired-key", IP: "10.9.0.10", ExpiresAt: now.Add(-time.Second)}
	ts.leases["boundary"] = &Lease{PubKey: "boundary-key", IP: "10.9.0.11", ExpiresAt: now}
	ts.leases["active"] = &Lease{PubKey: "active-key", IP: "10.9.0.12", ExpiresAt: now.Add(time.Minute)}

	ts.ExpireStale()

	if _, kept := ts.leases["expired"]; kept {
		t.Error("expired lease kept")
	}
	// Expiry is strict: a lease is still valid at exactly ExpiresAt.
	if _, kept := ts.leases["boundary"]; !kept {
		t.Error("lease expiring exactly now was removed")
	}
	if _, kept := ts.leases["active"]; !kept {
		t.Error("active lease removed")
	}
	if len(ts.wg.Removed) != 1 || ts.wg.Removed[0] != "expired-key" {
		t.Errorf("removed peers = %v, want [expired-key]", ts.wg.Removed)
	}
}

func prefixes(ips ...string) []netip.Prefix {
	var p []netip.Prefix
	for _, ip := range ips {
		p = append(p, netip.MustParsePrefix(ip))
	}
	return p
}

func TestRemoveOrphans(t *testing.T) {
	const key = "orphan-public-key"
	now := newTestRegistry(t).now()

	cases := []struct {
		name    string
		peer    wireguard.PeerStat
		removed bool
	}{
		{"no handshake, inside the range", wireguard.PeerStat{AllowedIPs: prefixes("10.9.0.20/32")}, true},
		{"handshake older than the idle limit", wireguard.PeerStat{AllowedIPs: prefixes("10.9.0.20/32"), Handshake: now.Add(-orphanIdle - time.Minute)}, true},
		{"recent handshake", wireguard.PeerStat{AllowedIPs: prefixes("10.9.0.20/32"), Handshake: now.Add(-orphanIdle + time.Minute)}, false},
		{"no allowed IPs", wireguard.PeerStat{}, true},
		{"the range's first and last addresses", wireguard.PeerStat{AllowedIPs: prefixes("10.9.0.10/32", "10.9.0.250/32")}, true},
		{"made by hand below the range", wireguard.PeerStat{AllowedIPs: prefixes("10.9.0.2/32")}, false},
		{"above the range", wireguard.PeerStat{AllowedIPs: prefixes("10.9.0.251/32")}, false},
		{"outside the subnet", wireguard.PeerStat{AllowedIPs: prefixes("10.8.0.20/32")}, false},
		{"a whole prefix", wireguard.PeerStat{AllowedIPs: prefixes("10.9.0.0/24")}, false},
		{"one address outside the range", wireguard.PeerStat{AllowedIPs: prefixes("10.9.0.20/32", "10.9.0.2/32")}, false},
		{"an IPv6 address", wireguard.PeerStat{AllowedIPs: prefixes("fd00::20/128")}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ts := newTestRegistry(t)
			c.peer.PubKey = key
			ts.wg.Stats = map[string]wireguard.PeerStat{key: c.peer}

			ts.RemoveOrphans()
			if removed := slices.Contains(ts.wg.Removed, key); removed != c.removed {
				t.Errorf("removed = %v, want %v", removed, c.removed)
			}
		})
	}

	t.Run("a peer with a lease is kept", func(t *testing.T) {
		ts := newTestRegistry(t)
		ts.leases["lease"] = &Lease{PubKey: key, IP: "10.9.0.30", ExpiresAt: now.Add(TTL)}
		ts.wg.Stats = map[string]wireguard.PeerStat{key: {PubKey: key, AllowedIPs: prefixes("10.9.0.20/32")}}

		ts.RemoveOrphans()
		if len(ts.wg.Removed) != 0 {
			t.Errorf("removed peers = %v, want none", ts.wg.Removed)
		}
	})

	t.Run("a peer whose address is reserved is kept", func(t *testing.T) {
		ts := newTestRegistry(t)
		ts.reservedIPs["10.9.0.20"] = ""
		ts.wg.Stats = map[string]wireguard.PeerStat{key: {PubKey: key, AllowedIPs: prefixes("10.9.0.20/32")}}

		ts.RemoveOrphans()
		if len(ts.wg.Removed) != 0 {
			t.Errorf("removed peers = %v, want none", ts.wg.Removed)
		}
	})

	t.Run("nothing is removed when the dump fails", func(t *testing.T) {
		ts := newTestRegistry(t)
		ts.wg.Stats = map[string]wireguard.PeerStat{key: {PubKey: key}}
		ts.wg.DumpErr = errors.New("wg show failed")

		ts.RemoveOrphans()
		if len(ts.wg.Removed) != 0 {
			t.Errorf("removed peers = %v, want none", ts.wg.Removed)
		}
	})

	t.Run("logs the removed peer", func(t *testing.T) {
		ts := newTestRegistry(t)
		ts.wg.Stats = map[string]wireguard.PeerStat{key: {PubKey: key, AllowedIPs: prefixes("10.9.0.20/32", "10.9.0.21/32")}}
		logged := captureLog(t)

		ts.RemoveOrphans()
		if want := "orphan peer removed: " + wireguard.ShortKey(key) + " at 10.9.0.20, 10.9.0.21"; !strings.Contains(logged.String(), want) {
			t.Errorf("log = %q, want %q", logged.String(), want)
		}
	})
}

func TestReapExpiresLeasesThenRemovesOrphans(t *testing.T) {
	ts := newTestRegistry(t)
	ts.leases["expired"] = &Lease{PubKey: "expired-key", IP: "10.9.0.10", ExpiresAt: ts.now().Add(-time.Second)}
	ts.wg.Stats = map[string]wireguard.PeerStat{"orphan-key": {PubKey: "orphan-key", AllowedIPs: prefixes("10.9.0.20/32")}}

	ts.reap()
	if want := []string{"expired-key", "orphan-key"}; !slices.Equal(ts.wg.Removed, want) {
		t.Errorf("removed peers = %v, want %v", ts.wg.Removed, want)
	}
}

func TestReserveRespectsTheInviteCap(t *testing.T) {
	ts := newTestRegistry(t)
	ts.leases["granted"] = &Lease{PubKey: "granted-key", IP: "10.9.0.10", InviteID: "capped", ExpiresAt: ts.now().Add(TTL)}

	if _, err := ts.Reserve("reserved-key", "capped", 2); err != nil {
		t.Fatalf("second lease: %v", err)
	}
	if _, err := ts.Reserve("over-key", "capped", 2); !errors.Is(err, ErrInviteLimitReached) {
		t.Errorf("third lease: err = %v, want ErrInviteLimitReached (a reservation counts)", err)
	}
	if _, err := ts.Reserve("other-key", "other", 2); err != nil {
		t.Errorf("another invite: %v", err)
	}
	if _, err := ts.Reserve("uncapped-key", "capped", 0); err != nil {
		t.Errorf("no cap: %v", err)
	}
}
