package ui

import (
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"

	"github.com/bumaruf/discord-go-vpn/internal/supervisor"
)

func TestTruncateKeepsAccents(t *testing.T) {
	got := truncate("conexão recusada", 8)
	if !strings.HasPrefix(got, "conexão ") {
		t.Fatalf("truncate split a rune: %q", got)
	}
	if truncate("curto", 40) != "curto" {
		t.Fatal("text that fits must be left alone")
	}
}

// TestScreens renders the states people see; run with -v to inspect them.
func TestScreens(t *testing.T) {
	base := func(st supervisor.Status) Model {
		return Model{
			spin:    spinner.New(),
			status:  st,
			profile: "arquivo próprio (wg0.conf)",
			width:   84,
		}
	}

	now := time.Now()

	cases := []struct {
		name  string
		model Model
	}{
		{"starting", base(supervisor.Status{State: supervisor.StateStarting, Since: now})},
		{"connected", base(supervisor.Status{
			State: supervisor.StateHealthy, Since: now.Add(-12 * time.Minute), DiscordTunneled: true,
			LastError: errors.New("handshake não fechou em 10s"), LastErrorAt: now.Add(-9 * time.Minute),
		})},
		{"reconnecting", base(supervisor.Status{
			State: supervisor.StateDegraded, Attempt: 3, Since: now,
			LastError: errors.New("o namespace não chegou a subir"), LastErrorAt: now,
			DiscordTunneled: true,
		})},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			screen := c.model.View()
			t.Logf("\n%s", screen)

			if strings.Contains(screen, "%!") {
				t.Fatalf("a formatting verb was left on screen:\n%s", screen)
			}
			for _, line := range strings.Split(screen, "\n") {
				if n := len([]rune(line)); n > c.model.width {
					t.Fatalf("line with %d columns overflows a %d-column window: %q", n, c.model.width, line)
				}
			}
		})
	}

	for _, c := range cases {
		if got := frameWidth(c.model.View()); got != c.model.width {
			t.Fatalf("%s: frame has %d columns in a %d-column window", c.name, got, c.model.width)
		}
	}

	if strings.Contains(cases[1].model.View(), "tentar agora") {
		t.Fatal("the footer offers to retry while the tunnel is up")
	}
	if !strings.Contains(cases[2].model.View(), "tentar agora") {
		t.Fatal("the footer must offer to retry while waiting for the backoff")
	}
}

func TestViewShowsTransportReportsAndTrayFailure(t *testing.T) {
	m := Model{
		spin: spinner.New(), width: 84, trayErr: errors.New("sem área de notificação"),
		status: supervisor.Status{
			State:    supervisor.StateStarting,
			Progress: "WireSock não encontrado, instalando (pode levar um ou dois minutos)…",
			Warning:  "não consegui amarrar o WireSock a este processo (acesso negado)",
		},
	}

	screen := stripColors(m.View())
	t.Logf("\n%s", screen)
	for _, want := range []string{m.status.Progress, m.status.Warning, "bandeja indisponível: sem área de notificação"} {
		if !strings.Contains(screen, want) {
			t.Errorf("screen lacks %q", want)
		}
	}
	for _, line := range strings.Split(screen, "\n") {
		if n := len([]rune(line)); n > m.width {
			t.Errorf("line with %d columns overflows a %d-column window: %q", n, m.width, line)
		}
	}
}

func TestViewShowsEveryLineOfAMultiLineError(t *testing.T) {
	const instruction = `clique com o botão direito no .exe e escolha "Executar como administrador"`
	m := Model{
		spin: spinner.New(), width: 84,
		status: supervisor.Status{
			State: supervisor.StateDegraded, Attempt: 1, LastErrorAt: time.Now(),
			LastError: errors.New("o WireSock instala um filtro no kernel e precisa de administrador.\n  " + instruction),
		},
	}

	screen := stripColors(m.View())
	t.Logf("\n%s", screen)
	for _, want := range []string{"precisa de administrador.", instruction} {
		if !strings.Contains(words(screen), want) {
			t.Errorf("screen lacks %q", want)
		}
	}
	assertFits(t, screen, m.width)
}

func TestViewWrapsALongErrorInsideTheFrame(t *testing.T) {
	const cause = "a conexão foi recusada (tcp 203.0.113.10:443)"
	m := Model{
		spin: spinner.New(), width: 60,
		status: supervisor.Status{
			State: supervisor.StateDegraded, Attempt: 1, LastErrorAt: time.Now(),
			LastError: errors.New("não consegui obter um acesso: não consegui falar com o servidor de acesso: " + cause),
		},
	}

	screen := stripColors(m.View())
	t.Logf("\n%s", screen)
	if !strings.Contains(words(screen), cause) {
		t.Errorf("the end of a long error was cut off:\n%s", screen)
	}
	assertFits(t, screen, m.width)
}

