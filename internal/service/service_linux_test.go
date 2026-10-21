//go:build linux && !android

package service

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/connectiontoken"
	"github.com/bumaruf/discord-go-vpn/internal/control"
	"github.com/bumaruf/discord-go-vpn/internal/routing"
	"github.com/bumaruf/discord-go-vpn/internal/startup"
	"github.com/bumaruf/discord-go-vpn/internal/supervisor"
	"github.com/bumaruf/discord-go-vpn/internal/transport"
	"github.com/bumaruf/discord-go-vpn/internal/wg"
)

type fakeTransport struct{ rx atomic.Uint64 }

func (f *fakeTransport) Start(*wg.Config) error      { return nil }
func (f *fakeTransport) Ready() error                { return nil }
func (f *fakeTransport) Open(string) (string, error) { return "", nil }
func (f *fakeTransport) Close()                      {}
func (f *fakeTransport) Traffic() (wg.Traffic, error) {
	return wg.Traffic{RxBytes: f.rx.Add(1000)}, nil
}

type recorder struct {
	mu    sync.Mutex
	steps []startup.State
}

func (r *recorder) Step(_ startup.Step, state startup.State, _ string, _ float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.steps = append(r.steps, state)
}
func (r *recorder) Tell(string)          {}
func (r *recorder) Ask() (string, error) { return "", context.Canceled }

func TestServiceRunsTheTunnelForItsClients(t *testing.T) {
	t.Setenv("SUDO_USER", "")
	t.Setenv("SUDO_UID", "")
	me, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	conf := filepath.Join(dir, "wg0.conf")
	if err := os.WriteFile(conf, []byte("[Interface]\nPrivateKey = yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk=\nAddress = 10.9.0.12/32\n"+
		"[Peer]\nPublicKey = xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg=\nEndpoint = 203.0.113.10:51820\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "control.sock")

	var discordOutside atomic.Bool
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan struct{})
	go func() {
		defer close(served)
		_, _ = Run(ctx, Options{
			User: me.Username, ConfigPath: conf, Version: "v1.0.0", Socket: socket,
			NewTransport: func(routing.Mode, transport.Reporter) (transport.Transport, error) {
				return &fakeTransport{}, nil
			},
			RestartDiscord: func(string) (string, error) { discordOutside.Store(true); return "", nil },
		})
	}()

	first := dialWhenUp(t, socket)
	rec := &recorder{}
	rm, sess, err := control.Attach(first, rec)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if sess.Profile != "arquivo próprio (wg0.conf)" || sess.Leasing {
		t.Errorf("session = %+v", sess)
	}
	healthy := waitFor(t, rm.Updates(), func(st supervisor.Status) bool {
		return st.State == supervisor.StateHealthy && st.DiscordTunneled
	})
	if healthy.Endpoint != "203.0.113.10:51820" {
		t.Errorf("endpoint = %q", healthy.Endpoint)
	}
	if len(rec.steps) == 0 {
		t.Error("the client saw none of startup's steps")
	}
	if os.Getenv("SUDO_USER") != me.Username {
		t.Error("Discord must open as the service's user")
	}

	// A client arriving later catches up without starting a second tunnel.
	late, _, err := control.Attach(dialWhenUp(t, socket), &recorder{})
	if err != nil {
		t.Fatalf("late Attach: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for !late.Reading().Metered && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !late.Reading().Metered {
		t.Error("the late client never got a reading")
	}

	if err := first.Send(control.Request{Op: control.OpDisconnect}); err != nil {
		t.Fatal(err)
	}
	for range rm.Updates() {
	}
	if !discordOutside.Load() {
		t.Error("disconnecting must give Discord back to the normal network")
	}

	cancel()
	select {
	case <-served:
	case <-time.After(5 * time.Second):
		t.Fatal("Run didn't return after its context ended")
	}
	if _, err := os.Stat(socket); !os.IsNotExist(err) {
		t.Error("the socket outlived the service")
	}
}

// asker types a malformed token, then gives up.
type asker struct {
	recorder
	asked atomic.Int32
	told  chan string
}

func (a *asker) Tell(msg string) { a.told <- msg }
func (a *asker) Ask() (string, error) {
	if a.asked.Add(1) == 1 {
		return "nao-e-token", nil
	}
	return "", context.Canceled
}

func TestServiceAsksAClientForTheToken(t *testing.T) {
	t.Setenv("SUDO_USER", "")
	t.Setenv("SUDO_UID", "")
	t.Setenv("VPN_TOKEN", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	me, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(t.TempDir(), "control.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _, _ = Run(ctx, Options{User: me.Username, Version: "v1.0.0", Socket: socket}) }()

	a := &asker{told: make(chan string, 4)}
	go func() { _, _, _ = control.Attach(dialWhenUp(t, socket), a) }()

	select {
	case msg := <-a.told:
		if msg != connectiontoken.MalformedHelp {
			t.Errorf("told %q, want why the token was refused", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the service never asked the client for a token")
	}
}

func dialWhenUp(t *testing.T, socket string) *control.Conn {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		c, err := control.DialPath(socket)
		if err == nil {
			t.Cleanup(func() { _ = c.Close() })
			return c
		}
		if time.Now().After(deadline) {
			t.Fatalf("the service never listened: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitFor(t *testing.T, updates <-chan supervisor.Status, ok func(supervisor.Status) bool) supervisor.Status {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case st, open := <-updates:
			if !open {
				t.Fatal("updates closed before the state was reached")
			}
			if ok(st) {
				return st
			}
		case <-timeout:
			t.Fatal("the state was never reached")
		}
	}
}
