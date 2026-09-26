package metrics

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/config"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/lease"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/store"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/wireguard"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/wireguard/wireguardtest"
)

func TestDelta(t *testing.T) {
	cases := []struct {
		name     string
		current  uint64
		previous uint64
		want     uint64
	}{
		{"normal count", 1500, 1000, 500},
		{"no traffic between ticks", 1000, 1000, 0},
		{"first reading", 800, 0, 800},
		// A plain subtraction would wrap around to 18446744073709551516.
		{"peer recreated, counter reset", 100, 5000, 100},
		{"peer recreated with no traffic yet", 0, 5000, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := delta(c.current, c.previous); got != c.want {
				t.Errorf("delta(%d, %d) = %d, want %d", c.current, c.previous, got, c.want)
			}
		})
	}
}

func TestIdentity(t *testing.T) {
	withDevice := peerMeasurement{DeviceID: "65f1a2b3c4d5e6f7a8b9c0d1", PubKey: "abcdefghijklmnop"}
	if got := identity(withDevice); got != "65f1a2b3c4d5e6f7a8b9c0d1" {
		t.Errorf("with a device, identity = %q, want the device id", got)
	}

	withoutDevice := peerMeasurement{PubKey: "abcdefghijklmnop"}
	if got := identity(withoutDevice); got != "peer:abcdefghij…" {
		t.Errorf("without a device, identity = %q, want the shortened key", got)
	}
}

// tickBudget bounds a collector tick while another operation's wg call is held.
const tickBudget = 2 * time.Second

func newTestCollector(wg *wireguardtest.Fake, registry *lease.Registry) *Collector {
	return NewCollector(wg, registry, nil, time.Now, func(Snapshot) {})
}

// grantLease reserves an address for publicKey and stores its lease under id.
func grantLease(t *testing.T, registry *lease.Registry, id, publicKey, inviteID string) {
	t.Helper()
	ip, err := registry.Reserve(publicKey, inviteID, 0)
	if err != nil {
		t.Errorf("reserving %s: %v", id, err)
		return
	}
	registry.Grant(id, lease.Lease{PubKey: publicKey, IP: ip, InviteID: inviteID})
}

func TestCollectorTickDoesntWaitOnWireguard(t *testing.T) {
	const heldKey, inviteID = "held-key", "65f1a2b3c4d5e6f7a8b9c0d1"

	// Each blocker prepares a registry operation whose wg call is on heldKey.
	blockers := []struct {
		name  string
		setup func(t *testing.T, registry *lease.Registry, clock *time.Time) func()
	}{
		{"adding a peer", func(_ *testing.T, registry *lease.Registry, _ *time.Time) func() {
			return func() { _, _ = registry.Reserve(heldKey, "", 0) }
		}},
		{"releasing a lease", func(t *testing.T, registry *lease.Registry, _ *time.Time) func() {
			grantLease(t, registry, "held", heldKey, "")
			return func() { registry.Release("held", "") }
		}},
		{"disconnecting a lease", func(t *testing.T, registry *lease.Registry, _ *time.Time) func() {
			grantLease(t, registry, "held", heldKey, "")
			return func() { registry.Disconnect("held") }
		}},
		{"disconnecting an invite", func(t *testing.T, registry *lease.Registry, _ *time.Time) func() {
			grantLease(t, registry, "held", heldKey, inviteID)
			return func() { registry.DisconnectInvite(inviteID) }
		}},
		{"expiring a lease", func(t *testing.T, registry *lease.Registry, clock *time.Time) func() {
			// Granted one TTL and a second ago, so it expired a second ago.
			*clock = clock.Add(-lease.TTL - time.Second)
			grantLease(t, registry, "held", heldKey, "")
			*clock = clock.Add(lease.TTL + time.Second)
			return registry.ExpireStale
		}},
	}

	for _, b := range blockers {
		t.Run("the tick proceeds while "+b.name+" is held", func(t *testing.T) {
			wg := wireguardtest.NewFake()
			clock := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
			registry := lease.NewRegistry(wg, config.DefaultWGSubnet, func() time.Time { return clock }, 50)
			grantLease(t, registry, "other", "other-key", "")
			blocked := b.setup(t, registry, &clock)
			c := newTestCollector(wg, registry)

			entered, release := wg.Hold(t, heldKey)
			blockedDone := make(chan struct{})
			go func() {
				defer close(blockedDone)
				blocked()
			}()
			select {
			case <-entered:
			case <-time.After(tickBudget):
				t.Fatal("the blocker never reached wg")
			}

			ticked := make(chan struct{})
			go func() {
				defer close(ticked)
				c.measure(1)
			}()
			stalled := false
			select {
			case <-ticked:
			case <-time.After(tickBudget):
				stalled = true
			}
			release()
			<-blockedDone
			<-ticked

			if stalled {
				t.Fatalf("the collector tick stalled while %s was held", b.name)
			}
		})
	}
}

