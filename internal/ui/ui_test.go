package ui

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/bumaruf/discord-go-vpn/internal/startup"
	"github.com/bumaruf/discord-go-vpn/internal/supervisor"
	"github.com/bumaruf/discord-go-vpn/internal/wg"
)

func dashboard(st supervisor.Status) Model {
	m := New(context.Background(), Options{Version: "v1.4.0"})
	m.phase = phaseDashboard
	m.status = st
	m.sess = Session{Profile: "acesso do servidor"}
	m.width = 84
	return m
}

// metered fills five minutes of traffic, a call's worth of video.
func metered(m Model) Model {
	start := time.Now().Add(-5 * time.Minute)
	for i := range 300 {
		rx := uint64(i) * 250_000 * uint64(1+i%7)
		m.hist.add(supervisor.Reading{Metered: true, HasTraffic: true, RTT: time.Duration(20+i%30) * time.Millisecond,
			Traffic: wg.Traffic{RxBytes: rx, TxBytes: rx / 8}}, start.Add(time.Duration(i)*time.Second))
	}
	m.metered = true
	m.reading = supervisor.Reading{Metered: true, HasTraffic: true, RTT: 23 * time.Millisecond,
		Traffic: wg.Traffic{RxBytes: 340 << 20, TxBytes: 22 << 20, LastHandshake: time.Now().Add(-42 * time.Second)}}
	return m
}

func healthy() supervisor.Status {
	return supervisor.Status{
		State: supervisor.StateHealthy, Since: time.Now().Add(-12 * time.Minute), DiscordTunneled: true,
		Endpoint: "203.0.113.10:51820", Address: "10.9.0.12",
	}
}

func screenOf(m Model) string { return ansi.Strip(m.View().Content) }

func assertFits(t *testing.T, screen string, width int) {
	t.Helper()
	for _, line := range strings.Split(screen, "\n") {
		if n := lipgloss.Width(line); n > width {
			t.Errorf("line with %d columns overflows a %d-column window: %q", n, width, line)
		}
	}
}

// words joins the screen's words across rows and card borders, for text that
// may wrap.
func words(screen string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(screen, "│", " ")), " ")
}

// TestScreens renders the states people see; run with -v to inspect them.
func TestScreens(t *testing.T) {
	now := time.Now()
	reconnecting := supervisor.Status{
		State: supervisor.StateDegraded, Attempt: 3, Since: now,
		LastError: errors.New("o namespace não chegou a subir"), LastErrorAt: now, DiscordTunneled: true,
	}
	cases := []struct {
		name  string
		model Model
	}{
		{"connected", metered(dashboard(healthy()))},
		{"connected on WireSock", func() Model {
			m := dashboard(healthy())
			m.reading = supervisor.Reading{RTT: 31 * time.Millisecond}
			return m
		}()},
		{"reconnecting", metered(dashboard(reconnecting))},
		{"fallback", dashboard(supervisor.Status{State: supervisor.StateFallback, Attempt: 4, Since: now,
			LastError: errors.New("o servidor não respondeu"), LastErrorAt: now})},
		{"stopping", func() Model { m := metered(dashboard(healthy())); m.stopping = true; return m }()},
		{"narrow", func() Model { m := metered(dashboard(healthy())); m.width = 60; return m }()},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, height := range []int{0, 40, 24, 14} {
				m := c.model
				m.height = height
				screen := screenOf(m)
				t.Logf("height %d:\n%s", height, screen)

				if strings.Contains(screen, "%!") {
					t.Fatalf("a formatting verb was left on screen:\n%s", screen)
				}
				assertFits(t, screen, m.width)
				if height > 0 && strings.Count(screen, "\n")+1 > height {
					t.Errorf("the screen took %d lines in a %d-line window", strings.Count(screen, "\n")+1, height)
				}
			}
		})
	}

	if strings.Contains(screenOf(cases[0].model), "tentar agora") {
		t.Error("the footer offers to retry while the tunnel is up")
	}
	if !strings.Contains(screenOf(cases[2].model), "tentar agora") {
		t.Error("the footer must offer to retry while waiting for the backoff")
	}
	if !strings.Contains(screenOf(cases[1].model), "WireSock não informa") {
		t.Error("a transport without counters must say why there is no traffic")
	}
}

