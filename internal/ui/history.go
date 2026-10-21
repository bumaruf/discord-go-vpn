package ui

import (
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/supervisor"
	"github.com/bumaruf/discord-go-vpn/internal/wg"
)

// sample is one second of the tunnel. A second without a session keeps its
// place in the series, so the chart's time axis stays true.
type sample struct {
	traffic  bool    // the rates below were measured
	down, up float64 // bytes per second
	rtt      time.Duration
}

// maxSamples covers the widest chart window.
const maxSamples = 15 * 60

type history struct {
	samples []sample

	last   wg.Traffic
	lastAt time.Time
}

// add turns a reading's counters into rates. Counters that went back mean a new
// session, whose first second only sets the baseline.
func (h *history) add(r supervisor.Reading, at time.Time) {
	s := sample{rtt: r.RTT}
	if r.HasTraffic {
		t := r.Traffic
		if !h.lastAt.IsZero() && t.RxBytes >= h.last.RxBytes && t.TxBytes >= h.last.TxBytes {
			if dt := at.Sub(h.lastAt).Seconds(); dt > 0 {
				s.traffic = true
				s.down = float64(t.RxBytes-h.last.RxBytes) / dt
				s.up = float64(t.TxBytes-h.last.TxBytes) / dt
			}
		}
		h.last, h.lastAt = t, at
	} else {
		h.lastAt = time.Time{}
	}

	h.samples = append(h.samples, s)
	if len(h.samples) > maxSamples {
		h.samples = h.samples[len(h.samples)-maxSamples:]
	}
}

// window returns the last n seconds, oldest first, padded at the start with
// empty seconds before the first sample.
func (h history) window(n int) []sample {
	out := make([]sample, n)
	src := h.samples[max(0, len(h.samples)-n):]
	copy(out[n-len(src):], src)
	return out
}

func (h history) latest() sample {
	if len(h.samples) == 0 {
		return sample{}
	}
	return h.samples[len(h.samples)-1]
}
