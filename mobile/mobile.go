//go:build linux

// Package mobile is the Android app's Go side, bound with gomobile into the .aar the app
// links. It runs the same supervisor, lease and token code as the desktop client;
// the app draws the screen and owns the VpnService.
//
// gomobile binds only basic types, so lists cross as comma-separated strings and
// errors carry text already translated for the person.
package mobile

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/connectiontoken"
	"github.com/bumaruf/discord-go-vpn/internal/errtext"
	"github.com/bumaruf/discord-go-vpn/internal/profile"
	"github.com/bumaruf/discord-go-vpn/internal/routing"
	"github.com/bumaruf/discord-go-vpn/internal/statustext"
	"github.com/bumaruf/discord-go-vpn/internal/supervisor"
	"github.com/bumaruf/discord-go-vpn/internal/transport"
	"github.com/bumaruf/discord-go-vpn/internal/transport/vpnservice"
	"github.com/bumaruf/discord-go-vpn/internal/wg"
)

const (
	StateStarting = int(supervisor.StateStarting)
	StateHealthy  = int(supervisor.StateHealthy)
	StateDegraded = int(supervisor.StateDegraded)
	StateFallback = int(supervisor.StateFallback)
	StateStopped  = int(supervisor.StateStopped)

	TokenFine       = int(supervisor.TokenFine)
	TokenRefused    = int(supervisor.TokenRefused)
	TokenUnanswered = int(supervisor.TokenUnanswered)
)

// message is an error whose text the person reads as is, ending in a full stop.
type message string

func (m message) Error() string { return string(m) }

const (
	errMalformed = message(connectiontoken.MalformedHelp)
	errRefused   = message(connectiontoken.RefusedHelp)
)

// Host is the app's VpnService.
type Host interface {
	// Establish creates the VPN interface with Discord as its only app and returns
	// its descriptor, which Go then owns. addresses, routes and dns are
	// comma-separated.
	Establish(addresses, routes, dns string, mtu int) (int, error)
	// OpenDiscord brings Discord to the front when the app is on screen.
	OpenDiscord()
	// Update receives every status change, from a Go thread.
	Update(status *Status)
}

// Status is a snapshot of supervisor.Status in the words the screen shows. Times
// are Unix milliseconds, 0 when unset.
type Status struct {
	State           int
	Label           string
	Attempt         int
	Since           int64
	DiscordTunneled bool
	LastError       string
	LastErrorAt     int64
	Progress        string
	Warning         string
	TokenAsk        int
	TokenAskReason  string
	Profile         string
	Server          string
}

// Init points the saved token at the app's own files: an Android app process has
// no HOME for os.UserConfigDir to find.
func Init(filesDir string) error {
	return os.Setenv("XDG_CONFIG_HOME", filesDir)
}

func SavedToken() string { return connectiontoken.Saved() }

func ForgetToken() { connectiontoken.Forget() }

// CheckToken rejects a malformed token before anything leaves the phone.
func CheckToken(typed string) error {
	if _, err := connectiontoken.Parse(typed); err != nil {
		return errMalformed
	}
	return nil
}

func StateLabel(state int) string { return statustext.StateLabel(supervisor.State(state)) }

// Duration formats an elapsed time in milliseconds as the desktop screen does.
func Duration(ms int64) string { return statustext.Duration(time.Duration(ms) * time.Millisecond) }

// Session is one run of the supervisor, from Connect to Stop.
type Session struct {
	sup    *supervisor.Supervisor
	cancel context.CancelFunc
	done   chan struct{}
	label  string

	mu     sync.Mutex
	server string
}

// Connect validates the token by leasing, as the desktop prompt does, and starts
// the supervisor on that lease. A refusal forgets the saved token and returns
// connectiontoken.RefusedHelp; no answer keeps it and starts anyway, retrying.
// It blocks on the network, so the app calls it off the main thread.
func Connect(typed, label string, host Host) (*Session, error) {
	token, err := connectiontoken.Parse(typed)
	if err != nil {
		return nil, errMalformed
	}
	prov := token.Provisioning()
	prov.Label = label

	result, source := connectiontoken.Validate(prov)
	if result == connectiontoken.Refused {
		connectiontoken.Forget()
		return nil, errRefused
	}
	connectiontoken.Save(token)

	s := &Session{label: label, server: serverHost(token)}
	s.start(supervisor.Options{Provision: prov, Source: source}, host, profile.LeaseOrigin)
	return s, nil
}

