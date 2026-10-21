// Package app wires startup, the supervisor, the screen and the tray, and owns
// the process lifetime.
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
	"sync"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"

	"github.com/bumaruf/discord-go-vpn/internal/connectiontoken"
	"github.com/bumaruf/discord-go-vpn/internal/errtext"
	"github.com/bumaruf/discord-go-vpn/internal/profile"
	"github.com/bumaruf/discord-go-vpn/internal/routing"
	"github.com/bumaruf/discord-go-vpn/internal/startup"
	"github.com/bumaruf/discord-go-vpn/internal/statustext"
	"github.com/bumaruf/discord-go-vpn/internal/supervisor"
	"github.com/bumaruf/discord-go-vpn/internal/ui"
	"github.com/bumaruf/discord-go-vpn/internal/update"
)

// Options carries what the command line decided.
type Options struct {
	// ConfigPath is a .conf to use; empty leases with a connection token.
	ConfigPath  string
	DiscordPath string
	Mode        routing.Mode
	TokenFlag   string
	CheckUpdate bool

	Out io.Writer
	Err io.Writer
	// In is where the plain mode reads a replacement token from.
	In io.Reader
}

// run is what startup left running, for Run to take down.
type run struct {
	opts Options
	ctx  context.Context

	mu       sync.Mutex
	booted   chan struct{} // closed when boot returns; nil until it starts
	sup      *supervisor.Supervisor
	done     chan struct{} // closed when the supervisor stops
	stopTray func()
	restart  *update.Installed
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

	r := &run{opts: opts, ctx: ctx}
	code := 0
	if !r.screen(cancel) {
		code = r.plain()
	}

	cancel()
	r.wait()
	if r.restart != nil {
		code, err := r.restart.Relaunch()
		if err != nil {
			fmt.Fprintln(opts.Err, errtext.Translate(err))
		}
		return code
	}
	fmt.Fprintln(opts.Out, "encerrado.")
	return code
}

// boot runs startup and starts the tunnel and the tray, reporting through r.
func (r *run) boot(rep startup.Reporter) (ui.Session, error) {
	r.mu.Lock()
	r.booted = make(chan struct{})
	r.mu.Unlock()
	defer close(r.booted)

	res, err := startup.Run(r.ctx, startup.Options{
		ConfigPath: r.opts.ConfigPath, TokenFlag: r.opts.TokenFlag, CheckUpdate: r.opts.CheckUpdate,
	}, rep)
	switch {
	case err != nil:
		return ui.Session{}, err
	case res.Restart != nil:
		r.restart = res.Restart
		return ui.Session{Restart: res.Restart.Version}, nil
	case r.ctx.Err() != nil:
		if res.Source != nil {
			res.Source.Close()
		}
		return ui.Session{}, r.ctx.Err()
	}

	sup := supervisor.New(supervisor.Options{
		ConfigPath:  r.opts.ConfigPath,
		DiscordPath: r.opts.DiscordPath,
		Mode:        r.opts.Mode,
		Provision:   res.Provision,
		Source:      res.Source,
	})
	stopTray, trayErr := startTray(sup)
	done := make(chan struct{})
	go func() {
		sup.Run(r.ctx)
		close(done)
	}()

	r.mu.Lock()
	r.sup, r.done, r.stopTray = sup, done, stopTray
	r.mu.Unlock()

	sess := ui.Session{Sup: sup, Profile: res.Origin, TrayHint: stopTray != nil, TrayErr: trayErr}
	// A .conf takes no token, so there is nothing to replace.
	if r.opts.ConfigPath == "" && res.Provision.Configured() {
		sess.ApplyToken = applyToken(sup)
	}
	return sess, nil
}

// wait returns once whatever boot started has stopped.
func (r *run) wait() {
	r.mu.Lock()
	booted := r.booted
	r.mu.Unlock()
	if booted == nil {
		return
	}
	<-booted

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done != nil {
		<-r.done
	}
	if r.stopTray != nil {
		r.stopTray()
	}
}

func (r *run) started() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.booted != nil
}

// screen reports false when the terminal cannot draw it, so the caller falls
// back to plain text; startup runs only once the screen is up, so nothing is
// left half done then.
func (r *run) screen(cancel context.CancelFunc) bool {
	if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
		return false
	}
	m := ui.New(r.ctx, ui.Options{Version: update.Version, Boot: r.boot, Stop: cancel})
	if _, err := tea.NewProgram(m).Run(); err != nil && !r.started() {
		fmt.Fprintln(r.opts.Err, "a interface não pôde ser desenhada, seguindo em modo texto")
		return false
	}
	return true
}

// plain runs startup and the tunnel printing line by line, for consoles that
// cannot draw the screen.
func (r *run) plain() int {
	sess, err := r.boot(plainReporter{out: r.opts.Out, prompt: connectiontoken.Terminal()})
	if err != nil {
		if errors.Is(err, profile.ErrNoProfile) {
			fmt.Fprintln(r.opts.Err, profile.HelpText)
		} else if r.ctx.Err() == nil {
			fmt.Fprintln(r.opts.Err, "erro:", errtext.Translate(err))
		}
		return 1
	}
	if sess.Sup == nil {
		return 0
	}
	runPlain(r.ctx, r.opts, sess.Sup.Updates(), sess.Profile, sess.TrayErr, sess.ApplyToken)
	return 0
}

// plainReporter prints each step once it settles, and a running step when its
// line changes, but not every percent of a download.
type plainReporter struct {
	out    io.Writer
	prompt connectiontoken.Prompter
}

func (p plainReporter) Step(_ startup.Step, state startup.State, detail string, fraction float64) {
	switch state {
	case startup.Running:
		if fraction <= 0 {
			fmt.Fprintf(p.out, "  · %s\n", detail)
		}
	case startup.Done:
		fmt.Fprintf(p.out, "  ✓ %s\n", detail)
	case startup.Skipped:
		fmt.Fprintf(p.out, "  – %s\n", detail)
	case startup.Failed:
		fmt.Fprintf(p.out, "  ✗ %s\n", detail)
	}
}

func (p plainReporter) Tell(msg string)      { p.prompt.Tell(msg) }
func (p plainReporter) Ask() (string, error) { return p.prompt.Ask() }

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
				fmt.Fprintln(opts.Out, "\n  "+statustext.TokenAskReason(st.TokenAsk))
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

			name := statustext.StateLabel(st.State)
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
