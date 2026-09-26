package lease

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/wireguard"
)

const snapshotInvite = "65f1a2b3c4d5e6f7a8b9c0d0"

// peerAt returns the dump entry of a peer holding just ip.
func peerAt(publicKey, ip string) wireguard.PeerStat {
	return wireguard.PeerStat{PubKey: publicKey, AllowedIPs: prefixes(ip + "/32")}
}

// grant reserves an address for publicKey, stores its lease under id and puts its peer in
// the dump, as the kernel would.
func (ts *testRegistry) grant(t *testing.T, id, publicKey string) Lease {
	t.Helper()
	ip, err := ts.Reserve(publicKey, snapshotInvite, 0)
	if err != nil {
		t.Fatalf("reserving %s: %v", id, err)
	}
	ts.Grant(id, Lease{PubKey: publicKey, IP: ip, InviteID: snapshotInvite, Label: "laptop", OSUser: "ana", DeviceID: "device-" + id})
	if ts.wg.Stats == nil {
		ts.wg.Stats = make(map[string]wireguard.PeerStat)
	}
	ts.wg.Stats[publicKey] = peerAt(publicKey, ip)
	return ts.Leases()[id]
}

// restart returns a registry on the same interface, as a new process would build it.
func (ts *testRegistry) restart(t *testing.T) *testRegistry {
	t.Helper()
	next := newTestRegistry(t)
	next.wg.Stats = ts.wg.Stats
	return next
}

func snapshotPath(t *testing.T) string {
	return filepath.Join(t.TempDir(), "leases.json")
}

func restore(t *testing.T, ts *testRegistry, path string) []string {
	t.Helper()
	ids, err := ts.Restore(path)
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	slices.Sort(ids)
	return ids
}

func TestSaveAndRestoreKeepLeases(t *testing.T) {
	ts := newTestRegistry(t)
	a := ts.grant(t, "a", "key-a")
	b := ts.grant(t, "b", "key-b")
	path := snapshotPath(t)

	if err := ts.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	next := ts.restart(t)
	ids := restore(t, next, path)

	if !slices.Equal(ids, []string{"a", "b"}) {
		t.Fatalf("restored ids = %v, want [a b]", ids)
	}
	leases := next.Leases()
	if !leases["a"].ExpiresAt.Equal(a.ExpiresAt) || !leases["a"].Since.Equal(a.Since) {
		t.Errorf("restored times = %v/%v, want %v/%v", leases["a"].ExpiresAt, leases["a"].Since, a.ExpiresAt, a.Since)
	}
	got, want := leases["b"], b
	got.ExpiresAt, got.Since, want.ExpiresAt, want.Since = time.Time{}, time.Time{}, time.Time{}, time.Time{}
	if got != want {
		t.Errorf("restored lease = %+v, want %+v", got, want)
	}
	if !next.Renew("a", snapshotInvite) {
		t.Error("Renew() of a restored lease = false, want true")
	}
	if len(next.wg.Peers) != 0 || len(next.wg.Removed) != 0 {
		t.Errorf("restore touched the interface: added %v, removed %v", next.wg.Peers, next.wg.Removed)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the file is still there after Restore (stat error %v)", err)
	}
}

func TestRestoredLeaseKeepsItsAddressFromNewLeases(t *testing.T) {
	ts := newTestRegistry(t)
	a := ts.grant(t, "a", "key-a")
	path := snapshotPath(t)
	if err := ts.Save(path); err != nil {
		t.Fatal(err)
	}
	next := ts.restart(t)
	restore(t, next, path)
	delete(next.wg.Stats, "key-a") // the address must stay taken by the lease, not only by the peer

	ip, err := next.Reserve("key-new", snapshotInvite, 0)
	if err != nil || ip == a.IP {
		t.Errorf("Reserve() = %q, %v; want an address other than %s", ip, err, a.IP)
	}
}

func TestRestoreWithoutFileAdoptsNothing(t *testing.T) {
	ts := newTestRegistry(t)
	if ids := restore(t, ts, snapshotPath(t)); len(ids) != 0 {
		t.Errorf("restored ids = %v, want none", ids)
	}
}

