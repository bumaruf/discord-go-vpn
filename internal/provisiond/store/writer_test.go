package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

var writerTestTime = time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)

// fakeMetrics records the writes it receives. While down, every write fails; while gate is
// set, every write first waits for it to close or for its context to end.
type fakeMetrics struct {
	mu       sync.Mutex
	down     bool
	gate     chan struct{}
	samples  []string
	sessions []Session
	usage    []DailyUsage
}

func (f *fakeMetrics) write(ctx context.Context, record func()) error {
	f.mu.Lock()
	gate := f.gate
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return errors.New("server selection error")
	}
	record()
	return nil
}

func (f *fakeMetrics) AddServerSample(ctx context.Context, _ time.Time, downBps, _ float64) error {
	return f.write(ctx, func() { f.samples = append(f.samples, fmt.Sprintf("server %g", downBps)) })
}

func (f *fakeMetrics) AddUserSamples(ctx context.Context, _ time.Time, samples []UserSample) error {
	return f.write(ctx, func() { f.samples = append(f.samples, fmt.Sprintf("users %d", len(samples))) })
}

func (f *fakeMetrics) SaveSession(ctx context.Context, s Session) error {
	return f.write(ctx, func() { f.sessions = append(f.sessions, s) })
}

func (f *fakeMetrics) AddDailyUsage(ctx context.Context, u DailyUsage) error {
	return f.write(ctx, func() { f.usage = append(f.usage, u) })
}

func (f *fakeMetrics) setDown(down bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.down = down
}

// hold makes the next writes wait; the returned function fails them if down is set, and
// lets later writes through.
func (f *fakeMetrics) hold() (release func(down bool)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	gate := make(chan struct{})
	f.gate = gate
	return func(down bool) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.down = down
		f.gate = nil
		close(gate)
	}
}

func captureWriterLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

func TestWriterSendsQueuedWrites(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		st := &fakeMetrics{}
		w := NewWriter(st)
		defer w.Close(context.Background())

		w.AddServerSample(writerTestTime, 1, 2)
		w.AddUserSamples(writerTestTime, []UserSample{{Identity: "a"}, {Identity: "b"}})
		w.SaveSession(Session{LeaseID: "lease", UpdatedAt: writerTestTime})
		w.AddDailyUsage(DailyUsage{Identity: "a", Day: "2026-09-12", RxBytes: 10})
		synctest.Wait()

		st.mu.Lock()
		defer st.mu.Unlock()
		if want := []string{"server 1", "users 2"}; !slices.Equal(st.samples, want) {
			t.Errorf("samples = %q, want %q", st.samples, want)
		}
		if len(st.sessions) != 1 || len(st.usage) != 1 {
			t.Errorf("sessions %v, usage %v; want one of each", st.sessions, st.usage)
		}
	})
}

func TestWriterRetriesFailedWritesInOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		st := &fakeMetrics{down: true}
		w := NewWriter(st)
		defer w.Close(context.Background())

		w.AddServerSample(writerTestTime, 1, 0)
		w.AddServerSample(writerTestTime, 2, 0)
		synctest.Wait()

		st.setDown(false)
		w.AddServerSample(writerTestTime, 3, 0)
		synctest.Wait()

		st.mu.Lock()
		defer st.mu.Unlock()
		if want := []string{"server 1", "server 2", "server 3"}; !slices.Equal(st.samples, want) {
			t.Errorf("samples = %q, want %q", st.samples, want)
		}
	})
}

func TestWriterKeepsTheLatestSessionOfEachLease(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		st := &fakeMetrics{}
		w := NewWriter(st)
		defer w.Close(context.Background())

		session := func(leaseID string, rxBytes uint64, at time.Duration) Session {
			when := writerTestTime.Add(at)
			return Session{LeaseID: leaseID, RxBytes: rxBytes, StartedAt: when, UpdatedAt: when}
		}

		release := st.hold()
		w.SaveSession(session("a", 1, 0))
		synctest.Wait() // in flight

		w.SaveSession(session("a", 2, 30*time.Second))
		w.SaveSession(session("a", 3, 60*time.Second))
		w.SaveSession(session("b", 9, 0))
		release(true) // the write in flight fails
		synctest.Wait()

		st.setDown(false)
		w.AddServerSample(writerTestTime, 0, 0)
		synctest.Wait()

		st.mu.Lock()
		defer st.mu.Unlock()
		got := map[string]Session{}
		for _, s := range st.sessions {
			if _, twice := got[s.LeaseID]; twice {
				t.Errorf("lease %s written twice: %v", s.LeaseID, st.sessions)
			}
			got[s.LeaseID] = s
		}
		want := map[string]Session{
			// The latest state, with the start of the first write that failed.
			"a": {LeaseID: "a", RxBytes: 3, StartedAt: writerTestTime, UpdatedAt: writerTestTime.Add(60 * time.Second)},
			"b": session("b", 9, 0),
		}
		if len(got) != len(want) || got["a"] != want["a"] || got["b"] != want["b"] {
			t.Errorf("sessions = %+v, want %+v", got, want)
		}
	})
}

