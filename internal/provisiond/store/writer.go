package store

import (
	"context"
	"log"
	"slices"
	"sync"
	"time"
)

// MetricsStore takes the metric writes a Writer sends.
type MetricsStore interface {
	AddServerSample(ctx context.Context, at time.Time, downBps, upBps float64) error
	AddUserSamples(ctx context.Context, at time.Time, samples []UserSample) error
	SaveSession(ctx context.Context, s Session) error
	AddDailyUsage(ctx context.Context, u DailyUsage) error
}

// maxPending bounds each kind of queued write, so a long database outage can't exhaust the
// VPS's memory. Beyond it, the oldest write of that kind is dropped.
const maxPending = 2000

// Writer queues metric writes and sends them from its own goroutine, so a slow database
// never holds the caller. A write that fails stays queued, and is retried when the next one
// arrives. A queued session keeps only the latest state of its lease, and queued daily usage
// is summed per identity and day.
type Writer struct {
	store  MetricsStore
	wake   chan struct{}
	stop   chan struct{}
	done   chan struct{}
	ctx    context.Context
	cancel context.CancelFunc

	closeOnce sync.Once

	mu      sync.Mutex
	pending pendingWrites
	dropped int // since the database last caught up
}

type pendingWrites struct {
	samples  []func(context.Context, MetricsStore) error // oldest first
	sessions map[string]Session                          // by lease id
	usage    map[usageKey]DailyUsage
}

type usageKey struct{ identity, day string }

func newPendingWrites() pendingWrites {
	return pendingWrites{sessions: make(map[string]Session), usage: make(map[usageKey]DailyUsage)}
}

func (p pendingWrites) len() int {
	return len(p.samples) + len(p.sessions) + len(p.usage)
}

// NewWriter starts a writer that sends to st until Close.
func NewWriter(st MetricsStore) *Writer {
	ctx, cancel := context.WithCancel(context.Background())
	w := &Writer{
		store:   st,
		wake:    make(chan struct{}, 1),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		ctx:     ctx,
		cancel:  cancel,
		pending: newPendingWrites(),
	}
	go w.run()
	return w
}

func (w *Writer) AddServerSample(at time.Time, downBps, upBps float64) {
	w.addSample(func(ctx context.Context, st MetricsStore) error {
		return st.AddServerSample(ctx, at, downBps, upBps)
	})
}

func (w *Writer) AddUserSamples(at time.Time, samples []UserSample) {
	w.addSample(func(ctx context.Context, st MetricsStore) error {
		return st.AddUserSamples(ctx, at, samples)
	})
}

func (w *Writer) SaveSession(s Session) {
	w.enqueue(func() { w.putSession(s) })
}

func (w *Writer) AddDailyUsage(u DailyUsage) {
	w.enqueue(func() { w.putUsage(u) })
}

// Close sends what is still queued and stops the writer. When ctx ends first, it cancels
// the write in flight and drops the rest.
func (w *Writer) Close(ctx context.Context) {
	w.closeOnce.Do(func() { close(w.stop) })
	select {
	case <-w.done:
	case <-ctx.Done():
		w.cancel()
		<-w.done
	}
	w.cancel()

	w.mu.Lock()
	defer w.mu.Unlock()
	if n := w.pending.len(); n > 0 {
		log.Printf("metrics writer closed with %d writes not sent", n)
	}
}

func (w *Writer) addSample(write func(context.Context, MetricsStore) error) {
	w.enqueue(func() {
		w.pending.samples = append(w.pending.samples, write)
		w.trimSamples()
	})
}

func (w *Writer) enqueue(put func()) {
	w.mu.Lock()
	put()
	w.mu.Unlock()

	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *Writer) run() {
	defer close(w.done)
	for {
		select {
		case <-w.wake:
			w.flush()
		case <-w.stop:
			w.flush()
			return
		}
	}
}

