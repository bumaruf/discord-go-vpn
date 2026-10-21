//go:build linux && !android

// Package service holds the tunnel as root under systemd, so the person runs
// nothing with sudo. It stays idle until a client, the tray or the screen, asks
// to connect, and streams what happens to every client on the control socket.
package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/user"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/bumaruf/discord-go-vpn/internal/connectiontoken"
	"github.com/bumaruf/discord-go-vpn/internal/control"
	"github.com/bumaruf/discord-go-vpn/internal/errtext"
	"github.com/bumaruf/discord-go-vpn/internal/routing"
	"github.com/bumaruf/discord-go-vpn/internal/startup"
	"github.com/bumaruf/discord-go-vpn/internal/supervisor"
	"github.com/bumaruf/discord-go-vpn/internal/transport"
	"github.com/bumaruf/discord-go-vpn/internal/update"
)

type Options struct {
	// User owns the desktop Discord runs in. Only they and root may use the
	// socket, and Discord is opened as them.
	User        string
	ConfigPath  string
	DiscordPath string
	Mode        routing.Mode
	Version     string

	// Socket defaults to control.SocketPath; CheckUpdate is off in tests.
	Socket      string
	CheckUpdate bool

	// As in supervisor.Options; nil keeps the real ones.
	NewTransport   func(mode routing.Mode, report transport.Reporter) (transport.Transport, error)
	RestartDiscord func(explicit string) (string, error)
}