func TestBootScreens(t *testing.T) {
	m := New(context.Background(), Options{Version: "v1.4.0"})
	m.width = 84
	m.steps[stepUpdate] = stepView{state: stepRunning, detail: "baixando v1.5.0", fraction: 0.62}
	m.steps[stepAccess] = stepView{state: stepPending}
	screen := screenOf(m)
	t.Logf("\n%s", screen)
	for _, want := range []string{"atualização", "baixando v1.5.0", "62%", "acesso", "túnel", "Discord"} {
		if !strings.Contains(screen, want) {
			t.Errorf("boot screen lacks %q", want)
		}
	}
	assertFits(t, screen, m.width)
	if bar := m.View().ProgressBar; bar == nil || bar.Value != 62 {
		t.Errorf("terminal progress bar = %+v, want the download's 62%%", bar)
	}

	m.steps[stepUpdate] = stepView{state: stepDone, detail: "em dia (v1.4.0)", fraction: startup.NoFraction}
	m.asking, m.tell = true, "Esse token não foi aceito: o acesso pode ter sido revogado."
	screen = screenOf(m)
	t.Logf("\n%s", screen)
	for _, want := range []string{"Cole o token", "revogado", "dgv1_", "enter"} {
		if !strings.Contains(screen, want) {
			t.Errorf("token prompt lacks %q", want)
		}
	}
}

func TestBootHandsOverOnceTheFirstAttemptEnds(t *testing.T) {
	m := New(context.Background(), Options{Version: "v1.4.0"})
	m.status = supervisor.Status{State: supervisor.StateStarting, Progress: "WireSock não encontrado, instalando…"}
	m.followTunnel()
	if s := m.steps[stepTunnel]; s.state != stepRunning || s.detail != m.status.Progress {
		t.Fatalf("tunnel step = %+v, want it running with the transport's progress", s)
	}

	m.status = healthy()
	m.followTunnel()
	if m.steps[stepDiscord].state != stepDone || m.handoverAt == 0 {
		t.Fatalf("a healthy tunnel must finish the checklist and schedule the dashboard: %+v", m.steps)
	}
	if !strings.Contains(screenOf(m), "tudo pronto") {
		t.Errorf("the finished checklist doesn't celebrate:\n%s", screenOf(m))
	}

	for m.phase == phaseBoot {
		next, _ := m.Update(frameMsg{})
		m = next.(Model)
	}
	if m.frame > celebration+1 {
		t.Errorf("the dashboard took %d frames to take over", m.frame)
	}
}

func TestBootFailureStaysOnScreen(t *testing.T) {
	m := New(context.Background(), Options{Version: "v1.4.0"})
	next, _ := m.Update(sessionMsg{err: errors.New("lendo wg0.conf: falta PrivateKey")})
	m = next.(Model)
	screen := screenOf(m)
	if !strings.Contains(screen, "falta PrivateKey") || m.phase != phaseBoot {
		t.Errorf("a failed startup must stay on the checklist with its error:\n%s", screen)
	}
}

func TestViewShowsTransportReportsAndTrayFailure(t *testing.T) {
	m := dashboard(supervisor.Status{
		State:   supervisor.StateHealthy,
		Warning: "não consegui amarrar o WireSock a este processo (acesso negado)",
	})
	m.sess.TrayErr = errors.New("sem área de notificação")

	screen := screenOf(m)
	t.Logf("\n%s", screen)
	for _, want := range []string{m.status.Warning, "bandeja indisponível: sem área de notificação"} {
		if !strings.Contains(screen, want) {
			t.Errorf("screen lacks %q", want)
		}
	}
	assertFits(t, screen, m.width)
}

