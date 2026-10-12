//go:build linux

package mobile

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/connectiontoken"
	"github.com/bumaruf/discord-go-vpn/internal/profile/profiletest"
	"github.com/bumaruf/discord-go-vpn/internal/transport/vpnservice"
)

// fakeHost refuses the interface, so each attempt leases, fails to start the
// transport and releases, which is enough to watch the supervisor run.
type fakeHost struct {
	mu          sync.Mutex
	established []string
	updates     []*Status
	changed     chan struct{}
}

func newFakeHost() *fakeHost { return &fakeHost{changed: make(chan struct{}, 64)} }

func (h *fakeHost) Establish(addresses, routes, dns string, mtu int) (int, error) {
	h.mu.Lock()
	h.established = append(h.established, addresses+"|"+routes+"|"+dns)
	h.mu.Unlock()
	return -1, errors.New("permissão negada")
}

func (h *fakeHost) OpenDiscord() {}

func (h *fakeHost) Update(s *Status) {
	h.mu.Lock()
	h.updates = append(h.updates, s)
	h.mu.Unlock()
	select {
	case h.changed <- struct{}{}:
	default:
	}
}

// waitFor returns the first update that matches, failing the test after a while.
func (h *fakeHost) waitFor(t *testing.T, match func(*Status) bool) *Status {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		h.mu.Lock()
		for _, s := range h.updates {
			if match(s) {
				h.mu.Unlock()
				return s
			}
		}
		h.mu.Unlock()
		select {
		case <-h.changed:
		case <-deadline:
			t.Fatal("no matching status update")
		}
	}
}

func setup(t *testing.T) *profiletest.Provisiond {
	t.Helper()
	if err := Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("XDG_CONFIG_HOME") })
	return profiletest.NewProvisiond(t)
}

func tokenFor(p *profiletest.Provisiond) string {
	return connectiontoken.Token{Server: p.URL, Fingerprint: p.Fingerprint(), Invite: "vpn_64wu72y2-4vjhv8yv-hmanr9te"}.String()
}

func TestCheckToken(t *testing.T) {
	if err := CheckToken("dgv1_nope"); err == nil || err.Error() != connectiontoken.MalformedHelp {
		t.Errorf("CheckToken(malformed) = %v, want MalformedHelp", err)
	}
	if err := CheckToken(tokenFor(setup(t))); err != nil {
		t.Errorf("CheckToken(valid) = %v", err)
	}
}

func TestConnectRefusedForgetsToken(t *testing.T) {
	p := setup(t)
	token := tokenFor(p)
	if err := save(token); err != nil {
		t.Fatal(err)
	}
	p.LeaseStatus = http.StatusUnauthorized

	s, err := Connect(token, "Pixel 8", newFakeHost())
	if s != nil || err == nil || err.Error() != connectiontoken.RefusedHelp {
		t.Fatalf("Connect = (%v, %v), want RefusedHelp", s, err)
	}
	if SavedToken() != "" {
		t.Error("a refused token stayed saved")
	}
}

func TestConnectRunsSupervisorOnValidationLease(t *testing.T) {
	p := setup(t)
	token := tokenFor(p)
	host := newFakeHost()

	s, err := Connect(token, "Pixel 8", host)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if SavedToken() != token {
		t.Error("an accepted token was not saved")
	}

	failed := host.waitFor(t, func(s *Status) bool { return s.State == StateDegraded })
	if failed.LastError == "" || failed.Profile != "acesso do servidor" || failed.Server != "127.0.0.1" {
		t.Errorf("degraded status = %+v", failed)
	}
	s.Stop()

	calls := p.Calls("/lease")
	if len(calls) == 0 || calls[0]["label"] != "Pixel 8" {
		t.Errorf("first /lease = %v, want label Pixel 8", calls)
	}
	host.mu.Lock()
	defer host.mu.Unlock()
	if last := host.updates[len(host.updates)-1]; last.State != StateStopped {
		t.Errorf("last update = %+v, want stopped", last)
	}
	if leases, releases := len(p.Calls("/lease")), len(p.Calls("/release")); leases != releases {
		t.Errorf("%d leases and %d releases: every lease must be released", leases, releases)
	}
}

func TestHostAdapterJoinsLists(t *testing.T) {
	host := newFakeHost()
	_, _ = hostAdapter{host}.Establish(vpnservice.Interface{
		Addresses: []string{"10.9.0.10/32", "fd00::a/128"},
		Routes:    []string{"0.0.0.0/0", "::/0"},
		DNS:       []string{"10.9.0.1"},
		MTU:       1420,
	})
	if len(host.established) != 1 || host.established[0] != "10.9.0.10/32,fd00::a/128|0.0.0.0/0,::/0|10.9.0.1" {
		t.Errorf("established = %v", host.established)
	}
}

func TestConnectFile(t *testing.T) {
	setup(t)
	path := filepath.Join(t.TempDir(), "casa.conf")
	conf := `[Interface]
PrivateKey = yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk=
Address = 10.2.0.2/32

[Peer]
PublicKey = xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg=
Endpoint = 127.0.0.1:51820
`
	if err := os.WriteFile(path, []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	host := newFakeHost()

	s, err := ConnectFile(path, host)
	if err != nil {
		t.Fatalf("ConnectFile: %v", err)
	}
	st := host.waitFor(t, func(s *Status) bool { return s.State == StateDegraded })
	s.Stop()
	if st.Profile != "arquivo próprio (casa.conf)" || st.Server != "127.0.0.1" {
		t.Errorf("status = %+v", st)
	}
}

func TestConnectFileReportsBrokenConf(t *testing.T) {
	setup(t)
	if _, err := ConnectFile(filepath.Join(t.TempDir(), "missing.conf"), newFakeHost()); err == nil {
		t.Error("ConnectFile succeeded without a file")
	}
}

func TestUseTokenRejectsMalformed(t *testing.T) {
	if err := (&Session{}).UseToken("lixo"); err == nil || err.Error() != connectiontoken.MalformedHelp {
		t.Errorf("UseToken(malformed) = %v, want MalformedHelp", err)
	}
}

func save(typed string) error {
	token, err := connectiontoken.Parse(typed)
	if err != nil {
		return err
	}
	connectiontoken.Save(token)
	return nil
}