func TestRestoreDropsLeasesItCantTrust(t *testing.T) {
	cases := []struct {
		name  string
		after func(*testRegistry) // runs on the new registry before Restore
		clock time.Duration       // how far the new process's clock is ahead
	}{
		{name: "expired", clock: TTL + time.Second},
		{name: "peer gone from the interface", after: func(ts *testRegistry) { delete(ts.wg.Stats, "key-a") }},
		{name: "peer moved to another address", after: func(ts *testRegistry) {
			ts.wg.Stats["key-a"] = peerAt("key-a", "10.9.0.99")
		}},
		{name: "peer holding more than the lease's address", after: func(ts *testRegistry) {
			p := ts.wg.Stats["key-a"]
			p.AllowedIPs = append(p.AllowedIPs, prefixes("10.9.0.99/32")...)
			ts.wg.Stats["key-a"] = p
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ts := newTestRegistry(t)
			ts.grant(t, "a", "key-a")
			ts.grant(t, "b", "key-b")
			path := snapshotPath(t)
			if err := ts.Save(path); err != nil {
				t.Fatal(err)
			}

			next := ts.restart(t)
			next.wg.Stats = make(map[string]wireguard.PeerStat)
			for k, v := range ts.wg.Stats {
				next.wg.Stats[k] = v
			}
			if c.after != nil {
				c.after(next)
			}
			later := ts.now().Add(c.clock)
			next.now = func() time.Time { return later }
			captureLog(t)

			ids := restore(t, next, path)
			want := []string{"b"}
			if c.clock > 0 {
				want = nil
			}
			if !slices.Equal(ids, want) {
				t.Errorf("restored ids = %v, want %v", ids, want)
			}
		})
	}
}

func TestRestoreDropsUntrustworthyFields(t *testing.T) {
	ts := newTestRegistry(t)
	now := ts.now()
	// Written by hand, as a tampered or corrupted file would be.
	ts.wg.Stats = map[string]wireguard.PeerStat{
		"key-far":   peerAt("key-far", "10.9.0.10"),
		"key-range": peerAt("key-range", "10.9.0.5"),
		"key-dup":   peerAt("key-dup", "10.9.0.12"),
	}
	path := snapshotPath(t)
	saved := &Registry{leases: map[string]*Lease{
		"far":   {PubKey: "key-far", IP: "10.9.0.10", ExpiresAt: now.Add(TTL + time.Hour)},
		"range": {PubKey: "key-range", IP: "10.9.0.5", ExpiresAt: now.Add(TTL)},
		"bad":   {PubKey: "key-bad", IP: "not-an-ip", ExpiresAt: now.Add(TTL)},
		"dup-1": {PubKey: "key-dup", IP: "10.9.0.12", ExpiresAt: now.Add(TTL)},
		"dup-2": {PubKey: "key-dup", IP: "10.9.0.12", ExpiresAt: now.Add(TTL)},
	}}
	if err := saved.Save(path); err != nil {
		t.Fatal(err)
	}
	captureLog(t)

	ids := restore(t, ts, path)
	if len(ids) != 1 || (ids[0] != "dup-1" && ids[0] != "dup-2") {
		t.Errorf("restored ids = %v, want one of the two leases on 10.9.0.12", ids)
	}
}

func TestRestoreRefusesUnreadableFiles(t *testing.T) {
	cases := map[string]string{
		"corrupted":     "{not json",
		"other version": `{"version": 2, "leases": {}}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			ts := newTestRegistry(t)
			path := snapshotPath(t)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}

			if ids, err := ts.Restore(path); err == nil || len(ids) != 0 {
				t.Errorf("Restore() = %v, %v; want an error and no lease", ids, err)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("the file is still there after a refused Restore (stat error %v)", err)
			}
		})
	}
}

func TestSaveWritesAPrivateFileAndNoLeftovers(t *testing.T) {
	ts := newTestRegistry(t)
	ts.grant(t, "a", "key-a")
	dir := t.TempDir()
	path := filepath.Join(dir, "leases.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ts.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file mode = %o, want 600", perm)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("files in the directory = %d, want only leases.json", len(entries))
	}
}
