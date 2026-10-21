package supervisor

import (
	"context"
	"net"
	"net/url"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/transport"
	"github.com/bumaruf/discord-go-vpn/internal/wg"
)

// Reading is the live tunnel's numbers. The screen polls it instead of getting
// it through Updates, which would wake every other reader once a second.
type Reading struct {
	Traffic wg.Traffic
	// Metered says the session's transport counts traffic, which WireSock
	// doesn't; HasTraffic says this reading got the counters.
	Metered    bool
	HasTraffic bool
	// RTT is the latest TCP connect time to provisiond; zero when unknown, as
	// with a .conf profile, whose endpoint only speaks UDP.
	RTT time.Duration
}

const (
	rttEvery   = 5 * time.Second
	rttTimeout = 3 * time.Second
)

func (s *Supervisor) Reading() Reading {
	s.meterMu.Lock()
	defer s.meterMu.Unlock()

	r := Reading{RTT: s.rtt, Metered: s.meter != nil}
	if s.meter != nil {
		if t, err := s.meter.Traffic(); err == nil {
			r.Traffic, r.HasTraffic = t, true
		}
	}
	return r
}

// useMeter points Reading at tr, or at nothing. It must be cleared before the
// transport closes: holding meterMu, no Traffic call can race the Close.
func (s *Supervisor) useMeter(tr transport.Transport) {
	m, _ := tr.(transport.Meter)
	s.meterMu.Lock()
	s.meter = m
	if m == nil {
		s.rtt = 0
	}
	s.meterMu.Unlock()
}

// measureRTT times a TCP connect to provisiond until ctx ends. It runs from the
// host network, the same path the tunnel's packets take.
func (s *Supervisor) measureRTT(ctx context.Context) {
	server := s.currentProvision().Server
	if !s.leasing() {
		return
	}
	u, err := url.Parse(server)
	if err != nil || u.Host == "" {
		return
	}
	addr := u.Host
	if u.Port() == "" {
		addr = net.JoinHostPort(u.Hostname(), "443")
	}

	every := time.NewTicker(rttEvery)
	defer every.Stop()
	for {
		rtt := dialTime(ctx, addr)
		s.meterMu.Lock()
		s.rtt = rtt
		s.meterMu.Unlock()

		select {
		case <-ctx.Done():
			return
		case <-every.C:
		}
	}
}

// dialTime returns zero when the connect fails, which the screen shows as unknown.
func dialTime(ctx context.Context, addr string) time.Duration {
	ctx, cancel := context.WithTimeout(ctx, rttTimeout)
	defer cancel()

	start := time.Now()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return 0
	}
	elapsed := time.Since(start)
	_ = conn.Close()
	return elapsed
}
