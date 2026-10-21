package control

import (
	"errors"
	"os"
	"sync"

	"github.com/bumaruf/discord-go-vpn/internal/connectiontoken"
	"github.com/bumaruf/discord-go-vpn/internal/startup"
	"github.com/bumaruf/discord-go-vpn/internal/supervisor"
)

// SessionEnv reads this process's desktop session, for OpConnect.
func SessionEnv() map[string]string {
	env := map[string]string{}
	for _, name := range []string{"DISPLAY", "XAUTHORITY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "XDG_SESSION_TYPE", "DBUS_SESSION_BUS_ADDRESS", "LANG"} {
		if v := os.Getenv(name); v != "" {
			env[name] = v
		}
	}
	return env
}

// Remote is the service's tunnel as a client sees it, with the methods the
// screen reads a supervisor by.
type Remote struct {
	conn    *Conn
	updates chan supervisor.Status

	mu      sync.Mutex
	status  supervisor.Status
	reading supervisor.Reading
	leasing bool
}

// ErrStopped is a start the service gave up on, or a tunnel taken down from
// another client.
var ErrStopped = errors.New("o serviço encerrou o túnel")

// Attach starts the tunnel if the service is idle and follows its startup,
// replaying the steps to r, until the session is up. A Session with Restart
// set means the service is relaunching into a newer release.
func Attach(conn *Conn, r startup.Reporter) (*Remote, *Session, error) {
	var hello Event
	if err := conn.Receive(&hello); err != nil || hello.Hello == nil {
		return nil, nil, errors.New("o serviço não se apresentou")
	}
	if hello.Hello.Phase == PhaseIdle {
		if err := conn.Send(Request{Op: OpConnect, Env: SessionEnv()}); err != nil {
			return nil, nil, err
		}
	}

	rm := &Remote{conn: conn, updates: make(chan supervisor.Status, 8)}
	for {
		var ev Event
		if err := conn.Receive(&ev); err != nil {
			return nil, nil, ErrStopped
		}
		switch {
		case ev.Step != nil:
			r.Step(startup.Step(ev.Step.Step), startup.State(ev.Step.State), ev.Step.Detail, ev.Step.Fraction)
		case ev.Tell != "":
			r.Tell(ev.Tell)
		case ev.Ask:
			// Asked off the read loop, which keeps taking events meanwhile.
			go func() {
				if typed, err := r.Ask(); err == nil {
					_ = conn.Send(Request{Op: OpToken, Token: typed})
				}
			}()
		case ev.Session != nil:
			if ev.Session.Error != "" {
				return nil, nil, errors.New(ev.Session.Error)
			}
			if ev.Session.Restart != "" {
				return nil, ev.Session, nil
			}
			rm.leasing = ev.Session.Leasing
			go rm.follow()
			return rm, ev.Session, nil
		}
	}
}

// follow keeps the status and reading current, and closes Updates once the
// tunnel ends or the service goes away.
func (rm *Remote) follow() {
	defer close(rm.updates)
	for {
		var ev Event
		if err := rm.conn.Receive(&ev); err != nil {
			return
		}
		switch {
		case ev.Status != nil:
			st := ev.Status.Supervisor()
			rm.mu.Lock()
			rm.status = st
			rm.mu.Unlock()
			select {
			case rm.updates <- st:
			default:
			}
		case ev.Reading != nil:
			rm.mu.Lock()
			rm.reading = *ev.Reading
			rm.mu.Unlock()
		case ev.Hello != nil && ev.Hello.Phase == PhaseIdle:
			return
		}
	}
}

func (rm *Remote) Updates() <-chan supervisor.Status { return rm.updates }

func (rm *Remote) Status() supervisor.Status {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	return rm.status
}

func (rm *Remote) Reading() supervisor.Reading {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	return rm.reading
}

func (rm *Remote) Reconnect() { _ = rm.conn.Send(Request{Op: OpReconnect}) }

// Leasing says a token is in use, which only then can be replaced.
func (rm *Remote) Leasing() bool { return rm.leasing }

// UseToken checks the token here, for an error the person reads at once, and
// hands it to the service.
func (rm *Remote) UseToken(typed string) error {
	if _, err := connectiontoken.Parse(typed); err != nil {
		return err
	}
	return rm.conn.Send(Request{Op: OpToken, Token: typed})
}

func (rm *Remote) Close() error { return rm.conn.Close() }
