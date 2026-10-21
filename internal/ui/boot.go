package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/bumaruf/discord-go-vpn/internal/errtext"
	"github.com/bumaruf/discord-go-vpn/internal/startup"
)

type stepID int

const (
	stepUpdate stepID = iota
	stepAccess
	stepTunnel
	stepDiscord
	stepCount
)

var stepNames = [stepCount]string{"atualização", "acesso", "túnel", "Discord"}

type stepState int

const (
	stepPending stepState = iota
	stepRunning
	stepDone
	stepSkipped
	stepFailed
)

type stepView struct {
	state    stepState
	detail   string
	fraction float64 // startup.NoFraction when there is no bar
}

func stepFor(s startup.Step) stepID {
	if s == startup.StepUpdate {
		return stepUpdate
	}
	return stepAccess
}

func stateFor(s startup.State) stepState {
	switch s {
	case startup.Done:
		return stepDone
	case startup.Skipped:
		return stepSkipped
	case startup.Failed:
		return stepFailed
	}
	return stepRunning
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const stepNameWidth = 14

func (m Model) bootMood() mood {
	switch {
	case m.bootErr != nil:
		return moodSad
	case m.asking:
		return moodWorried
	case m.handoverAt > 0 && m.steps[stepTunnel].state == stepFailed:
		return moodWorried
	case m.handoverAt > 0:
		return moodCelebrating
	}
	return moodBooting
}

func (m Model) bootQuip() string {
	switch {
	case m.bootErr != nil:
		return "não deu pra começar"
	case m.sess.Restart != "":
		return "versão nova instalada, já volto!"
	case m.asking:
		return "preciso de um token de conexão"
	case m.handoverAt > 0 && m.steps[stepTunnel].state == stepFailed:
		return "o túnel não subiu de primeira; sigo tentando"
	case m.handoverAt > 0:
		return "tudo pronto!"
	}
	return "preparando tudo…"
}

// bootBody draws the checklist and whatever startup is waiting on.
func (m Model) bootBody(width int) []string {
	var body []string
	for id, s := range m.steps {
		body = append(body, m.stepRow(stepID(id), s, width-2*cardChrome))
		if s.state == stepRunning && s.fraction >= 0 {
			body = append(body, strings.Repeat(" ", stepNameWidth+2)+m.progressBar(s.fraction, width-2*cardChrome-stepNameWidth-2))
		}
	}
	out := lines(m.card(bold("iniciando"), body, width))

	if m.asking {
		out = append(out, "")
		out = append(out, m.tokenPrompt("Cole o token de conexão que você recebeu", m.tell, width)...)
	}
	if m.bootErr != nil {
		out = append(out, "")
		for _, row := range wrap(errtext.Translate(m.bootErr), width) {
			out = append(out, paint(m.theme.err, row))
		}
	}
	return out
}

func (m Model) stepRow(id stepID, s stepView, width int) string {
	var icon string
	switch s.state {
	case stepPending:
		icon = paint(m.theme.dim, "○")
	case stepRunning:
		icon = paint(m.theme.brand, spinnerFrames[m.frame%len(spinnerFrames)])
	case stepDone:
		icon = paint(m.theme.ok, "✓")
	case stepSkipped:
		icon = paint(m.theme.dim, "–")
	case stepFailed:
		icon = paint(m.theme.warn, "✗")
		if id == stepAccess && m.bootErr != nil {
			icon = paint(m.theme.err, "✗")
		}
	}
	name := fit(stepNames[id], stepNameWidth-2)
	if s.state == stepPending {
		name = paint(m.theme.dim, name)
	}
	detail := paint(m.theme.dim, truncate(s.detail, max(1, width-stepNameWidth-1)))
	return icon + " " + name + " " + detail
}

// progressBar is a gradient from blurple to fuchsia, with the percentage.
func (m Model) progressBar(fraction float64, width int) string {
	pct := fmt.Sprintf(" %3d%%", int(fraction*100))
	n := max(4, width-len(pct))
	filled := int(fraction * float64(n))
	colors := lipgloss.Blend1D(max(2, n), m.theme.brand, m.theme.accent)
	var b strings.Builder
	for i := range n {
		if i < filled {
			b.WriteString(paint(colors[i], "━"))
		} else {
			b.WriteString(paint(m.theme.border, "─"))
		}
	}
	return b.String() + paint(m.theme.dim, pct)
}

// tokenPrompt draws a heading, why a token is asked for, and the field.
func (m Model) tokenPrompt(heading, reason string, width int) []string {
	out := []string{bold(heading)}
	style := m.theme.warn
	if reason == "" {
		reason, style = "Ele começa com dgv1_; quem te convidou pode gerar outro.", m.theme.dim
	}
	for _, row := range wrap(reason, width) {
		out = append(out, paint(style, row))
	}
	out = append(out, m.fieldView(width))
	if m.tokenError != "" {
		for _, row := range wrap(m.tokenError, width) {
			out = append(out, paint(m.theme.err, row))
		}
	}
	return out
}

// fieldView shows the end of what was typed, since a token is longer than the
// window and its last runes are the ones being typed.
func (m Model) fieldView(width int) string {
	const prompt = "› "
	room := width - len([]rune(prompt)) - 1 // the cursor
	cursor := paint(m.theme.brand, "▊")
	if m.frame/5%2 == 1 {
		cursor = " "
	}

	if m.tokenField == nil {
		return paint(m.theme.brand, prompt) + paint(m.theme.dim, "dgv1_…") + cursor
	}
	shown := m.tokenField
	if len(shown) > room {
		shown = shown[len(shown)-room:]
	}
	return paint(m.theme.brand, prompt) + string(shown) + cursor
}
