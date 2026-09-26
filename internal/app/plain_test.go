package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/routing"
	"github.com/bumaruf/discord-go-vpn/internal/supervisor"
)

func TestRunPlainPrintsEachNewDropReasonOnce(t *testing.T) {
	refused := func() error {
		return fmt.Errorf("não consegui obter um acesso: %w", &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED})
	}
	renewal := errors.New("não consegui renovar o acesso: o servidor recusou (HTTP 503): tente de novo")
	at := func(minute int) time.Time { return time.Date(2026, 9, 13, 10, minute, 0, 0, time.UTC) }

	updates := make(chan supervisor.Status, 16)
	for _, st := range []supervisor.Status{
		{State: supervisor.StateStarting},
		{State: supervisor.StateDegraded, Attempt: 1, LastError: refused(), LastErrorAt: at(0)},
		{State: supervisor.StateDegraded, Attempt: 2, LastError: refused(), LastErrorAt: at(1)},
		{State: supervisor.StateHealthy},
		{State: supervisor.StateHealthy, LastError: renewal, LastErrorAt: at(2)},
		// An unrelated update resends the same failure.
		{State: supervisor.StateHealthy, LastError: renewal, LastErrorAt: at(2), DiscordTunneled: true},
		// The renewal in between succeeded, which leaves LastError set.
		{State: supervisor.StateHealthy, LastError: renewal, LastErrorAt: at(6)},
		{State: supervisor.StateDegraded, Attempt: 1, LastError: refused(), LastErrorAt: at(7)},
	} {
		updates <- st
	}
	close(updates)

	var out, errOut strings.Builder
	opts := Options{Mode: routing.ModeNetns, Out: &out, Err: &errOut}
	runPlain(context.Background(), opts, updates, "acesso do servidor", nil, nil)

	wantErr := "  erro: não consegui obter um acesso: a conexão foi recusada\n" +
		"  erro: " + renewal.Error() + "\n" +
		"  erro: " + renewal.Error() + "\n" +
		"  erro: não consegui obter um acesso: a conexão foi recusada\n"
	if got := errOut.String(); got != wantErr {
		t.Errorf("stderr =\n%q\nwant\n%q", got, wantErr)
	}
}

func TestRunPlainPrintsReportsAndTrayFailure(t *testing.T) {
	const warning = "não consegui amarrar o WireSock a este processo (acesso negado)"
	updates := make(chan supervisor.Status, 8)
	for _, st := range []supervisor.Status{
		{State: supervisor.StateStarting},
		{State: supervisor.StateStarting, Progress: "WireSock não encontrado, instalando…"},
		{State: supervisor.StateStarting, Progress: "WireSock instalado"},
		{State: supervisor.StateStarting, Progress: "WireSock instalado", Warning: warning},
		{State: supervisor.StateStarting, Warning: warning},
		{State: supervisor.StateHealthy, Warning: warning},
	} {
		updates <- st
	}
	close(updates)

	var out, errOut strings.Builder
	opts := Options{Mode: routing.ModeWiresock, Out: &out, Err: &errOut}
	runPlain(context.Background(), opts, updates, "acesso do servidor", errors.New("sem área de notificação"), nil)

	wantOut := "\nperfil  acesso do servidor\n\n" +
		"  · WireSock não encontrado, instalando…\n" +
		"  · WireSock instalado\n" +
		"  ✓ conectado\n"
	if got := out.String(); got != wantOut {
		t.Errorf("stdout =\n%q\nwant\n%q", got, wantOut)
	}
	wantErr := "bandeja indisponível: sem área de notificação\n" +
		"  aviso: " + warning + "\n"
	if got := errOut.String(); got != wantErr {
		t.Errorf("stderr =\n%q\nwant\n%q", got, wantErr)
	}
}

// Without a screen to hold a field, the plain mode asks on stdout and reads the
// token from stdin.
func TestRunPlainTakesAReplacementTokenFromStdin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	applied := make(chan string, 1)
	apply := func(typed string) error {
		applied <- typed
		cancel()
		return nil
	}

	updates := make(chan supervisor.Status, 2)
	updates <- supervisor.Status{State: supervisor.StateFallback, TokenAsk: supervisor.TokenRefused}

	var out, errOut strings.Builder
	opts := Options{
		Mode: routing.ModeNetns, Out: &out, Err: &errOut,
		In: strings.NewReader("  dgv1_replacement  \n"),
	}

	done := make(chan struct{})
	go func() {
		runPlain(ctx, opts, updates, "acesso do servidor", nil, apply)
		close(done)
	}()

	select {
	case got := <-applied:
		if got != "dgv1_replacement" {
			t.Errorf("token read = %q, want it trimmed", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the token typed was never taken")
	}
	<-done

	if !strings.Contains(out.String(), "Cole um token de conexão novo") {
		t.Errorf("stdout never asked for a token:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "revogado") {
		t.Errorf("stdout never said why:\n%s", out.String())
	}
}