func TestViewShowsEveryLineOfAMultiLineError(t *testing.T) {
	const instruction = `clique com o botão direito no .exe e escolha "Executar como administrador"`
	m := dashboard(supervisor.Status{
		State: supervisor.StateDegraded, Attempt: 1, LastErrorAt: time.Now(),
		LastError: errors.New("o WireSock instala um filtro no kernel e precisa de administrador.\n  " + instruction),
	})

	screen := screenOf(m)
	t.Logf("\n%s", screen)
	for _, want := range []string{"precisa de administrador.", instruction} {
		if !strings.Contains(words(screen), want) {
			t.Errorf("screen lacks %q", want)
		}
	}
	assertFits(t, screen, m.width)
}

func TestViewWrapsALongError(t *testing.T) {
	const cause = "a conexão foi recusada (tcp 203.0.113.10:443)"
	m := dashboard(supervisor.Status{
		State: supervisor.StateDegraded, Attempt: 1, LastErrorAt: time.Now(),
		LastError: errors.New("não consegui obter um acesso: não consegui falar com o servidor de acesso: " + cause),
	})
	m.width = 60

	screen := screenOf(m)
	if !strings.Contains(words(screen), cause) {
		t.Errorf("the end of a long error was cut off:\n%s", screen)
	}
	assertFits(t, screen, m.width)
}

func TestViewTranslatesRawErrors(t *testing.T) {
	m := dashboard(supervisor.Status{
		State: supervisor.StateDegraded, Attempt: 1, LastErrorAt: time.Now(),
		LastError: fmt.Errorf("não consegui falar com o servidor de acesso: %w",
			&net.OpError{Op: "dial", Net: "tcp", Addr: &net.TCPAddr{IP: net.IPv4(203, 0, 113, 10), Port: 443},
				Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED}}),
	})
	m.sess.TrayErr = &os.SyscallError{Syscall: "Shell_NotifyIcon", Err: syscall.EACCES}

	screen := words(screenOf(m))
	for _, want := range []string{
		"não consegui falar com o servidor de acesso: a conexão foi recusada (tcp 203.0.113.10:443)",
		"bandeja indisponível: permissão negada",
	} {
		if !strings.Contains(screen, want) {
			t.Errorf("screen lacks %q:\n%s", want, screen)
		}
	}
}

func TestShortWindowCutsLeastImportantFirst(t *testing.T) {
	const instruction = "clique com o botão direito no .exe"
	base := dashboard(supervisor.Status{
		State: supervisor.StateDegraded, Attempt: 1, LastErrorAt: time.Now(),
		LastError: errors.New("o WireSock precisa de administrador.\n  " + instruction),
	})

	cases := []struct {
		name                                        string
		height                                      int
		wantChart, wantMascot, wantProfile, wantAll bool
	}{
		{"room for everything", 40, true, true, true, true},
		{"the chart goes first", 20, false, true, true, true},
		{"then the mascot and the profile", 13, false, false, false, true},
		{"the error folds last", 6, false, false, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := base
			m.height = c.height
			screen := screenOf(m)
			t.Logf("\n%s", screen)

			if !strings.Contains(screen, "precisa de administrador") || !strings.Contains(screen, "reconectando") {
				t.Error("the state and the error's first line must never be cut")
			}
			checks := []struct {
				what string
				got  bool
				want bool
			}{
				{"chart", strings.Contains(screen, "há 1 min"), c.wantChart},
				{"mascot", strings.Contains(screen, "▀"), c.wantMascot},
				{"profile", strings.Contains(screen, "acesso do servidor"), c.wantProfile},
				{"instruction", strings.Contains(screen, instruction), c.wantAll},
			}
			for _, ck := range checks {
				if ck.got != ck.want {
					t.Errorf("%s on screen = %v, want %v", ck.what, ck.got, ck.want)
				}
			}
		})
	}
}

