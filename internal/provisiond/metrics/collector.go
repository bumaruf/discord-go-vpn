// Package metrics reads the WireGuard interface every second: each tick feeds the live
// stream, and every few ticks a write is queued for the database.
package metrics

import (
	"context"
	"log"
	"slices"
	"sync"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/lease"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/store"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/wireguard"
)

const (
	tickInterval = 1 * time.Second

	serverSampleTicks = 5
	userSampleTicks   = 30
	sessionTicks      = 30
)

// peerMeasurement is a peer at one tick. Down and Up are from the client's side: the
// interface's rx is the client's upload.
type peerMeasurement struct {
	LeaseID   string    `json:"id"`
	InviteID  string    `json:"invite_id,omitempty"`
	DeviceID  string    `json:"device_id,omitempty"`
	Label     string    `json:"label"`
	OSUser    string    `json:"os_user,omitempty"`
	PubKey    string    `json:"key"`
	IP        string    `json:"ip"`
	Origin    string    `json:"origin"`
	OriginIP  string    `json:"origin_ip,omitempty"`
	DownBps   float64   `json:"down_bps"`
	UpBps     float64   `json:"up_bps"`
	RxBytes   uint64    `json:"rx_bytes"`
	TxBytes   uint64    `json:"tx_bytes"`
	Handshake time.Time `json:"last_handshake,omitzero"`
	Online    bool      `json:"online"`
}

// Snapshot is the whole state at one tick, as the live stream sends it.
type Snapshot struct {
	GeneratedAt time.Time         `json:"generated_at"`
	DownBps     float64           `json:"down_bps"`
	UpBps       float64           `json:"up_bps"`
	Online      int               `json:"online"`
	Active      int               `json:"active"`
	Limit       int               `json:"limit"`
	Peers       []peerMeasurement `json:"peers"`
}

// accumulator keeps what spans more than one tick for a lease.
type accumulator struct {
	prevRx uint64
	prevTx uint64

	sessionRx uint64
	sessionTx uint64

	// Since the last write handed to the store writer.
	unsavedRx            uint64
	unsavedTx            uint64
	unsavedOnlineSeconds int
}

type Collector struct {
	wg      wireguard.Interface
	leases  *lease.Registry
	writer  *store.Writer // nil without a database
	now     func() time.Time
	publish func(Snapshot)

	mu           sync.RWMutex
	snapshot     Snapshot
	accumulators map[string]*accumulator // by lease id

	// resumed holds the lease ids a restart restored; set before Run, read by the tick only.
	resumed map[string]bool

	tick int
}

func NewCollector(wg wireguard.Interface, leases *lease.Registry, writer *store.Writer, now func() time.Time, publish func(Snapshot)) *Collector {
	return &Collector{
		wg:           wg,
		leases:       leases,
		writer:       writer,
		now:          now,
		publish:      publish,
		accumulators: make(map[string]*accumulator),
	}
}

// Resume makes the first measurement of each lease in ids start from the peer's counters
// rather than from zero. Those leases were restored after a restart: their peers kept counting,
// and the previous process already saved their usage, so counting from zero would add it again
// and draw a spike. What the peers counted while no process was running goes uncounted.
// Call it before Run.
func (c *Collector) Resume(ids []string) {
	c.resumed = make(map[string]bool, len(ids))
	for _, id := range ids {
		c.resumed[id] = true
	}
}

// Current returns the latest snapshot.
func (c *Collector) Current() Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.snapshot
}

func (c *Collector) Run(ctx context.Context) {
	t := time.NewTicker(tickInterval)
	defer t.Stop()

	last := time.Now()

	for {
		select {
		case <-ctx.Done():
			c.saveOnStop()
			return
		case now := <-t.C:
			elapsed := now.Sub(last).Seconds()
			last = now

			// After a missed tick (the VPS stalled, the process was suspended), dividing many
			// seconds of bytes by one would draw a false spike, so the rates are dropped.
			if elapsed <= 0 || elapsed > 5 {
				c.measure(0)
				continue
			}
			c.measure(elapsed)
		}
	}
}

