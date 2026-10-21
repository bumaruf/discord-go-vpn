// Package ui draws the screen: an animated startup checklist, then the live
// dashboard of the tunnel. It only reads the supervisor's Status and Reading;
// its requests are Reconnect and a new token.
package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/bumaruf/discord-go-vpn/internal/errtext"
	"github.com/bumaruf/discord-go-vpn/internal/startup"
	"github.com/bumaruf/discord-go-vpn/internal/supervisor"
)

// ApplyToken takes what was typed in the token field. It returns an error the
// person reads, and nothing else: whether the server accepts the token shows up
// in the next attempt's status.
type ApplyToken func(typed string) error

// Session is what the screen draws once startup is through.
type Session struct {
	Sup     *supervisor.Supervisor
	Profile string
	// ApplyToken is nil when access comes from a .conf.
	ApplyToken ApplyToken
	// TrayHint says whether to mention the tray, which only exists on Windows.
	TrayHint bool
	TrayErr  error // why there is no tray icon

	// Restart is the version startup installed: the screen says so and quits,
	// and the caller relaunches it.
	Restart string
}

// Boot runs startup, reporting its steps through r, and starts the tunnel.
type Boot func(r startup.Reporter) (Session, error)

type Options struct {
	Version string
	Boot    Boot
	// Stop asks the tunnel to come down. The screen stays, with the mascot
	// asleep, until the supervisor closes its updates.
	Stop func()
}

type phase int

const (
	phaseBoot phase = iota
	phaseDashboard
)

type Model struct {
	ctx     context.Context
	opts    Options
	events  chan tea.Msg // from the boot goroutine
	answers chan string  // tokens typed while booting

	phase phase
	steps [stepCount]stepView
	// bootErr ends startup for good; only quitting is left.
	bootErr error
	// handoverAt is the frame the dashboard takes over; zero until scheduled.
	handoverAt int

	sess    Session
	status  supervisor.Status
	reading supervisor.Reading
	hist    history
	chart   chartState
	// metered sticks once the transport reported traffic, so a reconnection
	// doesn't flip the chart to RTT for a moment.
	metered bool

	frame int
	theme theme

	// asking is set while startup waits for a typed token; tell is why.
	asking bool
	tell   string
	// tokenField is typed by hand rather than with bubbles' textinput, which
	// pulls in a clipboard dependency that shells out on Linux.
	tokenField []rune
	tokenError string

	notice      string
	noticeUntil time.Time

	width, height int
	stopping      bool
}

func New(ctx context.Context, opts Options) Model {
	m := Model{
		ctx:     ctx,
		opts:    opts,
		events:  make(chan tea.Msg, 64),
		answers: make(chan string, 1),
		theme:   newTheme(true),
		chart:   chartState{back: -1},
	}
	for i := range m.steps {
		m.steps[i].state = stepPending
	}
	return m
}

type (
	frameMsg   struct{}
	sampleMsg  time.Time
	statusMsg  supervisor.Status
	sessionMsg struct {
		sess Session
		err  error
	}
	hoverMsg struct {
		cell int
		in   bool
	}
	forceQuitMsg struct{}
)

const (
	frameEvery  = 100 * time.Millisecond
	sampleEvery = time.Second

	noticeDuration = 4 * time.Second
	// celebration is how long, in frames, the finished checklist stays up.
	celebration = 18
	// stopTimeout bounds the wait for the tunnel to come down after q.
	stopTimeout = 15 * time.Second
)

func (m Model) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, m.runBoot(), m.waitEvent(), frameTick(), sampleTick())
}

func frameTick() tea.Cmd {
	return tea.Tick(frameEvery, func(time.Time) tea.Msg { return frameMsg{} })
}

func sampleTick() tea.Cmd {
	return tea.Tick(sampleEvery, func(t time.Time) tea.Msg { return sampleMsg(t) })
}

// runBoot starts startup only once the program runs, so a terminal that can't
// draw the screen leaves nothing half done.
func (m Model) runBoot() tea.Cmd {
	if m.opts.Boot == nil {
		return nil
	}
	return func() tea.Msg {
		r := reporter{events: m.events, answers: m.answers, done: m.ctx.Done()}
		sess, err := m.opts.Boot(r)
		r.send(sessionMsg{sess, err})
		return nil
	}
}

func (m Model) waitEvent() tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-m.events:
			return msg
		case <-m.ctx.Done():
			return nil
		}
	}
}

