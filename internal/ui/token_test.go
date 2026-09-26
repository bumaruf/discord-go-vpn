package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/bumaruf/discord-go-vpn/internal/supervisor"
)

func askingModel(ask supervisor.TokenAsk, apply ApplyToken) Model {
	return Model{
		spin:       spinner.New(),
		width:      84,
		profile:    "acesso do servidor",
		status:     supervisor.Status{State: supervisor.StateFallback, TokenAsk: ask},
		applyToken: apply,
	}
}

func typeRunes(m Model, text string) Model {
	next, _ := m.updateTokenField(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
	return next.(Model)
}

func press(m Model, key tea.KeyType) Model {
	next, _ := m.Update(tea.KeyMsg{Type: key})
	return next.(Model)
}

func TestScreenAsksForATokenWhenOneWouldHelp(t *testing.T) {
	refused := askingModel(supervisor.TokenRefused, func(string) error { return nil }).View()
	t.Logf("\n%s", refused)

	for _, want := range []string{"Cole um token de conexão novo", "revogado", "dgv1_", "enter", "esc"} {
		if !strings.Contains(refused, want) {
			t.Errorf("screen does not mention %q:\n%s", want, refused)
		}
	}
	if strings.Contains(refused, "tentar agora") {
		t.Error("the footer offers unrelated keys while the field takes the keyboard")
	}

	silent := askingModel(supervisor.TokenUnanswered, func(string) error { return nil }).View()
	if !strings.Contains(silent, "não respondeu") {
		t.Errorf("a silent server must say so:\n%s", silent)
	}

	// Without a token there is nothing to replace, so the field never shows up.
	withConf := askingModel(supervisor.TokenRefused, nil).View()
	if strings.Contains(withConf, "Cole um token") {
		t.Errorf("the field showed up with no token in use:\n%s", withConf)
	}
}

func TestTokenFieldHandsTheTokenOverAndKeepsTypingOffTheCommands(t *testing.T) {
	var got string
	m := askingModel(supervisor.TokenRefused, func(typed string) error {
		got = typed
		return nil
	})

	// "q" would quit and "r" would reconnect if the field did not take the keys.
	m = typeRunes(m, "dgv1_qr")
	m = typeRunes(m, "tokenx ")
	m = press(m, tea.KeyBackspace)
	if m.quit {
		t.Fatal("typing in the field quit the screen")
	}
	if !strings.Contains(m.View(), "dgv1_qrtokenx") {
		t.Errorf("what was typed is not on screen:\n%s", m.View())
	}

	m = press(m, tea.KeyEnter)
	// A pasted token often brings a trailing space, which is not part of it.
	if got != "dgv1_qrtokenx" {
		t.Errorf("token handed over = %q, want what was typed, trimmed", got)
	}
	if len(m.tokenField) != 0 {
		t.Errorf("field = %q, want it emptied once the token was taken", string(m.tokenField))
	}
}

func TestTokenFieldKeepsAMalformedTokenOnScreenWithItsError(t *testing.T) {
	m := askingModel(supervisor.TokenRefused, func(string) error {
		return errors.New("isso não é um token de conexão")
	})

	m = typeRunes(m, "nao-e-token")
	m = press(m, tea.KeyEnter)

	screen := m.View()
	if !strings.Contains(screen, "isso não é um token de conexão") {
		t.Errorf("the refusal is not on screen:\n%s", screen)
	}
	if !strings.Contains(screen, "nao-e-token") {
		t.Errorf("what was typed was thrown away:\n%s", screen)
	}
}

func TestTokenFieldEscStillQuits(t *testing.T) {
	m := askingModel(supervisor.TokenRefused, func(string) error { return nil })

	if m = press(typeRunes(m, "dgv1_x"), tea.KeyEsc); !m.quit {
		t.Error("esc must still quit while the field is on screen")
	}
}
