package supervisor

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/profile"
	"github.com/bumaruf/discord-go-vpn/internal/profile/profiletest"
	"github.com/bumaruf/discord-go-vpn/internal/routing"
	"github.com/bumaruf/discord-go-vpn/internal/transport"
	"github.com/bumaruf/discord-go-vpn/internal/wg"
)

var (
	errConnect = errors.New("connect failed")
	errStart   = errors.New("start failed")
	errReady   = errors.New("ready failed")
	errLeave   = errors.New("leave failed")
)

const discordPath = "/opt/discord"

// step scripts one connection attempt. A zero step connects; a healthy session
// without drop ends the scenario, and so does the backoff after the last step.
type step struct {
	profileErr error
	startErr   error
	readyErr   error
	enterErr   error
	drop       bool
	renewErr   error                    // already waiting on the session's renewal channel
	report     func(transport.Reporter) // what Start reports before returning
}

var fail = step{profileErr: errConnect}

type waitCall struct {
	d      time.Duration
	status Status
}

// harness runs Supervisor.Run against scripted dependencies. Its fields are
// written only by the Run goroutine and read after Run returns.
type harness struct {
	t      *testing.T
	sup    *Supervisor
	ctx    context.Context
	cancel context.CancelFunc

	steps     []step
	next      int
	nudgeOn   map[int]bool
	leaveErrs []error
	prov      *profiletest.Provisiond // when set, every connect leases from it

	transports      int        // transports created; Run needs only one
	renewals        chan error // renewal channel of the source the last connect opened
	tunnel          int        // transport whose tunnel holds the fixed name; 0 when none does
	events          []string
	waits           []waitCall
	probes          []Status
	probeTunnels    []int    // tunnel as each health probe ran
	releasedAtLease []int    // /release calls made before each lease a connect acquired
	starts          []Status // status as each Start returned
	connects        []Status // status as each scripted connect began
}

func newHarness(t *testing.T, opts Options, steps ...step) *harness {
	t.Helper()
	opts.ProbeEvery = time.Millisecond
	h := &harness{t: t, steps: steps, nudgeOn: map[int]bool{}}
	h.ctx, h.cancel = context.WithCancel(context.Background())
	t.Cleanup(h.cancel)

	h.sup = New(opts)
	h.sup.openProfile = h.openProfile
	h.sup.newTransport = h.newTransport
	h.sup.wait = h.wait
	return h
}

