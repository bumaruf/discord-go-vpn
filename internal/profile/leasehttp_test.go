package profile

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/curve25519"

	"github.com/bumaruf/discord-go-vpn/internal/platform/osuser"
	"github.com/bumaruf/discord-go-vpn/internal/profile/profiletest"
)

func newClient(f *profiletest.Provisiond) *Client {
	return &Client{Server: f.URL, Token: "vpn_test", Fingerprint: f.Fingerprint()}
}

type manualTicker struct {
	intervals chan time.Duration
	ticks     chan time.Time
	stopped   chan struct{}
}

func newManualTicker() *manualTicker {
	return &manualTicker{
		intervals: make(chan time.Duration, 1),
		ticks:     make(chan time.Time), // unbuffered: a send returns only once keepAlive is waiting again
		stopped:   make(chan struct{}),
	}
}

func (m *manualTicker) newTicker(d time.Duration) (<-chan time.Time, func()) {
	m.intervals <- d
	return m.ticks, func() { close(m.stopped) }
}

func (m *manualTicker) tick(t *testing.T) {
	t.Helper()
	select {
	case m.ticks <- time.Now():
	case <-time.After(5 * time.Second):
		t.Fatal("keepAlive did not take the tick")
	}
}

func receive[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		var zero T
		return zero
	}
}

func TestAcquireRequestsLeaseAndBuildsConfig(t *testing.T) {
	t.Setenv("SUDO_USER", "ana") // makes the OS user known on Linux
	f := profiletest.NewProvisiond(t)
	c := newClient(f)
	c.newTicker = newManualTicker().newTicker

	lease, err := c.Acquire()
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer lease.Release()

	calls := f.Calls("/lease")
	if len(calls) != 1 {
		t.Fatalf("got %d /lease calls, want 1", len(calls))
	}
	req := calls[0]
	hostname, _ := os.Hostname()
	if req["token"] != "vpn_test" || req["label"] != hostname {
		t.Errorf("/lease body = %v, want token vpn_test and label %q", req, hostname)
	}
	if user := osuser.Name(); user == "" || req["os_user"] != user {
		t.Errorf("/lease body = %v, want os_user %q", req, user)
	}

	priv, err := base64.StdEncoding.DecodeString(lease.Config.PrivateKey)
	if err != nil {
		t.Fatalf("private key is not base64: %v", err)
	}
	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		t.Fatalf("deriving public key: %v", err)
	}
	if got := base64.StdEncoding.EncodeToString(pub); got != req["pubkey"] {
		t.Errorf("sent pubkey %q does not match the config's private key (derives %q)", req["pubkey"], got)
	}

	if got := lease.Config.Addresses; len(got) != 1 || got[0] != netip.MustParseAddr("10.8.0.7") {
		t.Errorf("Addresses = %v, want [10.8.0.7]", got)
	}
	if lease.Config.Peer.Endpoint != "203.0.113.10:51820" {
		t.Errorf("Endpoint = %q", lease.Config.Peer.Endpoint)
	}
}

func TestAcquirePinsCertificateFingerprint(t *testing.T) {
	f := profiletest.NewProvisiond(t)

	// Every httptest server shares one certificate, so a second server would pin
	// the same fingerprint; a well-formed digest of something else does not.
	other := sha256.Sum256([]byte("another certificate"))
	c := newClient(f)
	c.Fingerprint = base64.StdEncoding.EncodeToString(other[:])

	_, err := c.Acquire()
	if err == nil || !strings.Contains(err.Error(), "não é o esperado") {
		t.Fatalf("Acquire with a different fingerprint: err = %v, want certificate mismatch", err)
	}
	if n := len(f.Calls("/lease")); n != 0 {
		t.Errorf("server received %d /lease calls through an unpinned connection", n)
	}
}

func TestAcquireRejectsMalformedFingerprint(t *testing.T) {
	f := profiletest.NewProvisiond(t)
	for name, fp := range map[string]string{
		"empty":      "",
		"not base64": "not*base64",
		"wrong size": base64.StdEncoding.EncodeToString([]byte("short")),
	} {
		t.Run(name, func(t *testing.T) {
			c := newClient(f)
			c.Fingerprint = fp
			if _, err := c.Acquire(); err == nil {
				t.Fatal("Acquire succeeded with a malformed fingerprint")
			}
		})
	}
	if n := len(f.Calls("/lease")); n != 0 {
		t.Errorf("server received %d /lease calls", n)
	}
}

func TestAcquireRefusalCarriesStatusInMessage(t *testing.T) {
	f := profiletest.NewProvisiond(t)
	f.LeaseStatus = http.StatusUnauthorized

	_, err := newClient(f).Acquire()
	if err == nil {
		t.Fatal("Acquire succeeded on 401")
	}
	if !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "convite inválido") {
		t.Errorf("err = %q, want status and server reason", err)
	}
}