// Meant for -race: leases come and go while the collector ticks.
func TestCollectorTicksWhileLeasesChange(t *testing.T) {
	const clients, rounds = 20, 10
	wg := wireguardtest.NewFake()
	registry := lease.NewRegistry(wg, config.DefaultWGSubnet, time.Now, 50)
	c := newTestCollector(wg, registry)

	stop := make(chan struct{})
	ticking := make(chan struct{})
	go func() {
		defer close(ticking)
		for {
			select {
			case <-stop:
				return
			default:
				c.measure(1)
			}
		}
	}()

	var clientsDone sync.WaitGroup
	for n := range clients {
		clientsDone.Go(func() {
			for round := range rounds {
				id := fmt.Sprintf("lease-%d-%d", n, round)
				grantLease(t, registry, id, fmt.Sprintf("key-%d", n), "")
				registry.Renew(id, "")
				registry.Release(id, "")
			}
		})
	}
	clientsDone.Wait()
	close(stop)
	<-ticking

	c.measure(1)
	if len(c.accumulators) != 0 || c.Current().Active != 0 {
		t.Errorf("after every lease ended: %d accumulators, %d active; want none", len(c.accumulators), c.Current().Active)
	}
}

func TestSnapshotCarriesTheLeaseIdentity(t *testing.T) {
	wg := wireguardtest.NewFake()
	registry := lease.NewRegistry(wg, config.DefaultWGSubnet, time.Now, 50)
	registry.Grant("lease", lease.Lease{PubKey: "key", IP: "10.9.0.10", Label: "notebook da ana", OSUser: "ana", InviteID: "invite", DeviceID: "device"})
	c := newTestCollector(wg, registry)

	c.measure(1)
	peers := c.Current().Peers
	if len(peers) != 1 {
		t.Fatalf("peers = %+v, want one", peers)
	}
	p := peers[0]
	got := []string{p.Label, p.OSUser, p.InviteID, p.DeviceID}
	want := []string{"notebook da ana", "ana", "invite", "device"}
	if !slices.Equal(got, want) {
		t.Errorf("label, os user, invite, device = %q, want %q", got, want)
	}
}

// blockingStore holds every metric write until the test ends, like an Atlas that stopped answering.
type blockingStore struct {
	release chan struct{}
	unblock func()
}

func newBlockingStore(t *testing.T) *blockingStore {
	release := make(chan struct{})
	s := &blockingStore{release: release, unblock: sync.OnceFunc(func() { close(release) })}
	t.Cleanup(s.unblock)
	return s
}

func (s *blockingStore) AddServerSample(context.Context, time.Time, float64, float64) error {
	<-s.release
	return nil
}

func (s *blockingStore) AddUserSamples(context.Context, time.Time, []store.UserSample) error {
	<-s.release
	return nil
}

func (s *blockingStore) SaveSession(context.Context, store.Session) error {
	<-s.release
	return nil
}

func (s *blockingStore) AddDailyUsage(context.Context, store.DailyUsage) error {
	<-s.release
	return nil
}

func newCollectorWithStore(t *testing.T, wg *wireguardtest.Fake, registry *lease.Registry, st *blockingStore) *Collector {
	t.Helper()
	writer := store.NewWriter(st)
	t.Cleanup(func() {
		st.unblock()
		writer.Close(context.Background())
	})
	return NewCollector(wg, registry, writer, time.Now, func(Snapshot) {})
}

func TestSlowStoreDoesntDelayTheTick(t *testing.T) {
	wg := wireguardtest.NewFake()
	registry := lease.NewRegistry(wg, config.DefaultWGSubnet, time.Now, 50)
	grantLease(t, registry, "lease", "key", "")
	c := newCollectorWithStore(t, wg, registry, newBlockingStore(t))

	// sessionTicks ticks reach every kind of write.
	for tick := 1; tick <= sessionTicks; tick++ {
		ticked := make(chan struct{})
		go func() {
			defer close(ticked)
			c.measure(1)
		}()
		select {
		case <-ticked:
		case <-time.After(tickBudget):
			t.Fatalf("tick %d waited on the store, so the next SSE snapshot would be late", tick)
		}
	}
}