// waitStatus blocks on the supervisor channel; each arrival schedules the next.
func waitStatus(sup *supervisor.Supervisor) tea.Cmd {
	return func() tea.Msg {
		s, ok := <-sup.Updates()
		if !ok {
			return tea.Quit()
		}
		return statusMsg(s)
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.BackgroundColorMsg:
		m.theme = newTheme(msg.IsDark())
		return m, nil

	case frameMsg:
		m.frame++
		if m.handoverAt > 0 && m.frame >= m.handoverAt {
			m.handoverAt = 0
			if m.sess.Restart != "" {
				return m, tea.Quit
			}
			m.phase = phaseDashboard
		}
		return m, frameTick()

	case sampleMsg:
		if m.sess.Sup != nil {
			m.reading = m.sess.Sup.Reading()
			m.metered = m.metered || m.reading.Metered
			m.hist.add(m.reading, time.Time(msg))
		}
		if !m.noticeUntil.IsZero() && time.Time(msg).After(m.noticeUntil) {
			m.notice, m.noticeUntil = "", time.Time{}
		}
		return m, sampleTick()

	case stepMsg:
		m.steps[msg.step] = stepView{state: msg.state, detail: msg.detail, fraction: msg.fraction}
		return m, m.waitEvent()

	case askMsg:
		m.asking = true
		return m, m.waitEvent()

	case tellMsg:
		m.tell = string(msg)
		return m, m.waitEvent()

	case sessionMsg:
		return m.handOver(msg)

	case statusMsg:
		m.status = supervisor.Status(msg)
		m.followTunnel()
		return m, waitStatus(m.sess.Sup)

	case hoverMsg:
		if msg.in {
			m.chart.back, m.chart.mouse = msg.cell, true
		} else if m.chart.mouse {
			m.chart.back, m.chart.mouse = -1, false
		}
		return m, nil

	case tea.MouseWheelMsg:
		if m.phase == phaseDashboard {
			m.zoom(msg.Button == tea.MouseWheelUp)
		}
		return m, nil

	case forceQuitMsg:
		return m, tea.Quit

	case tea.PasteMsg:
		if m.typing() {
			m.typeText(msg.Content)
		}
		return m, nil

	case tea.KeyPressMsg:
		return m.press(msg)
	}
	return m, nil
}

// handOver takes the session startup built and starts following the tunnel.
func (m Model) handOver(msg sessionMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.bootErr = msg.err
		return m, nil
	}
	m.sess = msg.sess
	if m.sess.Restart != "" {
		m.handoverAt = m.frame + celebration
		return m, nil
	}
	m.status = m.sess.Sup.Status()
	m.followTunnel()
	return m, waitStatus(m.sess.Sup)
}

// followTunnel moves the checklist's last two steps with the status, and hands
// over to the dashboard once the first attempt ends either way.
func (m *Model) followTunnel() {
	st := m.status
	tunnel, discord := &m.steps[stepTunnel], &m.steps[stepDiscord]
	switch {
	case st.State == supervisor.StateStarting && !st.OpeningDiscord:
		detail := st.Progress
		if detail == "" {
			detail = "subindo o WireGuard…"
		}
		*tunnel = stepView{state: stepRunning, detail: detail}
	case st.OpeningDiscord:
		*tunnel = stepView{state: stepDone, detail: "conectado a " + orDash(st.Endpoint)}
		*discord = stepView{state: stepRunning, detail: "reabrindo o Discord dentro do túnel…"}
	case st.State == supervisor.StateHealthy:
		*tunnel = stepView{state: stepDone, detail: "conectado a " + orDash(st.Endpoint)}
		if st.DiscordTunneled {
			*discord = stepView{state: stepDone, detail: "aberto dentro do túnel"}
		} else {
			*discord = stepView{state: stepFailed, detail: "não consegui reabrir o Discord"}
		}
	case st.State == supervisor.StateDegraded || st.State == supervisor.StateFallback:
		*tunnel = stepView{state: stepFailed, detail: errtext.Translate(st.LastError)}
	}

	if m.phase == phaseBoot && m.handoverAt == 0 &&
		(st.State == supervisor.StateHealthy || st.State == supervisor.StateDegraded || st.State == supervisor.StateFallback) {
		m.handoverAt = m.frame + celebration
	}
}