func TestLeaseRenewInterval(t *testing.T) {
	tests := []struct {
		ttlSeconds int
		want       time.Duration
	}{
		{180, 60 * time.Second},
		{30, 20 * time.Second},
		{0, 20 * time.Second},
	}
	for _, tt := range tests {
		f := profiletest.NewProvisiond(t)
		f.TTLSeconds = tt.ttlSeconds
		ticker := newManualTicker()
		c := newClient(f)
		c.newTicker = ticker.newTicker

		lease, err := c.Acquire()
		if err != nil {
			t.Fatalf("Acquire: %v", err)
		}
		if got := receive(t, ticker.intervals, "renew interval"); got != tt.want {
			t.Errorf("ttl %ds: renew interval = %v, want %v", tt.ttlSeconds, got, tt.want)
		}
		lease.Release()
	}
}

func TestLeaseRenewsOnEveryTick(t *testing.T) {
	f := profiletest.NewProvisiond(t)
	ticker := newManualTicker()
	c := newClient(f)
	c.newTicker = ticker.newTicker

	lease, err := c.Acquire()
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer lease.Release()

	ticker.tick(t)
	receive(t, f.Renewed, "first /renew")
	ticker.tick(t) // returns only after the first renewal finished, failure report included
	receive(t, f.Renewed, "second /renew")

	select {
	case err := <-lease.RenewFailures():
		t.Fatalf("renewal failure reported after a successful renewal: %v", err)
	default:
	}
	for _, body := range f.Calls("/renew") {
		if body["token"] != "vpn_test" || body["id"] != "lease-1" {
			t.Errorf("/renew body = %v, want token vpn_test and id lease-1", body)
		}
	}
}

func TestLeaseReportsRenewFailure(t *testing.T) {
	f := profiletest.NewProvisiond(t)
	f.RenewStatus = http.StatusGone
	ticker := newManualTicker()
	c := newClient(f)
	c.newTicker = ticker.newTicker

	lease, err := c.Acquire()
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer lease.Release()

	ticker.tick(t)
	if err := receive(t, lease.RenewFailures(), "renewal failure"); !strings.Contains(err.Error(), "410") {
		t.Errorf("renew failure = %q, want status 410", err)
	}
}

// With the fake's 180s TTL renewals run every 60s, so the third straight failure
// spans the TTL.
func TestLeaseRenewFailureMarksLostLease(t *testing.T) {
	for name, tc := range map[string]struct {
		status      int
		unreachable bool
		lostOn      int
	}{
		"expired lease":      {status: http.StatusGone, lostOn: 1},
		"refused token":      {status: http.StatusUnauthorized, lostOn: 3},
		"server error":       {status: http.StatusServiceUnavailable, lostOn: 3},
		"unreachable server": {status: http.StatusOK, unreachable: true, lostOn: 3},
	} {
		t.Run(name, func(t *testing.T) {
			f := profiletest.NewProvisiond(t)
			f.RenewStatus = tc.status
			ticker := newManualTicker()
			c := newClient(f)
			c.newTicker = ticker.newTicker

			lease, err := c.Acquire()
			if err != nil {
				t.Fatalf("Acquire: %v", err)
			}
			defer lease.Release()
			if tc.unreachable {
				f.Close()
			}

			for i := 1; i <= tc.lostOn; i++ {
				ticker.tick(t)
				err := receive(t, lease.RenewFailures(), "renewal failure")
				if got, want := errors.Is(err, ErrLeaseLost), i == tc.lostOn; got != want {
					t.Errorf("failure %d: errors.Is(%q, ErrLeaseLost) = %v, want %v", i, err, got, want)
				}
			}
		})
	}
}

func TestLeaseRenewSuccessRestartsLostCount(t *testing.T) {
	f := profiletest.NewProvisiond(t)
	ticker := newManualTicker()
	c := newClient(f)
	c.newTicker = ticker.newTicker

	lease, err := c.Acquire()
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer lease.Release()

	for i, status := range []int{http.StatusServiceUnavailable, http.StatusServiceUnavailable, http.StatusOK, http.StatusServiceUnavailable, http.StatusServiceUnavailable} {
		f.SetRenewStatus(status)
		ticker.tick(t)
		receive(t, f.Renewed, "/renew")
		if status == http.StatusOK {
			continue
		}
		if err := receive(t, lease.RenewFailures(), "renewal failure"); errors.Is(err, ErrLeaseLost) {
			t.Errorf("renewal %d: %q is ErrLeaseLost, but a success came between the failures", i+1, err)
		}
	}
}