func (h *harness) run() {
	h.t.Helper()
	done := make(chan struct{})
	go func() {
		h.sup.Run(h.ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		h.t.Fatal("Run did not return")
	}
}

func (h *harness) record(format string, args ...any) {
	h.events = append(h.events, fmt.Sprintf(format, args...))
}

func (h *harness) openProfile(string, profile.Provisioning) (*profile.Source, error) {
	if h.next == len(h.steps) {
		h.t.Error("connect attempted after the last step")
		h.cancel()
		return nil, h.ctx.Err()
	}
	h.next++
	h.connects = append(h.connects, h.sup.Status())
	h.renewals = nil
	st := h.steps[h.next-1]
	if st.profileErr != nil {
		return nil, st.profileErr
	}
	if h.prov != nil {
		h.releasedAtLease = append(h.releasedAtLease, len(h.prov.Calls("/release")))
		return profile.Open("", provisioning(h.prov))
	}
	src := &profile.Source{Config: &wg.Config{}}
	if st.renewErr != nil {
		h.renewals = make(chan error, 1)
		h.renewals <- st.renewErr
		src.RenewFailures = h.renewals
	}
	return src, nil
}

func (h *harness) newTransport(_ routing.Mode, path string, report transport.Reporter) (transport.Transport, error) {
	h.transports++
	return &fakeTransport{h: h, path: path, report: report}, nil
}

func (h *harness) wait(_ context.Context, d time.Duration, _ <-chan struct{}) (alive, nudged bool) {
	h.record("wait %v", d)
	h.waits = append(h.waits, waitCall{d: d, status: h.sup.Status()})
	if h.next == len(h.steps) {
		h.cancel()
		return false, false
	}
	return true, h.nudgeOn[len(h.waits)-1]
}

func (h *harness) waitDurations() []time.Duration {
	ds := make([]time.Duration, len(h.waits))
	for i, w := range h.waits {
		ds[i] = w.d
	}
	return ds
}

func (h *harness) wantEvents(want ...string) {
	h.t.Helper()
	if !reflect.DeepEqual(h.events, want) {
		h.t.Errorf("events =\n  %q\nwant\n  %q", h.events, want)
	}
}

// fakeTransport numbers the tunnels its Start calls bring up, and scripts each
// from the step being connected.
type fakeTransport struct {
	h          *harness
	path       string
	report     transport.Reporter
	id         int
	step       step
	renewals   chan error
	readyCalls int
}

func (f *fakeTransport) Start(*wg.Config) error {
	f.id++
	f.step, f.renewals, f.readyCalls = f.h.steps[f.h.next-1], f.h.renewals, 0
	f.h.record("start#%d", f.id)
	if f.step.report != nil {
		f.step.report(f.report)
	}
	f.h.starts = append(f.h.starts, f.h.sup.Status())
	if f.step.startErr == nil {
		f.h.tunnel = f.id
	}
	return f.step.startErr
}

// Ready answers connect's check first, then the health probes of watch. Probes
// pass without effect while a renewal failure is unread, since watch may pick
// the probe tick first.
func (f *fakeTransport) Ready() error {
	f.readyCalls++
	if f.readyCalls == 1 {
		return f.step.readyErr
	}
	if len(f.renewals) > 0 {
		if f.readyCalls > 1000 {
			f.h.t.Error("the session never read its renewal failure")
			f.h.cancel()
		}
		return nil
	}
	f.h.probes = append(f.h.probes, f.h.sup.Status())
	f.h.probeTunnels = append(f.h.probeTunnels, f.h.tunnel)
	if f.step.drop {
		return errors.New("tunnel stopped answering")
	}
	f.h.cancel()
	return nil
}

// Stop tears down by fixed name, as both real transports do: the interface
// wg-discord on Linux, every wiresock-client.exe on Windows.
func (f *fakeTransport) Stop() {
	f.h.record("stop#%d", f.id)
	f.h.tunnel = 0
}

func (f *fakeTransport) Enter() error {
	f.h.record("enter#%d %s", f.id, f.path)
	return f.step.enterErr
}

func (f *fakeTransport) Leave() error {
	f.h.record("leave %s", f.path)
	var err error
	if len(f.h.leaveErrs) > 0 {
		err, f.h.leaveErrs = f.h.leaveErrs[0], f.h.leaveErrs[1:]
	}
	return err
}

func (f *fakeTransport) Close() {
	f.h.record("close")
	f.h.tunnel = 0
}

func TestRunBackoffDoublesUpToCap(t *testing.T) {
	h := newHarness(t, Options{}, fail, fail, fail, fail, fail, fail, fail)
	h.run()

	want := []time.Duration{3 * time.Second, 6 * time.Second, 12 * time.Second, 24 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second}
	if got := h.waitDurations(); !reflect.DeepEqual(got, want) {
		t.Errorf("backoff = %v, want %v", got, want)
	}
	for i, w := range h.waits {
		st := w.status
		wantState := StateDegraded
		if i >= defaultMaxFailures-1 {
			wantState = StateFallback
		}
		if st.State != wantState || st.Failures != i+1 || st.Attempt != i+1 || !errors.Is(st.LastError, errConnect) {
			t.Errorf("wait %d: status {State: %v, Failures: %d, Attempt: %d, LastError: %v}, want {%v, %d, %d, %v}",
				i, st.State, st.Failures, st.Attempt, st.LastError, wantState, i+1, i+1, errConnect)
		}
	}
	if st := h.sup.Status(); st.State != StateStopped {
		t.Errorf("final state = %v, want %v", st.State, StateStopped)
	}
}

func TestRunNudgeResetsBackoff(t *testing.T) {
	h := newHarness(t, Options{}, fail, fail, fail, fail, fail)
	h.nudgeOn[1] = true
	h.run()

	want := []time.Duration{3 * time.Second, 6 * time.Second, 3 * time.Second, 6 * time.Second, 12 * time.Second}
	if got := h.waitDurations(); !reflect.DeepEqual(got, want) {
		t.Errorf("backoff = %v, want %v", got, want)
	}
}

func TestRunHonorsMaxFailures(t *testing.T) {
	h := newHarness(t, Options{MaxFailures: 1}, fail)
	h.run()

	if st := h.waits[0].status; st.State != StateFallback {
		t.Errorf("state after one failure = %v, want %v", st.State, StateFallback)
	}
}

func TestRunFallbackHandsDiscordBackToNormalNetwork(t *testing.T) {
	h := newHarness(t, Options{DiscordPath: discordPath},
		step{drop: true}, fail, fail, fail, fail)
	h.run()

	h.wantEvents(
		"start#1", "enter#1 "+discordPath, "stop#1",
		"wait 3s", "wait 6s",
		"leave "+discordPath, "wait 12s",
		"wait 24s",
		"close",
	)
	wantTunneled := []bool{true, true, false, false}
	for i, w := range h.waits {
		if w.status.DiscordTunneled != wantTunneled[i] {
			t.Errorf("wait %d: DiscordTunneled = %v, want %v", i, w.status.DiscordTunneled, wantTunneled[i])
		}
	}
}

func TestRunFallbackRetriesHandBackAfterLeaveFails(t *testing.T) {
	h := newHarness(t, Options{DiscordPath: discordPath},
		step{drop: true}, fail, fail, fail, fail)
	h.leaveErrs = []error{errLeave}
	h.run()

	h.wantEvents(
		"start#1", "enter#1 "+discordPath, "stop#1",
		"wait 3s", "wait 6s",
		"leave "+discordPath, "wait 12s",
		"leave "+discordPath, "wait 24s",
		"close",
	)
	wantTunneled := []bool{true, true, true, false}
	for i, w := range h.waits {
		if w.status.DiscordTunneled != wantTunneled[i] {
			t.Errorf("wait %d: DiscordTunneled = %v, want %v", i, w.status.DiscordTunneled, wantTunneled[i])
		}
	}
}

func TestRunReconnectAfterFallbackTunnelsDiscordAgain(t *testing.T) {
	h := newHarness(t, Options{DiscordPath: discordPath},
		step{drop: true}, fail, fail, fail, step{})
	h.run()

	h.wantEvents(
		"start#1", "enter#1 "+discordPath, "stop#1",
		"wait 3s", "wait 6s",
		"leave "+discordPath, "wait 12s",
		"start#2", "enter#2 "+discordPath,
		"stop#2", "close",
	)
	if len(h.probes) < 2 {
		t.Fatalf("got %d health probes, want at least 2", len(h.probes))
	}
	if got := h.probeTunnels[1]; got != 2 {
		t.Errorf("tunnel up while reconnected = #%d, want #2", got)
	}
	st := h.probes[1]
	if st.State != StateHealthy || st.Failures != 0 || st.LastError != nil || !st.DiscordTunneled || st.Attempt != 3 {
		t.Errorf("reconnected status = {State: %v, Failures: %d, LastError: %v, DiscordTunneled: %v, Attempt: %d}, want {%v, 0, <nil>, true, 3}",
			st.State, st.Failures, st.LastError, st.DiscordTunneled, st.Attempt, StateHealthy)
	}
	if final := h.sup.Status(); final.State != StateStopped {
		t.Errorf("final state = %v, want %v", final.State, StateStopped)
	}
}

func TestRunReconnectReusesTransportAndEntersDiscordAgain(t *testing.T) {
	h := newHarness(t, Options{DiscordPath: discordPath}, step{drop: true}, step{})
	h.run()

	h.wantEvents(
		"start#1", "enter#1 "+discordPath, "stop#1",
		"start#2", "enter#2 "+discordPath,
		"stop#2", "close",
	)
	if h.transports != 1 {
		t.Errorf("created %d transports, want 1 for the whole run", h.transports)
	}
	if len(h.probes) < 2 || !h.probes[1].DiscordTunneled {
		t.Errorf("health probes = %d, want a second one with DiscordTunneled", len(h.probes))
	}
}

func TestRunReconnectReportsDiscordOutsideWhenEnterFails(t *testing.T) {
	h := newHarness(t, Options{DiscordPath: discordPath},
		step{drop: true}, step{enterErr: errors.New("discord exited")})
	h.run()

	if len(h.probes) < 2 || h.probes[1].DiscordTunneled {
		t.Errorf("health probes = %d, want a second one without DiscordTunneled", len(h.probes))
	}
}

func TestRunDropRestartsBackoff(t *testing.T) {
	h := newHarness(t, Options{}, fail, fail, step{drop: true}, fail, fail)
	h.run()

	h.wantEvents("wait 3s", "wait 6s", "start#1", "enter#1 ", "stop#1", "wait 3s", "wait 6s", "close")
	st := h.waits[2].status
	if st.State != StateDegraded || st.Failures != 1 || !errors.Is(st.LastError, errConnect) {
		t.Errorf("status after drop = {State: %v, Failures: %d, LastError: %v}, want {%v, 1, %v}",
			st.State, st.Failures, st.LastError, StateDegraded, errConnect)
	}
}

func TestRunStopsTunnelOnlyWhenReadyFails(t *testing.T) {
	h := newHarness(t, Options{}, step{startErr: errStart}, step{readyErr: errReady})
	h.run()

	h.wantEvents("start#1", "wait 3s", "start#2", "stop#2", "wait 6s", "close")
	if !errors.Is(h.waits[0].status.LastError, errStart) || !errors.Is(h.waits[1].status.LastError, errReady) {
		t.Errorf("LastError = %v, %v; want %v, %v", h.waits[0].status.LastError, h.waits[1].status.LastError, errStart, errReady)
	}
}

func TestSleepCtx(t *testing.T) {
	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if alive, nudged := sleepCtx(ctx, time.Hour, nil); alive || nudged {
			t.Errorf("sleepCtx = (%v, %v), want (false, false)", alive, nudged)
		}
	})
	t.Run("nudged", func(t *testing.T) {
		nudge := make(chan struct{}, 1)
		nudge <- struct{}{}
		if alive, nudged := sleepCtx(context.Background(), time.Hour, nudge); !alive || !nudged {
			t.Errorf("sleepCtx = (%v, %v), want (true, true)", alive, nudged)
		}
	})
	t.Run("elapsed", func(t *testing.T) {
		if alive, nudged := sleepCtx(context.Background(), 0, make(chan struct{})); !alive || nudged {
			t.Errorf("sleepCtx = (%v, %v), want (true, false)", alive, nudged)
		}
	})
}

func TestRunLeavesDiscordAloneWhenNoTransportWasCreated(t *testing.T) {
	h := newHarness(t, Options{MaxFailures: 1}, fail, fail)
	h.run()

	h.wantEvents("wait 3s", "wait 6s")
	if h.transports != 0 {
		t.Errorf("created %d transports, want none before a profile opens", h.transports)
	}
}