// ConnectFile runs on a .conf profile, which takes no token.
func ConnectFile(path string, host Host) (*Session, error) {
	source, err := profile.Open(path, profile.Provisioning{})
	if err != nil {
		return nil, errors.New(errtext.Translate(err))
	}
	s := &Session{server: endpointHost(source.Config)}
	s.start(supervisor.Options{ConfigPath: path, Source: source}, host, source.Origin)
	return s, nil
}

func (s *Session) start(opts supervisor.Options, host Host, origin string) {
	opts.NewTransport = func(routing.Mode, transport.Reporter) (transport.Transport, error) {
		return vpnservice.New(hostAdapter{host}), nil
	}
	// Closing the transport already took Discord out of the tunnel: without the
	// interface Android sends its traffic through the normal network.
	opts.RestartDiscord = func(string) (string, error) { return "", nil }

	ctx, cancel := context.WithCancel(context.Background())
	s.sup = supervisor.New(opts)
	s.cancel = cancel
	s.done = make(chan struct{})

	host.Update(s.status(s.sup.Status(), origin))
	go s.sup.Run(ctx)
	go func() {
		defer close(s.done)
		for st := range s.sup.Updates() {
			host.Update(s.status(st, origin))
		}
	}()
}

// Reconnect tries now instead of waiting out the backoff.
func (s *Session) Reconnect() { s.sup.Reconnect() }

// UseToken leases with a pasted token from the next attempt on, saving it as the
// desktop screen does.
func (s *Session) UseToken(typed string) error {
	token, err := connectiontoken.Parse(typed)
	if err != nil {
		return errMalformed
	}
	connectiontoken.Save(token)

	s.mu.Lock()
	s.server = serverHost(token)
	s.mu.Unlock()

	prov := token.Provisioning()
	prov.Label = s.label
	s.sup.UseToken(prov)
	return nil
}

// Stop tears the tunnel down and releases the lease, returning once both are done.
func (s *Session) Stop() {
	s.cancel()
	<-s.done
}

func (s *Session) status(st supervisor.Status, origin string) *Status {
	s.mu.Lock()
	server := s.server
	s.mu.Unlock()

	out := &Status{
		State:           int(st.State),
		Label:           statustext.StateLabel(st.State),
		Attempt:         st.Attempt,
		Since:           unixMilli(st.Since),
		DiscordTunneled: st.DiscordTunneled,
		LastErrorAt:     unixMilli(st.LastErrorAt),
		Progress:        st.Progress,
		Warning:         st.Warning,
		TokenAsk:        int(st.TokenAsk),
		Profile:         origin,
		Server:          server,
	}
	if st.LastError != nil {
		out.LastError = errtext.Translate(st.LastError)
	}
	if st.TokenAsk != supervisor.TokenFine {
		out.TokenAskReason = statustext.TokenAskReason(st.TokenAsk)
	}
	return out
}

type hostAdapter struct{ host Host }

func (h hostAdapter) Establish(i vpnservice.Interface) (int, error) {
	return h.host.Establish(strings.Join(i.Addresses, ","), strings.Join(i.Routes, ","), strings.Join(i.DNS, ","), i.MTU)
}

func (h hostAdapter) OpenDiscord() error {
	h.host.OpenDiscord()
	return nil
}

func serverHost(token connectiontoken.Token) string {
	u, err := url.Parse(token.Server)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func endpointHost(cfg *wg.Config) string {
	host, _, err := net.SplitHostPort(cfg.Peer.Endpoint)
	if err != nil {
		return cfg.Peer.Endpoint
	}
	return host
}

func unixMilli(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}
