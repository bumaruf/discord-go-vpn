// Package app wires the supervisor to the screen and the tray, and owns the
// process lifetime.
package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bumaruf/discord-go-vpn/internal/connectiontoken"
	"github.com/bumaruf/discord-go-vpn/internal/errtext"
	"github.com/bumaruf/discord-go-vpn/internal/profile"
	"github.com/bumaruf/discord-go-vpn/internal/routing"
	"github.com/bumaruf/discord-go-vpn/internal/supervisor"
	"github.com/bumaruf/discord-go-vpn/internal/ui"
)

// Options carries what the command line decided.
type Options struct {
	ConfigPath  string
	DiscordPath string
	Mode        routing.Mode
	Provision   profile.Provisioning

	// Source, when set, is a profile the caller already opened; Run owns it.
	Source *profile.Source

	Out io.Writer
	Err io.Writer
	// In is where the plain mode reads a replacement token from.
	In io.Reader
}

func Run(opts Options) int {
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.Err == nil {
		opts.Err = os.Stderr
	}
	if opts.In == nil {
		opts.In = os.Stdin
	}

	// Opening the profile up front fails early with a clear message and labels
	// the screen; the supervisor's first connection reuses it.
	source := opts.Source
	if source == nil {
		var err error
		if source, err = profile.Open(opts.ConfigPath, opts.Provision); err != nil {
			if errors.Is(err, profile.ErrNoProfile) {
				fmt.Fprintln(opts.Err, profile.HelpText)
			} else {
				fmt.Fprintln(opts.Err, "erro:", errtext.Translate(err))
			}
			return 1
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Closing the terminal window sends SIGHUP, whose default action would kill
	// the process without tearing the tunnel down.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(stop)
	go func() {
		<-stop
		cancel()
	}()

	sup := supervisor.New(supervisor.Options{
		ConfigPath:  opts.ConfigPath,
		DiscordPath: opts.DiscordPath,
		Mode:        opts.Mode,
		Provision:   opts.Provision,
		Source:      source,
	})

	hasTray := false
	stopTray, trayErr := startTray(sup)
	if stopTray != nil {
		defer stopTray()
		hasTray = true
	}

	done := make(chan struct{})
	go func() {
		sup.Run(ctx)
		close(done)
	}()

	// A .conf takes no token, so there is nothing to replace.
	var apply ui.ApplyToken
	if opts.ConfigPath == "" && opts.Provision.Configured() {
		apply = applyToken(sup)
	}

	if !runTUI(opts, sup, cancel, source.Origin, hasTray, trayErr, apply) {
		runPlain(ctx, opts, sup.Updates(), source.Origin, trayErr, apply)
	}

	cancel()
	<-done
	fmt.Fprintln(opts.Out, "encerrado.")
	return 0
}

func tokenAskText(ask supervisor.TokenAsk) string {
	if ask == supervisor.TokenUnanswered {
		return "O servidor deste token não respondeu às últimas tentativas."
	}
	return "O convite deste token foi revogado, ou o token não vale mais."
}

// readLines delivers what is typed, or nil when nothing will read stdin. The
// goroutine outlives ctx, since a read in progress can't be cancelled; it ends
// with the process.
func readLines(ctx context.Context, in io.Reader, wanted bool) <-chan string {
	if !wanted {
		return nil
	}

	lines := make(chan string)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(in)
		for scanner.Scan() {
			select {
			case lines <- strings.TrimSpace(scanner.Text()):
			case <-ctx.Done():
				return
			}
		}
	}()
	return lines
}

// applyToken hands a token typed on the screen to the supervisor, which uses it
// from the next attempt on. It is saved right away: the person pasted it to
// replace the stored one, and a refusal on the next run discards it.
func applyToken(sup *supervisor.Supervisor) ui.ApplyToken {
	return func(typed string) error {
		token, err := connectiontoken.Parse(typed)
		if err != nil {
			return err
		}
		connectiontoken.Save(token)
		sup.UseToken(token.Provisioning())
		return nil
	}
}

// runTUI reports false when the terminal cannot draw the screen, so the caller
// falls back to plain text.
func runTUI(opts Options, sup *supervisor.Supervisor, cancel context.CancelFunc, profileName string, trayHint bool, trayErr error, apply ui.ApplyToken) bool {
	m := ui.New(sup, profileName, trayHint, trayErr, apply)

	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(opts.Err, "a interface não pôde ser desenhada, seguindo em modo texto")
		return false
	}

	cancel()
	return true
}

// runPlain prints state changes line by line, for consoles that cannot draw the
// screen.
func runPlain(ctx context.Context, opts Options, updates <-chan supervisor.Status, profileName string, trayErr error, apply ui.ApplyToken) {
	if trayErr != nil {
		fmt.Fprintf(opts.Err, "bandeja indisponível: %s\n", errtext.Translate(trayErr))
	}
	fmt.Fprintf(opts.Out, "\nperfil  %s\n\n", profileName)

	// Without a screen to hold a field, the token is read from stdin, which the
	// plain mode never uses for anything else.
	typed := readLines(ctx, opts.In, apply != nil)

	previous := supervisor.Status{State: -1}
	// A retry failing the same way is not repeated. A renewal success never clears
	// LastError, so while healthy a new LastErrorAt is a new failure.
	lastReason := ""
	for {
		select {
		case <-ctx.Done():
			return

		case line, ok := <-typed:
			if !ok {
				typed = nil
				continue
			}
			if line == "" {
				continue
			}
			if err := apply(line); err != nil {
				fmt.Fprintf(opts.Err, "  %s\n", errtext.Translate(err))
			}

		case st, ok := <-updates:
			if !ok {
				return
			}
			if st.TokenAsk != supervisor.TokenFine && previous.TokenAsk != st.TokenAsk && apply != nil {
				fmt.Fprintln(opts.Out, "\n  "+tokenAskText(st.TokenAsk))
				fmt.Fprintln(opts.Out, "  Cole um token de conexão novo (dgv1_…) e tecle Enter:")
			}
			if st.Progress != "" && st.Progress != previous.Progress {
				fmt.Fprintf(opts.Out, "  · %s\n", st.Progress)
			}
			if st.Warning != "" && st.Warning != previous.Warning {
				fmt.Fprintf(opts.Err, "  aviso: %s\n", st.Warning)
			}
			reason := errtext.Translate(st.LastError)
			newRenewalFailure := st.State == supervisor.StateHealthy && !st.LastErrorAt.Equal(previous.LastErrorAt)
			if reason != lastReason || (reason != "" && newRenewalFailure) {
				if reason != "" {
					fmt.Fprintf(opts.Err, "  erro: %s\n", reason)
				}
				lastReason = reason
			}
			changed := st.State != previous.State
			previous = st
			if !changed {
				continue
			}

			name := ui.StateLabel(st.State)
			switch st.State {
			case supervisor.StateHealthy:
				fmt.Fprintf(opts.Out, "  ✓ %s\n", name)
			case supervisor.StateDegraded:
				fmt.Fprintf(opts.Out, "  ◐ %s (tentativa %d)\n", name, st.Attempt)
			case supervisor.StateFallback:
				fmt.Fprintf(opts.Out, "  ○ %s — Discord devolvido à rede normal, seguindo tentativas\n", name)
			}
		}
	}
}