func TestScreenRespectsMaxWidth(t *testing.T) {
	m := metered(dashboard(healthy()))
	m.width = 240
	for _, line := range strings.Split(screenOf(m), "\n") {
		if n := lipgloss.Width(line); n > maxWidth {
			t.Fatalf("a 240-column window drew a %d-column line, over the cap of %d", n, maxWidth)
		}
	}
}

func TestChartInspectsWithKeysAndMouse(t *testing.T) {
	m := metered(dashboard(healthy()))
	m.height = 40

	m = pressKey(m, "left")
	m = pressKey(m, "left")
	if m.chart.back != 1 {
		t.Fatalf("two lefts put the cursor %d cells back, want 1", m.chart.back)
	}
	if !strings.Contains(screenOf(m), "▸ ") {
		t.Errorf("the readout doesn't follow the cursor:\n%s", screenOf(m))
	}

	m = pressKey(m, "esc")
	if m.chart.back != -1 || m.stopping {
		t.Fatal("esc must leave inspection before it quits")
	}

	// The mouse handler maps a cell of the plot to the cursor.
	v := m.View()
	_, geom := m.screen()
	cmd := v.OnMouse(tea.MouseMotionMsg{X: geom.x + geom.w - 3, Y: geom.y + 1})
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.chart.back != 2 || !m.chart.mouse {
		t.Fatalf("hovering 3 cells from the right set back=%d mouse=%v", m.chart.back, m.chart.mouse)
	}
	cmd = v.OnMouse(tea.MouseMotionMsg{X: 0, Y: 0})
	next, _ = m.Update(cmd())
	if m = next.(Model); m.chart.back != -1 {
		t.Error("leaving the plot must stop the mouse's inspection")
	}

	m = pressKey(m, "tab")
	if m.chart.mode != chartRTT || !strings.Contains(screenOf(m), "RTT 49 ms") {
		t.Errorf("tab must switch the chart to RTT:\n%s", screenOf(m))
	}
	m = pressKey(m, "z")
	if chartWindows[m.chart.window] != 5*60 {
		t.Error("z must widen the window")
	}
}

func TestQuitKeepsTheScreenUntilTheTunnelIsDown(t *testing.T) {
	stopped := false
	m := dashboard(healthy())
	m.opts.Stop = func() { stopped = true }
	m.sess.Sup = supervisor.New(supervisor.Options{})

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = next.(Model)
	if !stopped || !m.stopping || cmd == nil {
		t.Fatal("q must stop the tunnel and keep the screen up meanwhile")
	}
	if !strings.Contains(screenOf(m), "fechando o túnel") {
		t.Errorf("the stopping screen doesn't say what it waits for:\n%s", screenOf(m))
	}
	if _, cmd = m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"}); !isQuit(cmd) {
		t.Error("a second q must quit at once")
	}
}

func pressKey(m Model, key string) Model {
	var msg tea.KeyPressMsg
	switch key {
	case "left":
		msg = tea.KeyPressMsg{Code: tea.KeyLeft}
	case "esc":
		msg = tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		msg = tea.KeyPressMsg{Code: tea.KeyTab}
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "backspace":
		msg = tea.KeyPressMsg{Code: tea.KeyBackspace}
	default:
		r := []rune(key)[0]
		msg = tea.KeyPressMsg{Code: r, Text: key}
	}
	next, _ := m.Update(msg)
	return next.(Model)
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestWrap(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		width int
		want  []string
	}{
		{"fits", "curto", 10, []string{"curto"}},
		{"breaks at spaces", "um dois três", 7, []string{"um dois", "três"}},
		{"keeps each line's indentation", "erro:\n  faça isto agora", 10, []string{"erro:", "  faça", "  isto", "  agora"}},
		{"splits a word wider than the row", "abcdefghij", 4, []string{"abcd", "efgh", "ij"}},
		{"caps an indentation wider than the row", "      ab", 4, []string{"   a", "   b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := wrap(c.text, c.width); !reflect.DeepEqual(got, c.want) {
				t.Errorf("wrap(%q, %d) = %q, want %q", c.text, c.width, got, c.want)
			}
		})
	}
}