func TestViewTranslatesRawErrors(t *testing.T) {
	m := Model{
		spin: spinner.New(), width: 84, trayErr: &os.SyscallError{Syscall: "Shell_NotifyIcon", Err: syscall.EACCES},
		status: supervisor.Status{
			State: supervisor.StateDegraded, Attempt: 1, LastErrorAt: time.Now(),
			LastError: fmt.Errorf("não consegui falar com o servidor de acesso: %w",
				&net.OpError{Op: "dial", Net: "tcp", Addr: &net.TCPAddr{IP: net.IPv4(203, 0, 113, 10), Port: 443},
					Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED}}),
		},
	}

	screen := words(stripColors(m.View()))
	t.Logf("\n%s", screen)
	for _, want := range []string{
		"não consegui falar com o servidor de acesso: a conexão foi recusada (tcp 203.0.113.10:443)",
		"bandeja indisponível: permissão negada",
	} {
		if !strings.Contains(screen, want) {
			t.Errorf("screen lacks %q", want)
		}
	}
}

func TestShortWindowFoldsAMultiLineErrorLast(t *testing.T) {
	const instruction = "clique com o botão direito no .exe"
	base := Model{
		spin: spinner.New(), width: 84,
		profile: "acesso do servidor",
		status: supervisor.Status{
			State: supervisor.StateDegraded, Attempt: 1, LastErrorAt: time.Now(),
			LastError: errors.New("o WireSock precisa de administrador.\n  " + instruction),
		},
	}

	cases := []struct {
		name            string
		height          int
		wantInstruction bool
		wantProfile     bool
	}{
		// Every row in cutOrder is gone and the screen still needs one line.
		{"only the first line fits", 11, false, false},
		{"rows go before the instruction", 12, true, false},
		{"room for everything", 20, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := base
			m.height = c.height
			screen := stripColors(m.View())
			t.Logf("\n%s", screen)

			if n := strings.Count(screen, "\n"); n > m.height {
				t.Fatalf("the screen took %d lines in a %d-line window", n, m.height)
			}
			if !strings.Contains(screen, "precisa de administrador") {
				t.Error("the error's first line must never be cut")
			}
			if got := strings.Contains(screen, instruction); got != c.wantInstruction {
				t.Errorf("instruction on screen = %v, want %v", got, c.wantInstruction)
			}
			if got := strings.Contains(screen, "acesso do servidor"); got != c.wantProfile {
				t.Errorf("profile row on screen = %v, want %v", got, c.wantProfile)
			}
			assertFits(t, screen, m.width)
		})
	}
}

// words joins the screen's words across rows and frame borders, for text that
// may wrap.
func words(screen string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(screen, "│", " ")), " ")
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

func assertFits(t *testing.T, screen string, width int) {
	t.Helper()
	for _, line := range strings.Split(screen, "\n") {
		if n := len([]rune(line)); n > width {
			t.Errorf("line with %d columns overflows a %d-column window: %q", n, width, line)
		}
	}
}

func TestFrameRespectsMaxWidth(t *testing.T) {
	m := Model{spin: spinner.New(), status: supervisor.Status{State: supervisor.StateHealthy}, width: 240}
	if got := frameWidth(m.View()); got != maxWidth {
		t.Fatalf("in a 240-column window the frame has %d columns, want the cap of %d", got, maxWidth)
	}
}

func TestNarrowWindowDropsFrame(t *testing.T) {
	m := Model{spin: spinner.New(), status: supervisor.Status{State: supervisor.StateHealthy}, width: 40}
	if strings.Contains(m.View(), "╭") {
		t.Fatal("a narrow window must drop the frame")
	}
}

func TestShortWindowCutsLeastImportant(t *testing.T) {
	base := Model{
		spin: spinner.New(), width: 84, height: 10,
		profile: "arquivo próprio (wg0.conf)",
		status:  supervisor.Status{State: supervisor.StateHealthy, DiscordTunneled: true},
	}

	screen := base.View()
	t.Logf("\n%s", screen)
	if n := strings.Count(screen, "\n"); n > base.height {
		t.Fatalf("the screen took %d lines in a %d-line window:\n%s", n, base.height, screen)
	}

	for _, essential := range []string{"conectado", "discord"} {
		if !strings.Contains(screen, essential) {
			t.Fatalf("cut %q, which is never dropped:\n%s", essential, screen)
		}
	}
	if strings.Contains(screen, "wg0.conf") {
		t.Fatalf("the profile row should have been cut first:\n%s", screen)
	}

	// An unknown height (zero) cuts nothing.
	noHeight := base
	noHeight.height = 0
	if !strings.Contains(noHeight.View(), "wg0.conf") {
		t.Fatal("with an unknown height the whole screen must be drawn")
	}
}

// frameWidth measures the box's top line, in columns.
func frameWidth(screen string) int {
	for _, line := range strings.Split(screen, "\n") {
		if strings.Contains(line, "╭") {
			return len([]rune(stripColors(line)))
		}
	}
	return 0
}

// stripColors removes ANSI codes, which take bytes but no columns.
func stripColors(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