// sessionEnv is what a client may set of the desktop session: the variables
// graphicalEnv in internal/discord hands Discord.
var sessionEnv = []string{"DISPLAY", "XAUTHORITY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "XDG_SESSION_TYPE", "DBUS_SESSION_BUS_ADDRESS", "LANG"}

const (
	readingEvery = time.Second
	clientQueue  = 256
)

type Service struct {
	opts Options
	uid  int

	ctx         context.Context
	quit        chan struct{} // closed to stop serving, for a restart
	stopServing sync.Once

	mu      sync.Mutex
	clients map[*client]struct{}
	phase   string
	cancel  context.CancelFunc // ends the current start or tunnel
	done    chan struct{}      // closed when it has ended
	sup     *supervisor.Supervisor
	leasing bool
	answers chan string // tokens typed while startup asks
	asking  bool
	restart *update.Installed

	// What a client connecting midway needs to catch up.
	steps   [2]*control.Step
	tell    string
	session *control.Session
	status  *control.Status
}

type client struct {
	conn *control.Conn
	out  chan control.Event
}

// Run serves until ctx ends or startup installs a newer release, which it
// returns for the caller to relaunch once the tunnel is down.
func Run(ctx context.Context, opts Options) (*update.Installed, error) {
	if opts.Socket == "" {
		opts.Socket = control.SocketPath
	}
	u, err := user.Lookup(opts.User)
	if err != nil {
		return nil, fmt.Errorf("o usuário %q não existe: %w", opts.User, err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return nil, fmt.Errorf("uid %q de %s: %w", u.Uid, opts.User, err)
	}

	// internal/discord and osuser act for the person who invoked sudo; the
	// service stands in for that.
	_ = os.Setenv("SUDO_USER", u.Username)
	_ = os.Setenv("SUDO_UID", u.Uid)

	_ = os.Remove(opts.Socket)
	ln, err := net.Listen("unix", opts.Socket)
	if err != nil {
		return nil, fmt.Errorf("abrindo %s: %w", opts.Socket, err)
	}
	// Anyone may connect; acceptable checks who, by the socket's credentials.
	_ = os.Chmod(opts.Socket, 0o666)

	s := &Service{
		opts: opts, uid: uid, ctx: ctx, quit: make(chan struct{}),
		clients: map[*client]struct{}{}, phase: control.PhaseIdle,
	}
	go func() {
		select {
		case <-ctx.Done():
		case <-s.quit:
		}
		_ = ln.Close()
	}()

	for {
		c, err := ln.Accept()
		if err != nil {
			break
		}
		if !s.acceptable(c) {
			_ = c.Close()
			continue
		}
		go s.serve(control.NewConn(c))
	}

	s.disconnect()
	_ = os.Remove(opts.Socket)
	return s.restart, nil
}

// acceptable lets in root and the owner.
func (s *Service) acceptable(c net.Conn) bool {
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return false
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return false
	}
	var cred *unix.Ucred
	_ = raw.Control(func(fd uintptr) {
		cred, err = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})
	return err == nil && cred != nil && (cred.Uid == 0 || int(cred.Uid) == s.uid)
}

func (s *Service) serve(conn *control.Conn) {
	cl := &client{conn: conn, out: make(chan control.Event, clientQueue)}
	s.mu.Lock()
	cl.out <- control.Event{Hello: &control.Hello{Version: s.opts.Version, Phase: s.phase}}
	for _, ev := range s.replay() {
		cl.out <- ev
	}
	s.clients[cl] = struct{}{}
	s.mu.Unlock()

	go func() {
		for ev := range cl.out {
			if conn.Send(ev) != nil {
				break
			}
		}
		_ = conn.Close()
	}()

	for {
		var req control.Request
		if err := conn.Receive(&req); err != nil {
			break
		}
		s.handle(req)
	}

	s.mu.Lock()
	delete(s.clients, cl)
	close(cl.out)
	s.mu.Unlock()
}

// replay is what has happened in the current phase. Callers hold mu.
func (s *Service) replay() []control.Event {
	var out []control.Event
	for _, st := range s.steps {
		if st != nil {
			out = append(out, control.Event{Step: st})
		}
	}
	if s.tell != "" {
		out = append(out, control.Event{Tell: s.tell})
	}
	if s.asking {
		out = append(out, control.Event{Ask: true})
	}
	if s.session != nil {
		out = append(out, control.Event{Session: s.session})
	}
	if s.status != nil {
		out = append(out, control.Event{Status: s.status})
	}
	return out
}

// broadcast drops the event for a client too slow to take it: the next status
// carries the whole state.
func (s *Service) broadcast(ev control.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for cl := range s.clients {
		select {
		case cl.out <- ev:
		default:
		}
	}
}

func (s *Service) handle(req control.Request) {
	switch req.Op {
	case control.OpConnect:
		s.connect(req.Env)
	case control.OpDisconnect:
		s.disconnect()
	case control.OpReconnect:
		s.mu.Lock()
		sup := s.sup
		s.mu.Unlock()
		if sup != nil {
			sup.Reconnect()
		}
	case control.OpToken:
		s.useToken(req.Token)
	}
}

func (s *Service) useToken(typed string) {
	s.mu.Lock()
	asking, answers, sup, leasing := s.asking, s.answers, s.sup, s.leasing
	s.mu.Unlock()

	switch {
	case asking:
		select {
		case answers <- typed:
		default:
		}
	case sup != nil && leasing:
		token, err := connectiontoken.Parse(typed)
		if err != nil {
			s.broadcast(control.Event{Tell: errtext.Translate(err)})
			return
		}
		connectiontoken.Save(token)
		sup.UseToken(token.Provisioning())
	}
}

func (s *Service) connect(env map[string]string) {
	s.mu.Lock()
	if s.phase != control.PhaseIdle {
		s.mu.Unlock()
		return
	}
	for _, name := range sessionEnv {
		if v, ok := env[name]; ok {
			_ = os.Setenv(name, v)
		}
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.phase, s.cancel, s.done = control.PhaseStarting, cancel, make(chan struct{})
	s.steps, s.tell, s.session, s.status = [2]*control.Step{}, "", nil, nil
	s.answers = make(chan string, 1)
	done := s.done
	s.mu.Unlock()

	s.broadcast(control.Event{Hello: &control.Hello{Version: s.opts.Version, Phase: control.PhaseStarting}})
	go func() {
		defer close(done)
		s.run(ctx)
		s.mu.Lock()
		s.phase, s.sup, s.cancel = control.PhaseIdle, nil, nil
		s.mu.Unlock()
		s.broadcast(control.Event{Hello: &control.Hello{Version: s.opts.Version, Phase: control.PhaseIdle}})
	}()
}

// run is one start: startup, then the tunnel until ctx ends.
func (s *Service) run(ctx context.Context) {
	res, err := startup.Run(ctx, startup.Options{
		ConfigPath: s.opts.ConfigPath, CheckUpdate: s.opts.CheckUpdate,
	}, reporter{s, ctx})
	switch {
	case err != nil:
		s.endStart(&control.Session{Error: errtext.Translate(err)})
		return
	case res.Restart != nil:
		s.endStart(&control.Session{Restart: res.Restart.Version})
		s.mu.Lock()
		s.restart = res.Restart
		s.mu.Unlock()
		s.stopServing.Do(func() { close(s.quit) })
		return
	case ctx.Err() != nil:
		if res.Source != nil {
			res.Source.Close()
		}
		return
	}

	sup := supervisor.New(supervisor.Options{
		ConfigPath:     s.opts.ConfigPath,
		DiscordPath:    s.opts.DiscordPath,
		Mode:           s.opts.Mode,
		Provision:      res.Provision,
		Source:         res.Source,
		NewTransport:   s.opts.NewTransport,
		RestartDiscord: s.opts.RestartDiscord,
	})
	leasing := s.opts.ConfigPath == "" && res.Provision.Configured()
	s.mu.Lock()
	s.sup, s.leasing, s.phase = sup, leasing, control.PhaseRunning
	s.mu.Unlock()
	s.endStart(&control.Session{Profile: res.Origin, Leasing: leasing})

	forwarded := make(chan struct{})
	go func() {
		defer close(forwarded)
		for st := range sup.Updates() {
			status := control.StatusFrom(st)
			s.mu.Lock()
			s.status = &status
			s.mu.Unlock()
			s.broadcast(control.Event{Status: &status})
		}
	}()
	readings, stopReadings := context.WithCancel(ctx)
	go func() {
		t := time.NewTicker(readingEvery)
		defer t.Stop()
		for {
			select {
			case <-readings.Done():
				return
			case <-t.C:
				r := sup.Reading()
				s.broadcast(control.Event{Reading: &r})
			}
		}
	}()

	sup.Run(ctx)
	stopReadings()
	<-forwarded
}

func (s *Service) endStart(sess *control.Session) {
	s.mu.Lock()
	s.session = sess
	s.mu.Unlock()
	s.broadcast(control.Event{Session: sess})
}

// disconnect ends the current start or tunnel and waits for Discord to be back
// on the normal network.
func (s *Service) disconnect() {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-done
}

// reporter streams startup's steps to the clients and takes a token from any.
type reporter struct {
	s   *Service
	ctx context.Context
}

func (r reporter) Step(step startup.Step, state startup.State, detail string, fraction float64) {
	ev := &control.Step{Step: int(step), State: int(state), Detail: detail, Fraction: fraction}
	r.s.mu.Lock()
	r.s.steps[step] = ev
	r.s.mu.Unlock()
	r.s.broadcast(control.Event{Step: ev})
}

func (r reporter) Tell(msg string) {
	r.s.mu.Lock()
	r.s.tell = msg
	r.s.mu.Unlock()
	r.s.broadcast(control.Event{Tell: msg})
}

func (r reporter) Ask() (string, error) {
	r.s.mu.Lock()
	r.s.asking = true
	answers := r.s.answers
	r.s.mu.Unlock()
	r.s.broadcast(control.Event{Ask: true})

	defer func() {
		r.s.mu.Lock()
		r.s.asking, r.s.tell = false, ""
		r.s.mu.Unlock()
	}()
	select {
	case typed := <-answers:
		return typed, nil
	case <-r.ctx.Done():
		return "", errors.New("conexão cancelada")
	}
}
