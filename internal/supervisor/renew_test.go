package supervisor

import (
	"errors"
	"fmt"
	"testing"

	"github.com/bumaruf/discord-go-vpn/internal/profile"
)

func TestRunShowsRenewFailureAndKeepsSession(t *testing.T) {
	errRenew := errors.New("não consegui renovar o acesso: tempo esgotado")
	h := newHarness(t, Options{}, step{renewErr: errRenew})
	h.run()

	h.wantEvents("start#1", "enter#1 ", "stop#1", "close")
	if len(h.probes) == 0 {
		t.Fatal("no health probe after the renewal failure")
	}
	st := h.probes[0]
	if st.State != StateHealthy || !errors.Is(st.LastError, errRenew) || st.LastErrorAt.IsZero() {
		t.Errorf("status = {State: %v, LastError: %v, LastErrorAt: %v}, want {%v, %v, set}",
			st.State, st.LastError, st.LastErrorAt, StateHealthy, errRenew)
	}
}

func TestRunTreatsLostLeaseAsDrop(t *testing.T) {
	errLost := fmt.Errorf("%w: o servidor recusou (410 Gone): acesso expirado", profile.ErrLeaseLost)
	h := newHarness(t, Options{}, step{renewErr: errLost}, fail, fail, fail)
	h.run()

	h.wantEvents("start#1", "enter#1 ", "stop#1", "wait 3s", "wait 6s", "leave ", "wait 12s", "close")
	if len(h.connects) < 2 {
		t.Fatalf("got %d connection attempts, want a reconnect after the lost lease", len(h.connects))
	}
	if st := h.connects[1]; !errors.Is(st.LastError, profile.ErrLeaseLost) {
		t.Errorf("status on reconnect: LastError = %v, want %v", st.LastError, errLost)
	}
	wantStates := []State{StateDegraded, StateDegraded, StateFallback}
	for i, w := range h.waits {
		if w.status.State != wantStates[i] {
			t.Errorf("wait %d: state = %v, want %v", i, w.status.State, wantStates[i])
		}
	}
}
