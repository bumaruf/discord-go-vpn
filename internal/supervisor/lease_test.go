package supervisor

import (
	"testing"

	"github.com/bumaruf/discord-go-vpn/internal/profile"
	"github.com/bumaruf/discord-go-vpn/internal/profile/profiletest"
)

func provisioning(p *profiletest.Provisiond) profile.Provisioning {
	return profile.Provisioning{Server: p.URL, Token: "vpn_test", Fingerprint: p.Fingerprint()}
}

// newLeaseHarness leases from a fake provisiond on every connect and hands Run
// a lease acquired up front, as startup does. steps[0] scripts the attempt that
// uses the handed lease.
func newLeaseHarness(t *testing.T, opts Options, steps ...step) (*harness, *profiletest.Provisiond) {
	t.Helper()
	prov := profiletest.NewProvisiond(t)
	src, err := profile.Open("", provisioning(prov))
	if err != nil {
		t.Fatalf("profile.Open: %v", err)
	}
	opts.Source = src
	h := newHarness(t, opts, steps...)
	h.prov = prov
	h.next = 1
	return h, prov
}

func wantLeaseCalls(t *testing.T, p *profiletest.Provisiond, leases, releases int) {
	t.Helper()
	if got := len(p.Calls("/lease")); got != leases {
		t.Errorf("got %d /lease calls, want %d", got, leases)
	}
	if got := len(p.Calls("/release")); got != releases {
		t.Errorf("got %d /release calls, want %d", got, releases)
	}
}

func TestRunFirstConnectionUsesHandedLease(t *testing.T) {
	h, prov := newLeaseHarness(t, Options{}, step{})
	h.run()

	h.wantEvents("start#1", "open-discord#1 ", "close#1", "restart-discord ")
	wantLeaseCalls(t, prov, 1, 1)
}

func TestRunReconnectAcquiresFreshLease(t *testing.T) {
	h, prov := newLeaseHarness(t, Options{}, step{drop: true}, step{})
	h.run()

	h.wantEvents("start#1", "open-discord#1 ", "close#1", "start#2", "open-discord#2 ", "close#2", "restart-discord ")
	wantLeaseCalls(t, prov, 2, 2)
	if len(h.releasedAtLease) != 1 || h.releasedAtLease[0] != 1 {
		t.Errorf("/release calls before the fresh lease = %v, want [1]", h.releasedAtLease)
	}
	if len(h.probeTunnels) != 2 || h.probeTunnels[1] != 2 {
		t.Errorf("tunnel up at each health probe = %v, want [1 2]", h.probeTunnels)
	}
}

func TestRunReleasesHandedLeaseWhenFirstConnectionFails(t *testing.T) {
	for name, tc := range map[string]struct {
		opts Options
		step step
		want State
	}{
		"start fails":               {Options{}, step{startErr: errStart}, StateDegraded},
		"ready fails into fallback": {Options{MaxFailures: 1}, step{readyErr: errReady}, StateFallback},
	} {
		t.Run(name, func(t *testing.T) {
			h, prov := newLeaseHarness(t, tc.opts, tc.step)
			h.run()

			if got := h.waits[0].status.State; got != tc.want {
				t.Errorf("state after the failure = %v, want %v", got, tc.want)
			}
			wantLeaseCalls(t, prov, 1, 1)
		})
	}
}

func TestRunReleasesUnusedHandedLeaseOnCancel(t *testing.T) {
	h, prov := newLeaseHarness(t, Options{})
	h.cancel()
	h.run()

	wantLeaseCalls(t, prov, 1, 1)
}
