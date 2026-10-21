// Package supervisor keeps the tunnel up. It reconnects with backoff and, after
// MaxFailures in a row, hands Discord back to the normal network while it keeps
// retrying; Discord moves back in once the tunnel returns.
package supervisor

import (
	"context"
	"sync"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/discord"
	"github.com/bumaruf/discord-go-vpn/internal/profile"
	"github.com/bumaruf/discord-go-vpn/internal/routing"
	"github.com/bumaruf/discord-go-vpn/internal/transport"
)

type State int

const (
	StateStarting State = iota
	StateHealthy
	StateDegraded // link lost, retrying, Discord still pointed at the tunnel
	StateFallback // gave up for now: Discord back on the normal network
	StateStopped
)

// TokenAsk says why a new connection token would help, so the screen can offer
// to replace the one in use.
type TokenAsk int

const (
	// TokenFine covers a working tunnel and a failure another token can't fix,
	// such as a .conf profile or a transport that won't start.
	TokenFine TokenAsk = iota
	// TokenRefused: the server turned this token down, so only another gets in.
	TokenRefused
	// TokenUnanswered: attempts keep failing without an answer, which a token
	// carrying the wrong address also looks like.
	TokenUnanswered
)

// Status is a snapshot, passed by value so the UI never shares memory the
// supervisor mutates.
type Status struct {
	State     State
	Attempt   int
	Failures  int
	Since     time.Time
	LastError error

	// DiscordTunneled tells whether the running client is inside the tunnel.
	DiscordTunneled bool
	// OpeningDiscord is set while a tunnel that just came up restarts Discord.
	OpeningDiscord bool

	LastErrorAt time.Time

	// Progress is the step a connection attempt is on while its transport
	// starts, such as installing WireSock; it clears when the attempt ends.
	Progress string

	// Warning is a problem the current connection runs despite. The next
	// attempt clears it.
	Warning string

	// TokenAsk is set only while leasing: a .conf profile takes no token.
	TokenAsk TokenAsk

	// Endpoint and Address describe the session up now; empty without one.
	Endpoint string
	Address  string
}

// Uptime is how long the current state has lasted.
func (s Status) Uptime() time.Duration {
	if s.Since.IsZero() {
		return 0
	}
	return time.Since(s.Since).Truncate(time.Second)
}

type Options struct {
	ConfigPath  string
	DiscordPath string
	Mode        routing.Mode
	Provision   profile.Provisioning

	// Source, when set, is a profile opened at startup for the first connection
	// to reuse instead of leasing again. Run owns it and closes it.
	Source *profile.Source

	// MaxFailures is how many consecutive failures are tolerated before Discord
	// is handed back to the normal network. Three by default: fewer would react
	// to a passing hiccup, more leaves the person staring at a dead client.
	MaxFailures int

	// ProbeEvery is the gap between health probes on a healthy tunnel.
	ProbeEvery time.Duration

	// NewTransport and RestartDiscord replace the desktop ones where Discord is put
	// in the tunnel some other way, as on Android; nil keeps the desktop ones.
	NewTransport   func(mode routing.Mode, report transport.Reporter) (transport.Transport, error)
	RestartDiscord func(explicit string) (string, error)
}

const (
	defaultMaxFailures = 3
	defaultProbeEvery  = 15 * time.Second

	// Backoff between reconnect attempts. It grows so a server that is down for
	// minutes is not hammered, and caps so recovery is never far away.
	backoffStart = 3 * time.Second
	backoffMax   = 30 * time.Second
)

type Supervisor struct {
	opts Options

	mu     sync.RWMutex
	status Status
	// provision is Options.Provision until UseToken replaces it, which the
	// connect loop reads on every attempt.
	provision profile.Provisioning

	updates chan Status
	once    sync.Once

	// nudge holds a pending Reconnect request.
	nudge chan struct{}

	// initial is Options.Source until connect takes it.
	initial *profile.Source

	// meterMu guards what Reading reads, apart from mu so a slow Traffic call
	// never delays a status update.
	meterMu sync.Mutex
	meter   transport.Meter
	rtt     time.Duration

	// Dependencies New wires to the real implementations; tests replace them.
	openProfile    func(path string, prov profile.Provisioning) (*profile.Source, error)
	newTransport   func(mode routing.Mode, report transport.Reporter) (transport.Transport, error)
	restartDiscord func(explicit string) (string, error)
	wait           func(ctx context.Context, d time.Duration, nudge <-chan struct{}) (alive, nudged bool)
}

func New(opts Options) *Supervisor {
	if opts.MaxFailures <= 0 {
		opts.MaxFailures = defaultMaxFailures
	}
	if opts.ProbeEvery <= 0 {
		opts.ProbeEvery = defaultProbeEvery
	}

	s := &Supervisor{
		opts:      opts,
		provision: opts.Provision,
		// Buffered, and set drops a frame rather than block: the next frame
		// carries the whole state.
		updates: make(chan Status, 8),
		nudge:   make(chan struct{}, 1),
		status:  Status{State: StateStarting, Since: time.Now()},
		initial: opts.Source,

		openProfile:    profile.Open,
		newTransport:   transport.New,
		restartDiscord: discord.RestartAsUser,
		wait:           sleepCtx,
	}
	if opts.NewTransport != nil {
		s.newTransport = opts.NewTransport
	}
	if opts.RestartDiscord != nil {
		s.restartDiscord = opts.RestartDiscord
	}
	return s
}

// Updates delivers a snapshot whenever something changes.
func (s *Supervisor) Updates() <-chan Status { return s.updates }

func (s *Supervisor) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// UseToken leases with prov from the next attempt on, which starts right away.
// The caller owns storing the token; the supervisor only uses it.
func (s *Supervisor) UseToken(prov profile.Provisioning) {
	s.mu.Lock()
	s.provision = prov
	s.mu.Unlock()

	// Cleared before the attempt, so the screen leaves the token field while it
	// runs and comes back only if this token is refused too.
	s.set(func(st *Status) { st.TokenAsk = TokenFine })
	s.Reconnect()
}

// leasing reports whether access comes from a token, the only case a new one
// can change anything.
func (s *Supervisor) leasing() bool {
	return s.opts.ConfigPath == "" && s.currentProvision().Configured()
}

func (s *Supervisor) currentProvision() profile.Provisioning {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.provision
}

// Reconnect asks for an attempt now, cutting the backoff short. It never blocks;
// requests made while one is pending are dropped.
func (s *Supervisor) Reconnect() {
	select {
	case s.nudge <- struct{}{}:
	default:
	}
}

func (s *Supervisor) set(mutate func(*Status)) {
	s.mu.Lock()
	before := s.status.State
	mutate(&s.status)
	if s.status.State != before {
		s.status.Since = time.Now()
	}
	snapshot := s.status
	s.mu.Unlock()

	select {
	case s.updates <- snapshot:
	default:
	}
}

func (s *Supervisor) closeUpdates() {
	s.once.Do(func() { close(s.updates) })
}