func TestTruncateKeepsAccents(t *testing.T) {
	if got := truncate("conexão recusada", 9); got != "conexão …" {
		t.Fatalf("truncate = %q", got)
	}
	if truncate("curto", 40) != "curto" {
		t.Fatal("text that fits must be left alone")
	}
}

func TestFormats(t *testing.T) {
	cases := []struct{ got, want string }{
		{formatRate(0), "0 kbps"},
		{formatRate(1000), "8,0 kbps"},
		{formatRate(124_995), "1,0 Mbps"},
		{formatRate(150_000), "1,2 Mbps"},
		{formatRate(25_000_000), "200 Mbps"},
		{formatVolume(0), "0 B"},
		{formatVolume(512), "512 B"},
		{formatVolume(340 << 20), "340 MB"},
		{formatVolume(22<<20 + 100<<10), "22,1 MB"},
		{formatRTT(0), "—"},
		{formatRTT(23 * time.Millisecond), "23 ms"},
		{windowName(300), "5 min"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}

func TestHistoryTurnsCountersIntoRates(t *testing.T) {
	var h history
	t0 := time.Now()
	h.add(supervisor.Reading{HasTraffic: true, Traffic: wg.Traffic{RxBytes: 1000, TxBytes: 100}}, t0)
	h.add(supervisor.Reading{HasTraffic: true, Traffic: wg.Traffic{RxBytes: 3000, TxBytes: 600}}, t0.Add(time.Second))
	// A new session restarts the counters, which must not read as a huge drop.
	h.add(supervisor.Reading{HasTraffic: true, Traffic: wg.Traffic{RxBytes: 10, TxBytes: 10}}, t0.Add(2*time.Second))

	s := h.window(3)
	if s[0].traffic || s[1].down != 2000 || s[1].up != 500 || s[2].traffic {
		t.Fatalf("samples = %+v", s)
	}
}

func TestChartAreaAndBuckets(t *testing.T) {
	samples := make([]sample, 4)
	samples[3] = sample{traffic: true, down: 100}
	s := bucket(samples, 2, func(sm sample) (float64, bool) { return sm.down, sm.traffic })
	if s.known[0] || !s.known[1] || s.values[1] != 100 {
		t.Fatalf("bucket = %+v", s)
	}

	// Full value fills every dot of the column, growing up from the bottom.
	cells := area(series{values: []float64{0, 100}, known: []bool{false, true}}, 100, 1, 2, true)
	if cells[0][0] != 0x08|0x10|0x20|0x80 || cells[1][0] != 0x08|0x10|0x20|0x80 {
		t.Fatalf("cells = %x", cells)
	}
	if got := niceCeil(3.3); math.Abs(got-5) > 1e-9 {
		t.Errorf("niceCeil(3.3) = %v, want 5", got)
	}
}

func TestMascotKeepsItsSize(t *testing.T) {
	th := newTheme(true)
	for md := moodBooting; md <= moodAsleep; md++ {
		for frame := range 130 {
			p, parts := animate(md, frame, th)
			out := ansi.Strip(renderMascot(p, parts))
			rows := strings.Split(out, "\n")
			if len(rows) != mascotRows {
				t.Fatalf("mood %d frame %d: %d rows", md, frame, len(rows))
			}
			for _, r := range rows {
				if lipgloss.Width(r) != mascotWidth {
					t.Fatalf("mood %d frame %d: row %q is %d wide", md, frame, r, lipgloss.Width(r))
				}
			}
		}
	}
	t.Logf("\n%s", ansi.Strip(renderMascot(animate(moodHappy, 3, th))))
}
