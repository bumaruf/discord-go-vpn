package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/bumaruf/discord-go-vpn/internal/errtext"
	"github.com/bumaruf/discord-go-vpn/internal/statustext"
	"github.com/bumaruf/discord-go-vpn/internal/supervisor"
)

// Layout sizes, in terminal columns.
const (
	// maxWidth caps the screen, so a wide terminal does not strand values far
	// from their labels.
	maxWidth     = 110
	defaultWidth = 84
	// sideBySide is the narrowest screen that fits the two cards in one row.
	sideBySide = 72
	// cardChrome is a card's border and padding, on each side.
	cardChrome = 2
	labelWidth = 12

	defaultChartRows = 12
	maxErrorRows     = 6
)

// cutOrder lists what a short window drops, in order, once the chart is gone.
var cutOrder = []string{cutMascot, cutProfile, cutCards, cutErrorDetail}

const (
	cutMascot      = "mascote"
	cutProfile     = "perfil"
	cutCards       = "cartões"
	cutErrorDetail = "detalhe do erro"
)

func (m Model) View() tea.View {
	content, geom := m.screen()
	v := tea.NewView(content)
	v.AltScreen = true
	v.WindowTitle = "discord-go-vpn · " + m.stateWord()
	v.ProgressBar = m.taskbar()
	if m.phase == phaseDashboard && geom.w > 0 {
		v.MouseMode = tea.MouseModeAllMotion
		v.OnMouse = func(msg tea.MouseMsg) tea.Cmd {
			switch msg.(type) {
			case tea.MouseMotionMsg, tea.MouseClickMsg:
			default:
				return nil
			}
			mouse := msg.Mouse()
			cell, in := geom.cell(mouse.X, mouse.Y)
			return func() tea.Msg { return hoverMsg{cell: geom.w - 1 - cell, in: in} }
		}
	}
	return v
}

func (m Model) screenWidth() int {
	if m.width <= 0 {
		return defaultWidth
	}
	return min(m.width, maxWidth)
}

// screen drops parts in cutOrder until the screen fits the window. It measures
// the rendered screen instead of computing the space up front, which would
// drift whenever a part of the layout changes.
func (m Model) screen() (string, chartGeom) {
	omit := map[string]bool{}
	for i := 0; ; i++ {
		rows, geom := m.render(omit)
		if m.height <= 0 || len(rows) <= m.height || i >= len(cutOrder) {
			// One column of margin on the left.
			for r := range rows {
				rows[r] = " " + rows[r]
			}
			geom.x++
			return strings.Join(rows, "\n"), geom
		}
		omit[cutOrder[i]] = true
	}
}

func (m Model) render(omit map[string]bool) ([]string, chartGeom) {
	w := m.screenWidth() - 2
	top := []string{""}
	if omit[cutMascot] {
		top = append(top, m.compactHeader(w))
	} else {
		top = append(top, lines(m.header(w))...)
	}
	top = append(top, "")

	if m.phase == phaseBoot {
		top = append(top, m.bootBody(w)...)
		return append(top, m.footer(w)...), chartGeom{}
	}

	if !omit[cutCards] {
		top = append(top, m.cards(w, omit)...)
	}
	top = append(top, m.alerts(w, omit[cutErrorDetail])...)
	bottom := m.footer(w)

	if m.asksToken() {
		top = append(top, "")
		top = append(top, m.tokenPrompt("Cole um token de conexão novo", statustext.TokenAskReason(m.status.TokenAsk), w)...)
		return append(top, bottom...), chartGeom{}
	}

	room := defaultChartRows
	if m.height > 0 {
		room = m.height - len(top) - len(bottom)
	}
	if room < minChartRows {
		return append(top, bottom...), chartGeom{}
	}
	chart, geom := m.chartCard(w, room)
	geom.y += len(top)
	return append(append(top, lines(chart)...), bottom...), geom
}

