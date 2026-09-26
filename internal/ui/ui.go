// Package ui draws the live screen of the tunnel's current state. It only reads
// the supervisor's Status; its one request is Reconnect.
package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bumaruf/discord-go-vpn/internal/errtext"
	"github.com/bumaruf/discord-go-vpn/internal/supervisor"
)

// Adaptive colours, because the same binary runs on light and dark terminals.
var (
	accent = lipgloss.AdaptiveColor{Light: "166", Dark: "215"}
	okCol  = lipgloss.AdaptiveColor{Light: "28", Dark: "42"}
	warn   = lipgloss.AdaptiveColor{Light: "166", Dark: "214"}
	errCol = lipgloss.AdaptiveColor{Light: "160", Dark: "203"}
	dimCol = lipgloss.AdaptiveColor{Light: "245", Dark: "244"}

	dim    = lipgloss.NewStyle().Foreground(dimCol)
	bold   = lipgloss.NewStyle().Bold(true)
	label  = lipgloss.NewStyle().Foreground(dimCol).Width(labelWidth)
	keySt  = lipgloss.NewStyle().Foreground(accent).Bold(true)
	okSt   = lipgloss.NewStyle().Foreground(okCol)
	warnSt = lipgloss.NewStyle().Foreground(warn)
	errSt  = lipgloss.NewStyle().Foreground(errCol)

	box = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(dimCol).
		Padding(0, boxPadding)
)

// ApplyToken takes what was typed in the token field. It returns an error the
// person reads, and nothing else: whether the server accepts the token shows up
// in the next attempt's status.
type ApplyToken func(typed string) error

type Model struct {
	sup    *supervisor.Supervisor
	status supervisor.Status
	spin   spinner.Model

	profile string
	notice  string
	quit    bool

	applyToken ApplyToken
	// tokenField is typed by hand rather than with bubbles' textinput, which
	// pulls in a clipboard dependency that shells out on Linux.
	tokenField []rune
	tokenError string

	// The window Explorer opens is narrow, so the layout follows its size.
	width    int
	height   int
	trayHint bool
	trayErr  error // why there is no tray icon

	noticeUntil time.Time // zero when there is no notice
}

// New builds the screen. trayHint says whether to mention the tray, which only
// exists on Windows; applyToken may be nil when access comes from a .conf.
func New(sup *supervisor.Supervisor, profile string, trayHint bool, trayErr error, applyToken ApplyToken) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(accent)

	return Model{sup: sup, spin: sp, profile: profile, trayHint: trayHint, trayErr: trayErr,
		status: sup.Status(), applyToken: applyToken}
}

// asksToken reports whether the token field is on screen, which also takes the
// keyboard: while it is, typed letters are the token, not commands.
func (m Model) asksToken() bool {
	return m.applyToken != nil && m.status.TokenAsk != supervisor.TokenFine
}

type statusMsg supervisor.Status

type tickMsg time.Time

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, waitStatus(m.sup), tick())
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

const noticeDuration = 4 * time.Second

// tick redraws once a second, so the elapsed time moves when nothing else does.
func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.asksToken() {
			return m.updateTokenField(msg)
		}
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.quit = true
			return m, tea.Quit

		case "r":
			// Only while the supervisor waits to retry, the same states in
			// which the footer offers the key.
			if m.status.State == supervisor.StateDegraded || m.status.State == supervisor.StateFallback {
				m.sup.Reconnect()
				m.notice = "tentando agora…"
				m.noticeUntil = time.Now().Add(noticeDuration)
			}
			return m, nil
		}
		return m, nil

	case statusMsg:
		m.status = supervisor.Status(msg)
		return m, waitStatus(m.sup)

	case tickMsg:
		if !m.noticeUntil.IsZero() && time.Now().After(m.noticeUntil) {
			m.notice, m.noticeUntil = "", time.Time{}
		}
		return m, tick()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	}
	return m, nil
}

// maxTokenField caps what the field holds, well above a token's length: a
// paste of a whole file must not grow the screen without end.
const maxTokenField = 512

// updateTokenField owns the keyboard while the field is on screen: esc and
// ctrl+c still quit, enter hands the token over, and everything else is typing.
// A paste arrives as one message carrying every rune.
func (m Model) updateTokenField(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		m.quit = true
		return m, tea.Quit

	case tea.KeyEnter:
		typed := strings.TrimSpace(string(m.tokenField))
		if typed == "" {
			return m, nil
		}
		if err := m.applyToken(typed); err != nil {
			m.tokenError = errtext.Translate(err)
			return m, nil
		}
		// The supervisor clears TokenAsk, which takes the field off the screen.
		m.tokenField, m.tokenError = nil, ""

	case tea.KeyBackspace:
		if len(m.tokenField) > 0 {
			m.tokenField = m.tokenField[:len(m.tokenField)-1]
		}

	case tea.KeyCtrlU:
		m.tokenField = nil

	case tea.KeySpace:
		// A pasted token sometimes brings a trailing space, and none belongs
		// inside one.

	case tea.KeyRunes:
		room := maxTokenField - len(m.tokenField)
		m.tokenField = append(m.tokenField, msg.Runes[:min(len(msg.Runes), room)]...)
	}
	return m, nil
}

