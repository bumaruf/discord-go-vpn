package httpapi

import (
	"sync"
	"time"
)

const (
	maxFailures   = 20
	failureWindow = time.Minute
)

// failureLimiter counts, per address, the tokens refused within a fixed window. Guessing
// invites then costs at most maxFailures database lookups a minute per address, and the
// counts are dropped every window, so the map can't grow past one window of addresses.
// The zero value is ready to use.
type failureLimiter struct {
	mu          sync.Mutex
	windowStart time.Time
	failures    map[string]int
}

func (l *failureLimiter) blocked(addr string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.rollWindow(now)
	return l.failures[addr] >= maxFailures
}

func (l *failureLimiter) fail(addr string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.rollWindow(now)
	l.failures[addr]++
}

// rollWindow starts a new window once the current one is over. The caller holds l.mu.
func (l *failureLimiter) rollWindow(now time.Time) {
	if l.failures == nil || now.Sub(l.windowStart) >= failureWindow {
		l.failures = make(map[string]int)
		l.windowStart = now
	}
}