func TestLeaseKeepsRenewingWhileFailuresGoUnread(t *testing.T) {
	f := profiletest.NewProvisiond(t)
	f.RenewStatus = http.StatusUnauthorized
	ticker := newManualTicker()
	c := newClient(f)
	c.newTicker = ticker.newTicker

	lease, err := c.Acquire()
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer lease.Release()

	for range 4 {
		ticker.tick(t)
		receive(t, f.Renewed, "/renew")
	}
	// The fourth tick is taken only after the third failure was reported, which
	// already spans the TTL: an older, not-yet-lost failure must not be left.
	if err := receive(t, lease.RenewFailures(), "renewal failure"); !errors.Is(err, ErrLeaseLost) {
		t.Errorf("unread failure = %q, want the latest one, ErrLeaseLost", err)
	}
}

func TestLeaseReleaseStopsRenewalsAndReleasesOnce(t *testing.T) {
	f := profiletest.NewProvisiond(t)
	ticker := newManualTicker()
	c := newClient(f)
	c.newTicker = ticker.newTicker

	lease, err := c.Acquire()
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	lease.Release()
	lease.Release()

	receive(t, ticker.stopped, "renew ticker stop")
	calls := f.Calls("/release")
	if len(calls) != 1 {
		t.Fatalf("got %d /release calls, want 1", len(calls))
	}
	if calls[0]["token"] != "vpn_test" || calls[0]["id"] != "lease-1" {
		t.Errorf("/release body = %v, want token vpn_test and id lease-1", calls[0])
	}
	if n := len(f.Calls("/renew")); n != 0 {
		t.Errorf("got %d /renew calls, want 0", n)
	}
}

func TestLeaseReleaseIgnoresUnreachableServer(t *testing.T) {
	f := profiletest.NewProvisiond(t)
	c := newClient(f)
	c.newTicker = newManualTicker().newTicker

	lease, err := c.Acquire()
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	f.Close()

	lease.Release()
}

func TestOpenWithProvisioningReturnsTemporarySource(t *testing.T) {
	f := profiletest.NewProvisiond(t)

	src, err := Open("", Provisioning{Server: f.URL, Token: "vpn_test", Fingerprint: f.Fingerprint()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if src.Origin != "acesso do servidor" || src.Config == nil || src.RenewFailures == nil {
		t.Errorf("Source = {Origin: %q, Config: %v, RenewFailures: %v}, want a temporary lease", src.Origin, src.Config, src.RenewFailures)
	}

	src.Close()
	src.Close()
	receive(t, f.Released, "/release")
	if n := len(f.Calls("/release")); n != 1 {
		t.Errorf("got %d /release calls, want 1", n)
	}
}

func TestOpenWrapsLeaseError(t *testing.T) {
	f := profiletest.NewProvisiond(t)
	f.LeaseStatus = http.StatusTooManyRequests

	_, err := Open("", Provisioning{Server: f.URL, Token: "vpn_test", Fingerprint: f.Fingerprint()})
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("Open: err = %v, want the 429 refusal", err)
	}
	var status *StatusError
	if !errors.As(err, &status) || status.StatusCode != http.StatusTooManyRequests {
		t.Errorf("Open: err = %v, want a *StatusError with StatusCode 429", err)
	}
}

func TestIsRefusal(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"401", &StatusError{StatusCode: http.StatusUnauthorized}, true},
		{"403", &StatusError{StatusCode: http.StatusForbidden}, true},
		{"wrapped 401", fmt.Errorf("não consegui obter um acesso: %w", &StatusError{StatusCode: http.StatusUnauthorized}), true},
		{"410", &StatusError{StatusCode: http.StatusGone}, false},
		{"429", &StatusError{StatusCode: http.StatusTooManyRequests}, false},
		{"502", &StatusError{StatusCode: http.StatusBadGateway, reason: "upstream answered 401"}, false},
		{"no status", errors.New("401"), false},
		{"nil", nil, false},
	}
	for _, tc := range tests {
		if got := IsRefusal(tc.err); got != tc.want {
			t.Errorf("%s: IsRefusal(%v) = %v, want %v", tc.name, tc.err, got, tc.want)
		}
	}
}

func TestOpenWithoutSourceReturnsErrNoProfile(t *testing.T) {
	for name, prov := range map[string]Provisioning{
		"nothing":        {},
		"no token":       {Server: "https://127.0.0.1:1", Fingerprint: "x"},
		"no fingerprint": {Server: "https://127.0.0.1:1", Token: "vpn_test"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Open("", prov); !errors.Is(err, ErrNoProfile) {
				t.Fatalf("Open: err = %v, want ErrNoProfile", err)
			}
		})
	}
}