// header puts the mascot beside the title and three lines about now.
func (m Model) header(width int) string {
	md, info := m.headerInfo()
	text := []string{"", m.title()}
	for _, l := range info {
		text = append(text, fit(l, max(1, width-mascotWidth-2)))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, m.mascot(md), "  ", strings.Join(text, "\n"))
}

func (m Model) compactHeader(width int) string {
	_, info := m.headerInfo()
	return fit(m.title()+"  "+info[0], width)
}

func (m Model) title() string {
	return boldPaint(m.theme.brand, "discord-go-vpn") + " " + paint(m.theme.dim, m.opts.Version)
}

// headerInfo is the mascot's mood and the lines beside it: the state, where
// Discord is, and a remark.
func (m Model) headerInfo() (mood, []string) {
	if m.phase == phaseBoot {
		return m.bootMood(), []string{
			paint(m.theme.dim, "Discord numa VPN só dele"),
			italic(m.theme.dim, m.bootQuip()),
		}
	}
	md := moodHappy
	switch {
	case m.stopping:
		md = moodAsleep
	case m.status.State == supervisor.StateStarting:
		md = moodBooting
	case m.status.State == supervisor.StateDegraded:
		md = moodWorried
	case m.status.State == supervisor.StateFallback:
		md = moodSad
	}
	return md, []string{m.stateLine(), m.discordLine(), italic(m.theme.dim, m.quip(md))}
}

func (m Model) stateLine() string {
	st := m.status
	name := bold(statustext.StateLabel(st.State))
	since := paint(m.theme.dim, " · há "+statustext.Duration(st.Uptime()))
	switch st.State {
	case supervisor.StateHealthy:
		return paint(m.theme.ok, "●") + " " + name + since
	case supervisor.StateStarting:
		return paint(m.theme.brand, spinnerFrames[m.frame%len(spinnerFrames)]) + " " + name
	case supervisor.StateDegraded:
		return paint(m.theme.warn, "◐") + " " + name + paint(m.theme.dim, fmt.Sprintf(" · tentativa %d", st.Attempt))
	case supervisor.StateFallback:
		return paint(m.theme.err, "○") + " " + name + since
	}
	return paint(m.theme.dim, "○ "+statustext.StateLabel(st.State))
}

func (m Model) discordLine() string {
	switch {
	case m.stopping:
		return paint(m.theme.dim, "Discord voltando para a rede normal")
	case m.status.DiscordTunneled:
		return paint(m.theme.ok, "Discord dentro do túnel")
	case m.status.State == supervisor.StateFallback:
		return paint(m.theme.err, "Discord fora do túnel, na rede normal")
	}
	return paint(m.theme.dim, "Discord aguardando o túnel")
}

var quips = map[mood][]string{
	moodHappy: {
		"voz, vídeo e tela saindo pelo mesmo IP",
		"tudo tranquilo por aqui",
		"pode chamar a galera pra call",
		"túnel firme, Discord feliz",
	},
	moodBooting: {"subindo o túnel…"},
	moodWorried: {"perdi o servidor de vista, procurando…", "segurando as pontas, já volto"},
	moodSad:     {"sem túnel por enquanto, mas sigo tentando"},
	moodAsleep:  {"fechando o túnel e devolvendo o Discord à rede normal…"},
}

// quip changes every ten seconds.
func (m Model) quip(md mood) string {
	q := quips[md]
	return q[m.frame/100%len(q)]
}

// cards draws the connection and traffic cards, side by side when they fit.
func (m Model) cards(width int, omit map[string]bool) []string {
	conn, traffic := m.connectionBody(omit), m.trafficBody()
	if width < sideBySide {
		return append(lines(m.card(bold("conexão"), conn, width)), lines(m.card(bold("tráfego"), traffic, width))...)
	}
	// Both cards get the same height, so their borders line up.
	for len(conn) < len(traffic) {
		conn = append(conn, "")
	}
	for len(traffic) < len(conn) {
		traffic = append(traffic, "")
	}
	left := (width - 1) / 2
	return lines(lipgloss.JoinHorizontal(lipgloss.Top,
		m.card(bold("conexão"), conn, left), " ", m.card(bold("tráfego"), traffic, width-left-1)))
}