func (m Model) press(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m.quit()
	}
	if m.typing() {
		return m.typeKey(msg)
	}
	if m.stopping {
		if key == "q" || key == "esc" {
			return m, tea.Quit
		}
		return m, nil
	}

	if m.phase == phaseBoot {
		switch {
		case key == "q" || key == "esc":
			return m.quit()
		case m.handoverAt > 0 && m.sess.Restart == "":
			m.handoverAt = m.frame // any key skips the celebration
		}
		return m, nil
	}

	switch key {
	case "q":
		return m.quit()
	case "esc":
		if m.chart.back >= 0 {
			m.chart.back, m.chart.mouse = -1, false
			return m, nil
		}
		return m.quit()
	case "r":
		// Only while the supervisor waits to retry, the same states in which
		// the footer offers the key.
		if m.retrying() {
			m.sess.Sup.Reconnect()
			m.notice = "tentando agora…"
			m.noticeUntil = time.Now().Add(noticeDuration)
		}
	case "tab":
		if m.hasTraffic() {
			m.chart.mode = 1 - m.chart.mode
		}
	case "z":
		m.zoom(true)
	case "left", "h":
		m.chart.back, m.chart.mouse = min(m.chart.back+1, maxSamples), false
	case "right", "l":
		m.chart.back = max(m.chart.back-1, -1)
	case "end":
		m.chart.back = -1
	}
	return m, nil
}

// zoom widens the chart window, wrapping around, or narrows it.
func (m *Model) zoom(wider bool) {
	n := len(chartWindows)
	if wider {
		m.chart.window = (m.chart.window + 1) % n
	} else {
		m.chart.window = (m.chart.window + n - 1) % n
	}
}

// quit takes the tunnel down with the screen still up: putting Discord back on
// the normal network takes a moment, and an empty terminal meanwhile looks hung.
func (m Model) quit() (tea.Model, tea.Cmd) {
	if m.opts.Stop != nil {
		m.opts.Stop()
	}
	if m.sess.Sup == nil || m.stopping {
		return m, tea.Quit
	}
	m.stopping = true
	return m, tea.Tick(stopTimeout, func(time.Time) tea.Msg { return forceQuitMsg{} })
}

func (m Model) retrying() bool {
	return m.status.State == supervisor.StateDegraded || m.status.State == supervisor.StateFallback
}

func (m Model) hasTraffic() bool { return m.metered }

// asksToken reports whether the dashboard's token field is on screen, which
// also takes the keyboard: while it is, typed letters are the token, not commands.
func (m Model) asksToken() bool {
	return m.phase == phaseDashboard && m.sess.ApplyToken != nil && m.status.TokenAsk != supervisor.TokenFine
}

func (m Model) typing() bool { return (m.phase == phaseBoot && m.asking) || m.asksToken() }

// maxTokenField caps what the field holds, well above a token's length: a
// paste of a whole file must not grow the screen without end.
const maxTokenField = 512

// typeKey owns the keyboard while the field is on screen: esc still quits,
// enter hands the token over, and everything else is typing.
func (m Model) typeKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m.quit()
	case "enter":
		return m.submitToken()
	case "backspace":
		if len(m.tokenField) > 0 {
			m.tokenField = m.tokenField[:len(m.tokenField)-1]
		}
	case "ctrl+u":
		m.tokenField = nil
	default:
		m.typeText(msg.Text)
	}
	return m, nil
}

// typeText drops spaces: a pasted token sometimes brings one, and none belongs
// inside a token.
func (m *Model) typeText(text string) {
	runes := []rune(strings.Join(strings.Fields(text), ""))
	room := maxTokenField - len(m.tokenField)
	m.tokenField = append(m.tokenField, runes[:min(len(runes), room)]...)
}

func (m Model) submitToken() (tea.Model, tea.Cmd) {
	typed := strings.TrimSpace(string(m.tokenField))
	if typed == "" {
		return m, nil
	}
	if m.phase == phaseBoot {
		// Startup validates it and tells why if it fails.
		m.answers <- typed
		m.asking, m.tell, m.tokenField = false, "", nil
		return m, nil
	}
	if err := m.sess.ApplyToken(typed); err != nil {
		m.tokenError = errtext.Translate(err)
		return m, nil
	}
	// The supervisor clears TokenAsk, which takes the field off the screen.
	m.tokenField, m.tokenError = nil, ""
	return m, nil
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// reporter carries startup's steps to the screen from the boot goroutine.
type reporter struct {
	events  chan<- tea.Msg
	answers <-chan string
	done    <-chan struct{}
}

type (
	stepMsg struct {
		step     stepID
		state    stepState
		detail   string
		fraction float64
	}
	askMsg  struct{}
	tellMsg string
)

func (r reporter) send(msg tea.Msg) {
	select {
	case r.events <- msg:
	case <-r.done:
	}
}

func (r reporter) Step(step startup.Step, state startup.State, detail string, fraction float64) {
	r.send(stepMsg{step: stepFor(step), state: stateFor(state), detail: detail, fraction: fraction})
}

func (r reporter) Tell(msg string) { r.send(tellMsg(msg)) }

func (r reporter) Ask() (string, error) {
	r.send(askMsg{})
	select {
	case typed := <-r.answers:
		return typed, nil
	case <-r.done:
		return "", context.Canceled
	}
}