func TestWriterSumsDailyUsage(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		st := &fakeMetrics{}
		w := NewWriter(st)
		defer w.Close(context.Background())

		release := st.hold()
		w.AddDailyUsage(DailyUsage{Identity: "a", Day: "2026-09-12", RxBytes: 100, TxBytes: 10, OnlineSeconds: 30})
		synctest.Wait() // in flight

		w.AddDailyUsage(DailyUsage{Identity: "a", Day: "2026-09-12", RxBytes: 20, TxBytes: 2, OnlineSeconds: 30})
		w.AddDailyUsage(DailyUsage{Identity: "a", Day: "2026-09-12", RxBytes: 3, TxBytes: 1, OnlineSeconds: 30})
		w.AddDailyUsage(DailyUsage{Identity: "a", Day: "2026-09-13", RxBytes: 5})
		release(true) // the write in flight fails
		synctest.Wait()

		st.setDown(false)
		w.AddServerSample(writerTestTime, 0, 0)
		synctest.Wait()

		st.mu.Lock()
		defer st.mu.Unlock()
		want := map[string]DailyUsage{
			"2026-09-12": {Identity: "a", Day: "2026-09-12", RxBytes: 123, TxBytes: 13, OnlineSeconds: 90},
			"2026-09-13": {Identity: "a", Day: "2026-09-13", RxBytes: 5},
		}
		if len(st.usage) != len(want) {
			t.Fatalf("usage = %v, want one write per day: %v", st.usage, want)
		}
		for _, u := range st.usage {
			if u != want[u.Day] {
				t.Errorf("usage for %s = %+v, want %+v", u.Day, u, want[u.Day])
			}
		}
	})
}

func TestWriterDropsTheOldestBeyondTheLimit(t *testing.T) {
	logs := captureWriterLog(t)
	synctest.Test(t, func(t *testing.T) {
		st := &fakeMetrics{down: true}
		w := NewWriter(st)

		for i := range maxPending + 1 {
			at := writerTestTime.Add(time.Duration(i) * time.Second)
			w.AddServerSample(at, float64(i), 0)
			// Enqueued newest first, so the oldest is the last to arrive.
			w.SaveSession(Session{LeaseID: fmt.Sprint(i), UpdatedAt: writerTestTime.Add(-time.Duration(i) * time.Second)})
			w.AddDailyUsage(DailyUsage{Identity: "a", Day: writerTestTime.AddDate(0, 0, i).Format(time.DateOnly), RxBytes: 1})
		}
		synctest.Wait()

		st.setDown(false)
		w.Close(context.Background())

		if len(st.samples) != maxPending || st.samples[0] != "server 1" {
			t.Errorf("%d samples starting at %q, want %d starting at server 1", len(st.samples), st.samples[0], maxPending)
		}
		oldestSession := fmt.Sprint(maxPending)
		if len(st.sessions) != maxPending || slices.ContainsFunc(st.sessions, func(s Session) bool { return s.LeaseID == oldestSession }) {
			t.Errorf("%d sessions, want %d without the oldest (lease %s)", len(st.sessions), maxPending, oldestSession)
		}
		oldestDay := writerTestTime.Format(time.DateOnly)
		if len(st.usage) != maxPending || slices.ContainsFunc(st.usage, func(u DailyUsage) bool { return u.Day == oldestDay }) {
			t.Errorf("%d daily usage writes, want %d without the oldest day (%s)", len(st.usage), maxPending, oldestDay)
		}
	})

	for _, want := range []string{"dropping the oldest queued", "the database caught up; 3 queued metric writes were dropped"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log %q doesn't mention %q", logs.String(), want)
		}
	}
}

func TestWriterCloseSendsWhatIsQueued(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		st := &fakeMetrics{}
		release := st.hold()
		w := NewWriter(st)
		w.AddServerSample(writerTestTime, 1, 0)
		synctest.Wait() // in flight
		w.SaveSession(Session{LeaseID: "a", UpdatedAt: writerTestTime})
		w.AddDailyUsage(DailyUsage{Identity: "a", Day: "2026-09-12", RxBytes: 1})

		closed := make(chan struct{})
		go func() {
			defer close(closed)
			w.Close(context.Background())
		}()
		release(false)
		<-closed

		if len(st.samples) != 1 || len(st.sessions) != 1 || len(st.usage) != 1 {
			t.Errorf("samples %q, sessions %v, usage %v; want every queued write sent", st.samples, st.sessions, st.usage)
		}
	})
}

func TestWriterCloseGivesUpAtTheDeadline(t *testing.T) {
	logs := captureWriterLog(t)
	synctest.Test(t, func(t *testing.T) {
		st := &fakeMetrics{}
		st.hold() // never released: the database stopped answering
		w := NewWriter(st)
		w.AddServerSample(writerTestTime, 1, 0)
		w.SaveSession(Session{LeaseID: "a", UpdatedAt: writerTestTime})
		synctest.Wait()

		const deadline = 5 * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), deadline)
		defer cancel()
		start := time.Now()
		w.Close(ctx)

		if elapsed := time.Since(start); elapsed != deadline {
			t.Errorf("Close took %v, want the %v deadline", elapsed, deadline)
		}
		if len(st.samples) != 0 || len(st.sessions) != 0 {
			t.Errorf("samples %q, sessions %v; want nothing sent", st.samples, st.sessions)
		}
	})

	if want := "metrics writer closed with 2 writes not sent"; !strings.Contains(logs.String(), want) {
		t.Errorf("log %q doesn't mention %q", logs.String(), want)
	}
}