func (m Model) row(label, value string) string {
	return paint(m.theme.dim, fit(label, labelWidth)) + value
}

func (m Model) connectionBody(omit map[string]bool) []string {
	st, r := m.status, m.reading
	body := []string{
		m.row("servidor", orDash(st.Endpoint)),
		m.row("IP no túnel", orDash(st.Address)),
		m.row("handshake", m.handshake()),
		m.row("RTT", m.rtt(r.RTT)),
	}
	if !omit[cutProfile] && m.sess.Profile != "" {
		body = append(body, m.row("perfil", m.sess.Profile))
	}
	return body
}

// staleHandshake is when a handshake reads as old: WireGuard redoes it every
// two minutes while traffic flows.
const staleHandshake = 3 * time.Minute

func (m Model) handshake() string {
	r := m.reading
	switch {
	case !r.HasTraffic:
		return paint(m.theme.dim, "—")
	case r.Traffic.LastHandshake.IsZero():
		return paint(m.theme.warn, "aguardando o primeiro")
	}
	color := m.theme.ok
	if time.Since(r.Traffic.LastHandshake) > staleHandshake {
		color = m.theme.warn
	}
	return paint(color, ago(r.Traffic.LastHandshake, time.Now()))
}

func (m Model) rtt(d time.Duration) string {
	switch {
	case d <= 0:
		return paint(m.theme.dim, "—")
	case d < 80*time.Millisecond:
		return paint(m.theme.ok, formatRTT(d))
	case d < 200*time.Millisecond:
		return paint(m.theme.warn, formatRTT(d))
	}
	return paint(m.theme.err, formatRTT(d))
}

func (m Model) trafficBody() []string {
	if !m.hasTraffic() {
		if m.status.State == supervisor.StateHealthy {
			return []string{
				paint(m.theme.dim, "o WireSock não informa o tráfego;"),
				paint(m.theme.dim, "o gráfico mostra o RTT ao servidor."),
			}
		}
		return []string{paint(m.theme.dim, "sem túnel agora")}
	}
	now := m.hist.latest()
	seconds := chartWindows[m.chart.window]
	var peak float64
	for _, s := range m.hist.window(seconds) {
		peak = max(peak, s.down)
	}
	t := m.reading.Traffic
	return []string{
		m.row("↓ download", boldPaint(m.theme.download, formatRate(now.down))),
		m.row("↑ upload", boldPaint(m.theme.upload, formatRate(now.up))),
		m.row("total", paint(m.theme.download, "↓ "+formatVolume(t.RxBytes))+"  "+paint(m.theme.upload, "↑ "+formatVolume(t.TxBytes))),
		m.row("pico "+windowName(seconds), paint(m.theme.download, "↓ "+formatRate(peak))),
	}
}

// alerts are the transport's warning and the last error, kept on screen after
// reconnecting with its time, so an old error does not read as a current one.
func (m Model) alerts(width int, folded bool) []string {
	var out []string
	if m.status.Warning != "" {
		out = append(out, paint(m.theme.warn, "! "+truncate(m.status.Warning, width-2)))
	}
	if m.status.LastError == nil {
		return out
	}

	when := ""
	if !m.status.LastErrorAt.IsZero() {
		when = m.status.LastErrorAt.Format("15:04") + "  "
	}
	prefix := "✗ " + when
	pad := strings.Repeat(" ", len([]rune(prefix)))
	room := width - len(pad)

	limit := maxErrorRows
	if folded {
		limit = 1
	}
	// The error wraps instead of being cut, so the instruction some errors
	// carry on a later line stays readable.
	rows := elide(wrap(errtext.Translate(m.status.LastError), room), limit, room)

	color := m.theme.err
	if m.status.State == supervisor.StateHealthy {
		color = m.theme.dim
	}
	for i, row := range rows {
		lead := pad
		if i == 0 {
			lead = prefix
		}
		out = append(out, paint(color, lead+row))
	}
	return out
}

