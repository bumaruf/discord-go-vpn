package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/bumaruf/discord-go-vpn/internal/supervisor"
)

func askingModel(ask supervisor.TokenAsk, apply ApplyToken) Model {
	m := dashboard(supervisor.Status{State: supervisor.StateFallback, TokenAsk: ask})
	m.sess.ApplyToken = apply
	return m
}

func typeRunes(m Model, text string) Model {
	for _, r := range text {
		m = pressKey(m, string(r))
	}
	return m
}

func TestScreenAsksForATokenWhenOneWouldHelp(t *testing.T) {
	refused := screenOf(askingModel(supervisor.TokenRefused, func(string) error { return nil }))
	t.Logf("\n%s", refused)

	for _, want := range []string{"Cole um token de conexão novo", "revogado", "dgv1_", "enter", "esc"} {
		if !strings.Contains(refused, want) {
			t.Errorf("screen does not mention %q:\n%s", want, refused)
		}
	}
	if strings.Contains(refused, "tentar agora") {
		t.Error("the footer offers unrelated keys while the field takes the keyboard")
	}

	silent := screenOf(askingModel(supervisor.TokenUnanswered, func(string) error { return nil }))
	if !strings.Contains(silent, "não respondeu") {
		t.Errorf("a silent server must say so:\n%s", silent)
	}

	// Without a token there is nothing to replace, so the field never shows up.
	withConf := screenOf(askingModel(supervisor.TokenRefused, nil))
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
	next, _ := m.Update(tea.PasteMsg{Content: "tokenx \n"})
	m = pressKey(next.(Model), "backspace")
	if m.stopping {
		t.Fatal("typing in the field quit the screen")
	}
	if !strings.Contains(screenOf(m), "dgv1_qrtoken") {
		t.Errorf("what was typed is not on screen:\n%s", screenOf(m))
	}

	m = pressKey(m, "enter")
	// A pasted token often brings a trailing space or newline, which is not part of it.
	if got != "dgv1_qrtoken" {
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

	m = pressKey(typeRunes(m, "nao-e-token"), "enter")

	screen := screenOf(m)
	if !strings.Contains(screen, "isso não é um token de conexão") {
		t.Errorf("the refusal is not on screen:\n%s", screen)
	}
	if !strings.Contains(screen, "nao-e-token") {
		t.Errorf("what was typed was thrown away:\n%s", screen)
	}
}

func TestTokenFieldEscStillQuits(t *testing.T) {
	m := typeRunes(askingModel(supervisor.TokenRefused, func(string) error { return nil }), "dgv1_x")
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape}); !isQuit(cmd) {
		t.Error("esc must still quit while the field is on screen")
	}
}

func TestBootTokenGoesToStartup(t *testing.T) {
	m := New(t.Context(), Options{Version: "v1.4.0"})
	m.asking = true
	m = pressKey(typeRunes(m, "dgv1_abc"), "enter")
	select {
	case got := <-m.answers:
		if got != "dgv1_abc" {
			t.Errorf("startup got %q", got)
		}
	default:
		t.Fatal("enter didn't hand the token to startup")
	}
	if m.asking {
		t.Error("the field must leave while startup validates the token")
	}
}