// cutOrder lists the rows dropped from a short window. The state and Discord
// rows are never dropped, and the last cut folds the error to its first row.
var cutOrder = []string{"perfil", "há", errorDetail}

const errorDetail = "detalhe do erro"

// View drops rows in cutOrder until the screen fits the window. It measures the
// rendered screen instead of computing the space up front, which would drift
// whenever a part of the layout changes.
func (m Model) View() string {
	if m.quit {
		return "\n  encerrando…\n\n"
	}

	omit := map[string]bool{}
	for i := 0; ; i++ {
		screen := m.render(omit)
		if m.height <= 0 || strings.Count(screen, "\n") <= m.height || i >= len(cutOrder) {
			return screen
		}
		omit[cutOrder[i]] = true
	}
}

func (m Model) render(omit map[string]bool) string {
	var b strings.Builder
	b.WriteString("\n")

	// A narrow window drops the frame first.
	if m.width > 0 && m.width < minFramedWidth {
		b.WriteString(indent(m.hud(omit)) + "\n")
	} else {
		b.WriteString(m.frame().Render(m.hud(omit)) + "\n")
	}

	if m.asksToken() {
		b.WriteString(m.tokenPrompt())
	}

	if m.notice != "" {
		b.WriteString("\n  " + dim.Render(m.notice) + "\n")
	}
	if m.asksToken() {
		return b.String() + "\n  " + keySt.Render("enter") + " " + dim.Render("usar o token") +
			dim.Render("   ·   ") + keySt.Render("esc") + " " + dim.Render("sair") + "\n"
	}
	footer := "\n  " + keySt.Render("q") + " " + dim.Render("sair")
	if m.status.State == supervisor.StateDegraded || m.status.State == supervisor.StateFallback {
		footer += dim.Render("   ·   ") + keySt.Render("r") + " " + dim.Render("tentar agora")
	}
	if m.trayHint {
		footer += dim.Render("   ·   minimizar manda para a bandeja")
	}
	b.WriteString(footer + "\n")
	if m.trayErr != nil {
		b.WriteString("  " + warnSt.Render(truncate("bandeja indisponível: "+errtext.Translate(m.trayErr), m.innerWidth())) + "\n")
	}
	return b.String()
}

// tokenPrompt draws why the access was refused and the field for the new token.
func (m Model) tokenPrompt() string {
	reason := "O convite deste token foi revogado, ou o token não vale mais."
	if m.status.TokenAsk == supervisor.TokenUnanswered {
		reason = "O servidor deste token não respondeu às últimas tentativas."
	}

	var b strings.Builder
	b.WriteString("\n  " + bold.Render("Cole um token de conexão novo") + "\n")
	for _, row := range wrap(reason, m.innerWidth()) {
		b.WriteString("  " + dim.Render(row) + "\n")
	}
	b.WriteString("  " + m.fieldView() + "\n")
	if m.tokenError != "" {
		for _, row := range wrap(m.tokenError, m.innerWidth()) {
			b.WriteString("  " + errSt.Render(row) + "\n")
		}
	}
	return b.String()
}

// fieldView shows the end of what was typed, since a token is longer than the
// window and its last runes are the ones being typed.
func (m Model) fieldView() string {
	const prompt = "> "
	room := m.innerWidth() - len(prompt) - 1 // the cursor

	if m.tokenField == nil {
		return dim.Render(prompt+"dgv1_…") + keySt.Render("▊")
	}
	shown := m.tokenField
	if len(shown) > room {
		shown = shown[len(shown)-room:]
	}
	return prompt + string(shown) + keySt.Render("▊")
}

// maxWidth caps the frame, so a wide terminal does not strand values far from
// their labels.
const maxWidth = 100

// Layout sizes, in terminal columns.
const (
	labelWidth  = 11
	borderWidth = 1
	boxPadding  = 2

	minFramedWidth = 56

	defaultInnerWidth = 46
	minInnerWidth     = 24
)

// frame sizes the box to the window rather than to the content, which would
// resize it with every state change.
func (m Model) frame() lipgloss.Style {
	if m.width <= 0 {
		return box
	}
	// The border sits outside the width set here.
	return box.Width(m.boxWidth() - 2*borderWidth)
}

// boxWidth includes the border.
func (m Model) boxWidth() int { return min(m.width, maxWidth) }