type keyHint struct{ key, what string }

func (m Model) footer(width int) []string {
	var hints []keyHint
	switch {
	case m.typing():
		hints = []keyHint{{"enter", "usar o token"}, {"esc", "sair"}}
	case m.stopping:
		hints = []keyHint{{"q", "sair já"}}
	case m.phase == phaseBoot:
		hints = []keyHint{{"q", "sair"}}
	default:
		hints = []keyHint{{"q", "sair"}}
		if m.opts.Detached {
			hints = []keyHint{{"q", "fechar (o túnel continua)"}}
		}
		if m.retrying() {
			hints = append(hints, keyHint{"r", "tentar agora"})
		}
		if m.hasTraffic() {
			hints = append(hints, keyHint{"tab", "tráfego/RTT"})
		}
		hints = append(hints, keyHint{"←→", "inspecionar"}, keyHint{"z", "janela"})
	}

	parts := make([]string, len(hints))
	for i, h := range hints {
		parts[i] = boldPaint(m.theme.brand, h.key) + " " + paint(m.theme.dim, h.what)
	}
	keys := strings.Join(parts, paint(m.theme.dim, "  ·  "))
	if m.sess.TrayHint && m.phase == phaseDashboard {
		keys += paint(m.theme.dim, "  ·  minimizar manda para a bandeja")
	}

	out := []string{""}
	if m.notice != "" {
		out = append(out, paint(m.theme.dim, m.notice))
	}
	out = append(out, fit(keys, width))
	if m.sess.TrayErr != nil {
		out = append(out, paint(m.theme.warn, truncate("bandeja indisponível: "+errtext.Translate(m.sess.TrayErr), width)))
	}
	return out
}

// card frames body under a title set in the top border.
func (m Model) card(title string, body []string, width int) string {
	inner := max(1, width-2*cardChrome)
	border := func(s string) string { return paint(m.theme.border, s) }
	fill := max(0, width-lipgloss.Width(title)-5)

	var b strings.Builder
	b.WriteString(border("╭─ ") + title + border(" "+strings.Repeat("─", fill)+"╮") + "\n")
	for _, l := range body {
		b.WriteString(border("│ ") + fit(l, inner) + border(" │") + "\n")
	}
	b.WriteString(border("╰" + strings.Repeat("─", max(0, width-2)) + "╯"))
	return b.String()
}

func (m Model) stateWord() string {
	switch {
	case m.bootErr != nil:
		return "erro"
	case m.stopping:
		return "encerrando"
	case m.phase == phaseBoot:
		return "iniciando"
	}
	return statustext.StateLabel(m.status.State)
}

// taskbar mirrors the state in the terminal's progress indicator, which some
// terminals show on the tab or the taskbar.
func (m Model) taskbar() *tea.ProgressBar {
	if s := m.steps[stepUpdate]; s.state == stepRunning && s.fraction >= 0 {
		return tea.NewProgressBar(tea.ProgressBarDefault, int(s.fraction*100))
	}
	switch {
	case m.bootErr != nil:
		return tea.NewProgressBar(tea.ProgressBarError, 100)
	case m.phase == phaseBoot && m.handoverAt == 0:
		return tea.NewProgressBar(tea.ProgressBarIndeterminate, 0)
	case m.status.State == supervisor.StateDegraded:
		return tea.NewProgressBar(tea.ProgressBarWarning, 100)
	case m.status.State == supervisor.StateFallback:
		return tea.NewProgressBar(tea.ProgressBarError, 100)
	}
	return nil
}
