// Package control is how the Linux service talks to its clients, the tray and
// the screen: one JSON object per line over a Unix socket. Both ends are this
// same binary, so the shapes change together and need no fixture.
package control

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/errtext"
	"github.com/bumaruf/discord-go-vpn/internal/supervisor"
)

// SocketPath is where the service listens; systemd creates its directory.
const SocketPath = "/run/discord-go-vpn/control.sock"

// Ops a client can ask for.
const (
	OpConnect    = "connect"
	OpDisconnect = "disconnect"
	OpReconnect  = "reconnect"
	// OpToken answers an Ask, or replaces the token of a running tunnel.
	OpToken = "token"
)

type Request struct {
	Op    string `json:"op"`
	Token string `json:"token,omitempty"`
	// Env is the desktop session Discord opens in, sent with OpConnect: under
	// systemd the service has none of its own.
	Env map[string]string `json:"env,omitempty"`
}

// Event is one of its fields at a time.
type Event struct {
	Hello   *Hello              `json:"hello,omitempty"`
	Step    *Step               `json:"step,omitempty"`
	Ask     bool                `json:"ask,omitempty"`
	Tell    string              `json:"tell,omitempty"`
	Session *Session            `json:"session,omitempty"`
	Status  *Status             `json:"status,omitempty"`
	Reading *supervisor.Reading `json:"reading,omitempty"`
}

// Phases of the service.
const (
	PhaseIdle     = "idle"
	PhaseStarting = "starting"
	PhaseRunning  = "running"
)

// Hello opens every connection.
type Hello struct {
	Version string `json:"version"`
	Phase   string `json:"phase"`
}

// Step mirrors a startup.Reporter call.
type Step struct {
	Step     int     `json:"step"`
	State    int     `json:"state"`
	Detail   string  `json:"detail"`
	Fraction float64 `json:"fraction"`
}

// Session ends startup: a profile to draw, a failure, or a new version the
// service is about to relaunch into.
type Session struct {
	Profile string `json:"profile,omitempty"`
	// Leasing says a token is in use, so the screen may offer to replace it.
	Leasing bool   `json:"leasing,omitempty"`
	Error   string `json:"error,omitempty"`
	Restart string `json:"restart,omitempty"`
}

// Status is supervisor.Status with the error as the text a person reads.
type Status struct {
	State           supervisor.State    `json:"state"`
	Attempt         int                 `json:"attempt"`
	Failures        int                 `json:"failures"`
	Since           time.Time           `json:"since"`
	LastError       string              `json:"last_error,omitempty"`
	LastErrorAt     time.Time           `json:"last_error_at"`
	DiscordTunneled bool                `json:"discord_tunneled"`
	OpeningDiscord  bool                `json:"opening_discord"`
	Progress        string              `json:"progress,omitempty"`
	Warning         string              `json:"warning,omitempty"`
	TokenAsk        supervisor.TokenAsk `json:"token_ask"`
	Endpoint        string              `json:"endpoint,omitempty"`
	Address         string              `json:"address,omitempty"`
}

func StatusFrom(s supervisor.Status) Status {
	return Status{
		State: s.State, Attempt: s.Attempt, Failures: s.Failures, Since: s.Since,
		LastError: errtext.Translate(s.LastError), LastErrorAt: s.LastErrorAt,
		DiscordTunneled: s.DiscordTunneled, OpeningDiscord: s.OpeningDiscord,
		Progress: s.Progress, Warning: s.Warning, TokenAsk: s.TokenAsk,
		Endpoint: s.Endpoint, Address: s.Address,
	}
}

func (s Status) Supervisor() supervisor.Status {
	var err error
	if s.LastError != "" {
		err = errors.New(s.LastError)
	}
	return supervisor.Status{
		State: s.State, Attempt: s.Attempt, Failures: s.Failures, Since: s.Since,
		LastError: err, LastErrorAt: s.LastErrorAt,
		DiscordTunneled: s.DiscordTunneled, OpeningDiscord: s.OpeningDiscord,
		Progress: s.Progress, Warning: s.Warning, TokenAsk: s.TokenAsk,
		Endpoint: s.Endpoint, Address: s.Address,
	}
}

// Conn is one end of the socket, safe to Send on from several goroutines.
type Conn struct {
	conn net.Conn
	mu   sync.Mutex
	enc  *json.Encoder
	dec  *json.Decoder
}

func NewConn(c net.Conn) *Conn {
	return &Conn{conn: c, enc: json.NewEncoder(c), dec: json.NewDecoder(bufio.NewReader(c))}
}

// Dial reaches the service. It fails when the service isn't installed or
// running, which callers take as "run on your own".
func Dial() (*Conn, error) { return DialPath(SocketPath) }

func DialPath(path string) (*Conn, error) {
	c, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		return nil, err
	}
	return NewConn(c), nil
}

// Send writes v as one line.
func (c *Conn) Send(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.enc.Encode(v); err != nil {
		return fmt.Errorf("falando com o serviço: %w", err)
	}
	return nil
}

// Receive reads the next line into v.
func (c *Conn) Receive(v any) error { return c.dec.Decode(v) }

func (c *Conn) Close() error { return c.conn.Close() }

// Raw is the socket, for reading the peer's credentials.
func (c *Conn) Raw() net.Conn { return c.conn }