// flush sends every queued write and stops at the first failure: the database is likely
// unreachable, and trying the rest would spend a timeout on each.
func (w *Writer) flush() {
	w.mu.Lock()
	queued := w.pending
	w.pending = newPendingWrites()
	w.mu.Unlock()

	unsent := w.send(queued)

	w.mu.Lock()
	defer w.mu.Unlock()
	w.requeue(unsent)
	if unsent.len() == 0 && w.dropped > 0 {
		log.Printf("the database caught up; %d queued metric writes were dropped meanwhile", w.dropped)
		w.dropped = 0
	}
}

// send returns the writes it didn't send, the failed one included.
func (w *Writer) send(p pendingWrites) pendingWrites {
	for i, write := range p.samples {
		if write(w.ctx, w.store) != nil {
			p.samples = p.samples[i:]
			return p
		}
	}
	p.samples = nil

	for id, s := range p.sessions {
		if w.store.SaveSession(w.ctx, s) != nil {
			return p
		}
		delete(p.sessions, id)
	}

	for key, u := range p.usage {
		if w.store.AddDailyUsage(w.ctx, u) != nil {
			return p
		}
		delete(p.usage, key)
	}
	return p
}

// requeue puts unsent writes back, ahead of the samples queued meanwhile. The caller holds w.mu.
func (w *Writer) requeue(unsent pendingWrites) {
	w.pending.samples = append(unsent.samples, w.pending.samples...)
	w.trimSamples()
	for _, s := range unsent.sessions {
		w.putSession(s)
	}
	for _, u := range unsent.usage {
		w.putUsage(u)
	}
}

// trimSamples drops the oldest samples beyond maxPending. The caller holds w.mu.
func (w *Writer) trimSamples() {
	if excess := len(w.pending.samples) - maxPending; excess > 0 {
		w.pending.samples = slices.Delete(w.pending.samples, 0, excess)
		w.noteDropped("sample", excess)
	}
}

// putSession queues the latest state of the lease, with the earliest start queued for it.
// The caller holds w.mu.
func (w *Writer) putSession(s Session) {
	queued, ok := w.pending.sessions[s.LeaseID]
	if !ok {
		olderSession := func(a, b Session) bool { return a.UpdatedAt.Before(b.UpdatedAt) }
		if makeRoom(w, w.pending.sessions, s, olderSession, "session") {
			w.pending.sessions[s.LeaseID] = s
		}
		return
	}

	if queued.UpdatedAt.After(s.UpdatedAt) {
		s, queued = queued, s
	}
	if queued.StartedAt.Before(s.StartedAt) {
		s.StartedAt = queued.StartedAt
	}
	w.pending.sessions[s.LeaseID] = s
}

// putUsage adds u to the usage queued for its identity and day. The caller holds w.mu.
func (w *Writer) putUsage(u DailyUsage) {
	key := usageKey{u.Identity, u.Day}
	queued, ok := w.pending.usage[key]
	if ok {
		u.RxBytes += queued.RxBytes
		u.TxBytes += queued.TxBytes
		u.OnlineSeconds += queued.OnlineSeconds
	}
	olderUsage := func(a, b DailyUsage) bool { return a.Day < b.Day }
	if ok || makeRoom(w, w.pending.usage, u, olderUsage, "daily usage") {
		w.pending.usage[key] = u
	}
}

// makeRoom keeps m within maxPending before v joins it, by dropping the oldest of v and
// the queued entries. It reports whether v still joins. The caller holds w.mu.
func makeRoom[K comparable, V any](w *Writer, m map[K]V, v V, older func(a, b V) bool, kind string) bool {
	if len(m) < maxPending {
		return true
	}
	w.noteDropped(kind, 1)

	var oldestKey K
	var oldest V
	first := true
	for k, queued := range m {
		if first || older(queued, oldest) {
			oldestKey, oldest, first = k, queued, false
		}
	}
	if older(v, oldest) {
		return false
	}
	delete(m, oldestKey)
	return true
}

// noteDropped logs the first drop until the database catches up. The caller holds w.mu.
func (w *Writer) noteDropped(kind string, n int) {
	if w.dropped == 0 {
		log.Printf("metrics writer queue full: dropping the oldest queued %s until the database catches up", kind)
	}
	w.dropped += n
}