// innerWidth is the text width inside the frame. Longer lines make lipgloss
// wrap them, breaking the label alignment.
func (m Model) innerWidth() int {
	if m.width <= 0 {
		return defaultInnerWidth
	}
	return max(minInnerWidth, m.boxWidth()-2*(borderWidth+boxPadding))
}

func (m Model) hud(omit map[string]bool) string {
	var b strings.Builder

	b.WriteString(m.linkLine() + "\n")
	if m.status.Progress != "" {
		b.WriteString(dim.Render(truncate(m.status.Progress, m.innerWidth())) + "\n")
	}
	b.WriteString("\n")

	line := func(k, v string) {
		if v != "" && !omit[k] {
			b.WriteString(label.Render(k) + v + "\n")
		}
	}

	line("perfil", m.profile)
	line("discord", m.discordLine())
	line("há", humanDur(m.status.Uptime()))

	if m.status.Warning != "" {
		b.WriteString("\n" + warnSt.Render(truncate(m.status.Warning, m.innerWidth())) + "\n")
	}
	if m.status.LastError != nil {
		b.WriteString("\n" + m.errorRows(omit[errorDetail]))
	}
	return b.String()
}

// maxErrorRows caps the error, which can carry a command's stderr.
const maxErrorRows = 6

// errorRows keeps the last failure on screen after reconnecting, with its time
// so an old error does not read as a current one. The error wraps under its
// time instead of being cut, so the instruction some errors carry on a later
// line stays readable; folded keeps only the first row.
func (m Model) errorRows(folded bool) string {
	when := ""
	if !m.status.LastErrorAt.IsZero() {
		when = m.status.LastErrorAt.Format("15:04") + "  "
	}
	pad := strings.Repeat(" ", len([]rune(when)))
	width := m.innerWidth() - len(pad)

	limit := maxErrorRows
	if folded {
		limit = 1
	}
	rows := elide(wrap(errtext.Translate(m.status.LastError), width), limit, width)

	style := errSt
	if m.status.State == supervisor.StateHealthy {
		style = dim
	}
	var b strings.Builder
	for i, row := range rows {
		prefix := pad
		if i == 0 {
			prefix = when
		}
		b.WriteString(style.Render(prefix+row) + "\n")
	}
	return b.String()
}

// wrap breaks text into rows of at most width runes, at spaces where it can.
// Each line keeps its indentation on the rows it wraps into.
func wrap(text string, width int) []string {
	var rows []string
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		// Capped, so every row keeps room for at least one rune of a word.
		indent := line[:min(len(line)-len(strings.TrimLeft(line, " ")), width-1)]
		row := indent
		for _, word := range strings.Fields(line) {
			for {
				room := width - len([]rune(row))
				if row != indent {
					room-- // the space before the word
				}
				if len([]rune(word)) <= room {
					break
				}
				if row != indent {
					rows = append(rows, row)
					row = indent
					continue
				}
				// A word wider than a whole row is split where the row ends.
				r := []rune(word)
				cut := max(1, width-len(indent))
				rows = append(rows, indent+string(r[:cut]))
				word = string(r[cut:])
			}
			if row != indent {
				row += " "
			}
			row += word
		}
		rows = append(rows, row)
	}
	return rows
}

// elide keeps the first n rows, ending the last one in "…" when it drops any.
func elide(rows []string, n, width int) []string {
	if len(rows) <= n {
		return rows
	}
	last := []rune(rows[n-1])
	rows[n-1] = string(last[:min(len(last), width-1)]) + "…"
	return rows[:n]
}

func (m Model) linkLine() string {
	st := m.status
	name := StateLabel(st.State)
	switch st.State {
	case supervisor.StateHealthy:
		return okSt.Render("●") + "  " + bold.Render(name)

	case supervisor.StateStarting:
		return m.spin.View() + " " + bold.Render(name)

	case supervisor.StateDegraded:
		return warnSt.Render("◐") + "  " + bold.Render(name) +
			dim.Render(fmt.Sprintf("   tentativa %d", st.Attempt))

	case supervisor.StateFallback:
		return errSt.Render("○") + "  " + bold.Render(name) +
			dim.Render("   Discord na rede normal, tentando voltar")
	}
	return dim.Render("○  " + name)
}

func humanDur(d time.Duration) string {
	d = d.Truncate(time.Second)
	h, m, s := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	switch {
	case h > 0:
		return fmt.Sprintf("%dh %02dmin", h, m)
	case m > 0:
		return fmt.Sprintf("%dmin %02ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

func (m Model) discordLine() string {
	switch {
	case m.status.DiscordTunneled:
		return okSt.Render("dentro do túnel")
	case m.status.State == supervisor.StateFallback:
		return errSt.Render("fora do túnel")
	default:
		return dim.Render("aguardando")
	}
}

// indent replaces the frame when the window is too narrow for it.
func indent(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		b.WriteString("  " + line + "\n")
	}
	return b.String()
}

// truncate cuts by rune, so an accented letter is never split.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