func (c *Collector) measure(elapsed float64) {
	peers, err := c.wg.Dump()
	if err != nil {
		log.Printf("collector: reading the interface: %v", err)
		return
	}

	now := c.now()

	c.leases.ExpireStale()
	leases := c.leases.Leases()

	measurements := make([]peerMeasurement, 0, len(leases))
	alive := make(map[string]bool, len(leases))
	var totalDown, totalUp float64
	online := 0

	for id, l := range leases {
		alive[id] = true
		p := peers[l.PubKey]

		a := c.accumulators[id]
		if a == nil {
			a = &accumulator{}
			if c.resumed[id] {
				delete(c.resumed, id)
				a.prevRx, a.prevTx = p.RxBytes, p.TxBytes
				a.sessionRx, a.sessionTx = p.RxBytes, p.TxBytes
			}
			c.accumulators[id] = a
		}

		deltaRx := delta(p.RxBytes, a.prevRx)
		deltaTx := delta(p.TxBytes, a.prevTx)
		a.prevRx, a.prevTx = p.RxBytes, p.TxBytes

		a.sessionRx += deltaRx
		a.sessionTx += deltaTx
		a.unsavedRx += deltaRx
		a.unsavedTx += deltaTx

		isOnline := recentHandshake(p.Handshake, now)
		if isOnline {
			online++
			a.unsavedOnlineSeconds++
		}

		var downBps, upBps float64
		if elapsed > 0 {
			downBps = float64(deltaTx) * 8 / elapsed
			upBps = float64(deltaRx) * 8 / elapsed
		}
		totalDown += downBps
		totalUp += upBps

		m := peerMeasurement{
			LeaseID:  id,
			InviteID: l.InviteID,
			DeviceID: l.DeviceID,
			Label:    l.Label,
			OSUser:   l.OSUser,
			PubKey:   l.PubKey,
			IP:       l.IP,
			Origin:   maskOrigin(p.Endpoint),
			OriginIP: endpointHost(p.Endpoint),
			DownBps:  downBps,
			UpBps:    upBps,
			RxBytes:  a.sessionRx,
			TxBytes:  a.sessionTx,
			Online:   isOnline,
		}
		if !p.Handshake.IsZero() {
			m.Handshake = p.Handshake
		}
		measurements = append(measurements, m)
	}

	active := len(leases)
	limit := c.leases.MaxActive()

	for id := range c.accumulators {
		if !alive[id] {
			delete(c.accumulators, id)
		}
	}

	slices.SortFunc(measurements, func(a, b peerMeasurement) int { return compareIP(a.IP, b.IP) })

	snapshot := Snapshot{
		GeneratedAt: now,
		DownBps:     totalDown,
		UpBps:       totalUp,
		Online:      online,
		Active:      active,
		Limit:       limit,
		Peers:       measurements,
	}

	c.mu.Lock()
	c.snapshot = snapshot
	c.mu.Unlock()

	c.publish(snapshot)

	c.tick++
	c.enqueueWrites(now, measurements, totalDown, totalUp)
}

// delta treats a counter lower than the previous reading as a peer recreated from zero,
// which happens on every new lease.
func delta(current, previous uint64) uint64 {
	if current < previous {
		return current
	}
	return current - previous
}

func (c *Collector) enqueueWrites(now time.Time, measurements []peerMeasurement, down, up float64) {
	if c.writer == nil {
		return
	}

	if c.tick%serverSampleTicks == 0 {
		c.writer.AddServerSample(now, down, up)
	}

	if c.tick%userSampleTicks == 0 && len(measurements) > 0 {
		samples := make([]store.UserSample, 0, len(measurements))
		for _, m := range measurements {
			samples = append(samples, store.UserSample{Identity: identity(m), DownBps: m.DownBps, UpBps: m.UpBps})
		}
		c.writer.AddUserSamples(now, samples)
	}

	if c.tick%sessionTicks == 0 {
		c.saveSessions(now, measurements)
	}
}

// saveOnStop queues the sessions and the usage measured since the last save, which would
// otherwise wait for the next sessionTicks tick.
func (c *Collector) saveOnStop() {
	if c.writer == nil {
		return
	}
	c.saveSessions(c.now(), c.Current().Peers)
}

func (c *Collector) saveSessions(now time.Time, measurements []peerMeasurement) {
	day := now.UTC().Format("2006-01-02")

	for _, m := range measurements {
		c.writer.SaveSession(store.Session{
			LeaseID:       m.LeaseID,
			InviteID:      m.InviteID,
			DeviceID:      m.DeviceID,
			Label:         m.Label,
			PublicKey:     m.PubKey,
			IP:            m.IP,
			Origin:        m.Origin,
			RxBytes:       m.RxBytes,
			TxBytes:       m.TxBytes,
			Online:        m.Online,
			LastHandshake: m.Handshake,
			UpdatedAt:     now,
			StartedAt:     now,
		})

		c.addDailyUsage(m, day)
	}
}

func (c *Collector) addDailyUsage(m peerMeasurement, day string) {
	a := c.accumulators[m.LeaseID]
	if a == nil || (a.unsavedRx == 0 && a.unsavedTx == 0 && a.unsavedOnlineSeconds == 0) {
		return
	}

	c.writer.AddDailyUsage(store.DailyUsage{
		Identity:      identity(m),
		Day:           day,
		RxBytes:       a.unsavedRx,
		TxBytes:       a.unsavedTx,
		OnlineSeconds: a.unsavedOnlineSeconds,
	})
	// The writer keeps the usage queued until the $inc lands.
	a.unsavedRx, a.unsavedTx, a.unsavedOnlineSeconds = 0, 0, 0
}

// identity groups usage by device, which outlives its leases. A lease whose device the
// database couldn't register has none, so its usage is grouped by public key.
func identity(m peerMeasurement) string {
	if m.DeviceID != "" {
		return m.DeviceID
	}
	return "peer:" + wireguard.ShortKey(m.PubKey)
}