// recordingStore keeps every session and daily usage it is sent.
type recordingStore struct {
	mu       sync.Mutex
	sessions []store.Session
	usage    []store.DailyUsage
}

func (s *recordingStore) AddServerSample(context.Context, time.Time, float64, float64) error {
	return nil
}

func (s *recordingStore) AddUserSamples(context.Context, time.Time, []store.UserSample) error {
	return nil
}

func (s *recordingStore) SaveSession(_ context.Context, session store.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions = append(s.sessions, session)
	return nil
}

func (s *recordingStore) AddDailyUsage(_ context.Context, u store.DailyUsage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usage = append(s.usage, u)
	return nil
}

func TestCollectorQueuesUnsavedUsageWhenItStops(t *testing.T) {
	wg := wireguardtest.NewFake()
	registry := lease.NewRegistry(wg, config.DefaultWGSubnet, time.Now, 50)
	grantLease(t, registry, "lease", "key", "")
	wg.Stats = map[string]wireguard.PeerStat{"key": {PubKey: "key", RxBytes: 1000, TxBytes: 2000}}

	st := &recordingStore{}
	writer := store.NewWriter(st)
	c := NewCollector(wg, registry, writer, time.Now, func(Snapshot) {})

	// One tick, far from the next save.
	c.measure(1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Run(ctx)
	writer.Close(context.Background())

	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.sessions) != 1 || st.sessions[0].LeaseID != "lease" {
		t.Errorf("sessions sent = %+v, want the lease's", st.sessions)
	}
	if len(st.usage) != 1 || st.usage[0].RxBytes != 1000 || st.usage[0].TxBytes != 2000 {
		t.Errorf("daily usage sent = %+v, want 1000 rx and 2000 tx", st.usage)
	}
}

func TestResumedLeaseStartsFromThePeerCounters(t *testing.T) {
	wg := wireguardtest.NewFake()
	registry := lease.NewRegistry(wg, config.DefaultWGSubnet, time.Now, 50)
	grantLease(t, registry, "restored", "restored-key", "")
	grantLease(t, registry, "new", "new-key", "")
	// Counted before the restart, by a process that already saved it.
	wg.Stats = map[string]wireguard.PeerStat{
		"restored-key": {PubKey: "restored-key", RxBytes: 5_000_000, TxBytes: 9_000_000},
		"new-key":      {PubKey: "new-key", RxBytes: 100, TxBytes: 200},
	}

	st := &recordingStore{}
	writer := store.NewWriter(st)
	c := NewCollector(wg, registry, writer, time.Now, func(Snapshot) {})
	c.Resume([]string{"restored"})

	c.measure(1)
	if c.Current().DownBps != 1600 {
		t.Errorf("first tick total down = %v bps, want only the new lease's 200 bytes, with no spike", c.Current().DownBps)
	}
	wg.Stats["restored-key"] = wireguard.PeerStat{PubKey: "restored-key", RxBytes: 5_000_010, TxBytes: 9_000_020}
	c.measure(1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Run(ctx)
	writer.Close(context.Background())

	var restored peerMeasurement
	for _, p := range c.Current().Peers {
		if p.LeaseID == "restored" {
			restored = p
		}
	}
	if restored.RxBytes != 5_000_010 || restored.TxBytes != 9_000_020 {
		t.Errorf("restored session bytes = %d rx, %d tx; want the peer's whole count", restored.RxBytes, restored.TxBytes)
	}
	if restored.DownBps != 160 || restored.UpBps != 80 {
		t.Errorf("restored rates = %v down, %v up; want only the last second's bytes", restored.DownBps, restored.UpBps)
	}

	st.mu.Lock()
	defer st.mu.Unlock()
	usage := make(map[string]store.DailyUsage, len(st.usage))
	for _, u := range st.usage {
		usage[u.Identity] = u
	}
	if u := usage["peer:"+wireguard.ShortKey("restored-key")]; u.RxBytes != 10 || u.TxBytes != 20 {
		t.Errorf("restored daily usage = %+v, want only the 10 rx and 20 tx counted after the restart", u)
	}
	if u := usage["peer:"+wireguard.ShortKey("new-key")]; u.RxBytes != 100 || u.TxBytes != 200 {
		t.Errorf("new lease daily usage = %+v, want its 100 rx and 200 tx from zero", u)
	}
}
