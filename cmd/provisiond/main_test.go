package main

import (
	"context"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/store"
)

// slowStore takes a while to write each server sample, and counts them.
type slowStore struct {
	mu      sync.Mutex
	samples int
}

func (s *slowStore) AddServerSample(context.Context, time.Time, float64, float64) error {
	time.Sleep(50 * time.Millisecond)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.samples++
	return nil
}

func (s *slowStore) AddUserSamples(context.Context, time.Time, []store.UserSample) error {
	return nil
}

func (s *slowStore) SaveSession(context.Context, store.Session) error { return nil }

func (s *slowStore) AddDailyUsage(context.Context, store.DailyUsage) error { return nil }

func (s *slowStore) sent() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.samples
}

func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

// runUntilCancelled starts s.run, waits for entered to close, then cancels its context and
// fails the test unless run returns nil within 5s.
func runUntilCancelled(t *testing.T, s *service, entered <-chan struct{}, beforeCancel func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.run(ctx) }()

	<-entered
	beforeCancel()
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run() = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run didn't return within 5s of its context ending")
	}
}

func TestRunShutsDownAndSendsQueuedWritesWhenItsContextEnds(t *testing.T) {
	st := &slowStore{}
	writer := store.NewWriter(st)
	ln := listen(t)
	base := "http://" + ln.Addr().String()

	var handlers sync.WaitGroup
	handlers.Add(2)
	shuttingDown := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/stream", func(http.ResponseWriter, *http.Request) {
		// Like the live stream, it holds its connection until the shutdown ends it.
		handlers.Done()
		<-shuttingDown
	})
	mux.HandleFunc("/lease", func(w http.ResponseWriter, r *http.Request) {
		handlers.Done()
		<-shuttingDown
		time.Sleep(50 * time.Millisecond)
		if r.Context().Err() != nil {
			http.Error(w, "request cancelled", http.StatusServiceUnavailable)
		}
	})
	srv := &http.Server{Handler: mux}
	srv.RegisterOnShutdown(func() { close(shuttingDown) })

	svc := &service{
		http:            srv,
		listen:          func() error { return srv.Serve(ln) },
		shutdownTimeout: 5 * time.Second,
		background: []func(context.Context){func(ctx context.Context) {
			// Like the collector, it queues one last write as it stops.
			<-ctx.Done()
			writer.AddServerSample(time.Now(), 0, 0)
		}},
		writer: writer,
	}

	go func() {
		if resp, err := http.Get(base + "/stream"); err == nil {
			_ = resp.Body.Close()
		}
	}()
	leaseStatus := make(chan int, 1)
	go func() {
		resp, err := http.Get(base + "/lease")
		if err != nil {
			leaseStatus <- 0
			return
		}
		_ = resp.Body.Close()
		leaseStatus <- resp.StatusCode
	}()
	entered := make(chan struct{})
	go func() {
		handlers.Wait()
		close(entered)
	}()

	runUntilCancelled(t, svc, entered, func() {
		for range 3 {
			writer.AddServerSample(time.Now(), 0, 0)
		}
	})

	if got := st.sent(); got != 4 {
		t.Errorf("samples sent when run returned = %d, want 4", got)
	}
	if got := <-leaseStatus; got != http.StatusOK {
		t.Errorf("request in flight during the shutdown: status %d, want 200 with its context still live", got)
	}
}

func TestRunSavesTheLeasesOnceTheBackgroundWorkStops(t *testing.T) {
	ln := listen(t)
	srv := &http.Server{Handler: http.NewServeMux()}

	var mu sync.Mutex
	stopped, savedAfterStop, saves := false, false, 0
	svc := &service{
		http:            srv,
		listen:          func() error { return srv.Serve(ln) },
		shutdownTimeout: time.Second,
		background: []func(context.Context){func(ctx context.Context) {
			// Like the collector, which expires leases on its tick.
			<-ctx.Done()
			time.Sleep(50 * time.Millisecond)
			mu.Lock()
			stopped = true
			mu.Unlock()
		}},
		saveLeases: func() error {
			mu.Lock()
			defer mu.Unlock()
			saves++
			savedAfterStop = stopped
			return nil
		},
	}

	entered := make(chan struct{})
	close(entered)
	runUntilCancelled(t, svc, entered, func() {})

	mu.Lock()
	defer mu.Unlock()
	if saves != 1 || !savedAfterStop {
		t.Errorf("saves = %d, after the background work stopped = %v; want one save after it stopped", saves, savedAfterStop)
	}
}

func TestRunClosesTheConnectionsShutdownCantWaitFor(t *testing.T) {
	ln := listen(t)
	stuck := make(chan struct{})
	t.Cleanup(func() { close(stuck) })
	entered := make(chan struct{})
	srv := &http.Server{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		// Like a stream whose client stopped reading: nothing ends it.
		close(entered)
		<-stuck
	})}
	svc := &service{
		http:            srv,
		listen:          func() error { return srv.Serve(ln) },
		shutdownTimeout: 100 * time.Millisecond,
	}

	go func() {
		if resp, err := http.Get("http://" + ln.Addr().String()); err == nil {
			_ = resp.Body.Close()
		}
	}()
	runUntilCancelled(t, svc, entered, func() {})
}
