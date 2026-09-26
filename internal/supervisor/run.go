package supervisor

import (
	"context"
	"errors"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/profile"
	"github.com/bumaruf/discord-go-vpn/internal/transport"
)

// session holds what one successful connection owns.
type session struct {
	source *profile.Source
	tr     transport.Transport
}

func (s *session) close() {
	if s == nil {
		return
	}
	// Transport before profile: closing the lease first would leave the tunnel
	// pointed at a peer the server already removed.
	if s.tr != nil {
		s.tr.Close()
	}
	if s.source != nil {
		s.source.Close()
	}
}

// Run holds the tunnel up until the context is cancelled. It only returns when
// asked to stop -- every failure in between is handled, not returned.
func (s *Supervisor) Run(ctx context.Context) {
	defer s.closeUpdates()

	var current *session
	defer func() {
		current.close()
		if s.initial != nil {
			s.initial.Close()
		}
		s.set(func(st *Status) { st.State = StateStopped })

		_, _ = s.restartDiscord(s.opts.DiscordPath)
	}()

	backoff := backoffStart

	for {
		if ctx.Err() != nil {
			return
		}

		sess, err := s.connect(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}

			leasing := s.leasing()
			s.set(func(st *Status) {
				st.Failures++
				st.Attempt++
				st.LastError, st.LastErrorAt = err, time.Now()
				if st.Failures >= s.opts.MaxFailures {
					st.State = StateFallback
				} else {
					st.State = StateDegraded
				}
				st.TokenAsk = tokenAsk(leasing, err, st.Failures >= s.opts.MaxFailures)
			})

			if s.Status().State == StateFallback && s.Status().DiscordTunneled {
				if _, err := s.restartDiscord(s.opts.DiscordPath); err == nil {
					s.set(func(st *Status) { st.DiscordTunneled = false })
				}
			}

			alive, nudged := s.wait(ctx, backoff, s.nudge)
			if !alive {
				return
			}
			// A nudge says the network changed, so the backoff built up for the
			// old one starts over.
			if nudged {
				backoff = backoffStart
			} else {
				backoff = min(backoff*2, backoffMax)
			}
			continue
		}

		current = sess
		backoff = backoffStart

		// Discord is never inside a new session yet: after a drop it is still in
		// the closed netns on Linux, and on Windows its connections left through
		// the normal network while no filter was up.
		_, err = sess.tr.Open(s.opts.DiscordPath)
		s.set(func(st *Status) { st.DiscordTunneled = err == nil })

		s.set(func(st *Status) {
			st.State = StateHealthy
			st.Failures = 0
			st.LastError = nil
			st.TokenAsk = TokenFine
		})

		// This connection already served a nudge sent while connecting; left in
		// the channel, it would skip the next backoff.
		drain(s.nudge)

		s.watch(ctx, sess)

		// Closed before the next attempt: both transports tear down by fixed
		// name (the netns, every wiresock-client.exe), so a later Close would
		// take the replacement tunnel down, and the lease would stay held.
		current.close()
		current = nil
	}
}

// tokenAsk decides whether another token could get this attempt in. A refusal
// says so outright; repeated failures with no answer are asked about only once
// the retries have been given up on, since a server that is down looks the same
// as a token carrying the wrong address.
func tokenAsk(leasing bool, err error, gaveUp bool) TokenAsk {
	switch {
	case !leasing:
		return TokenFine
	case profile.IsRefusal(err):
		return TokenRefused
	case gaveUp:
		return TokenUnanswered
	default:
		return TokenFine
	}
}

// statusReporter keeps what a transport reports in the status: while Start
// runs the TUI owns the terminal, and writing there garbles the screen.
type statusReporter struct{ s *Supervisor }

func (r statusReporter) Progress(msg string) { r.s.set(func(st *Status) { st.Progress = msg }) }
func (r statusReporter) Warn(msg string)     { r.s.set(func(st *Status) { st.Warning = msg }) }

// connect brings up one attempt: profile, transport, route check.
func (s *Supervisor) connect(ctx context.Context) (*session, error) {
	s.set(func(st *Status) { st.State, st.Progress, st.Warning = StateStarting, "", "" })
	defer s.set(func(st *Status) { st.Progress = "" })

	source := s.initial
	s.initial = nil
	if source == nil {
		var err error
		if source, err = s.openProfile(s.opts.ConfigPath, s.currentProvision()); err != nil {
			return nil, err
		}
	}

	tr, err := s.newTransport(s.opts.Mode, statusReporter{s})
	if err != nil {
		source.Close()
		return nil, err
	}

	if err := tr.Start(source.Config); err != nil {
		source.Close()
		return nil, err
	}

	sess := &session{source: source, tr: tr}

	if err := tr.Ready(); err != nil {
		sess.close()
		return nil, err
	}

	return sess, nil
}

// watch probes the live tunnel and returns as soon as it stops answering.
func (s *Supervisor) watch(ctx context.Context, sess *session) {
	health := time.NewTicker(s.opts.ProbeEvery)
	defer health.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case err := <-sess.source.RenewFailures:
			s.set(func(st *Status) { st.LastError, st.LastErrorAt = err, time.Now() })
			// Any other failure leaves the lease the rest of its TTL for a later
			// renewal to succeed; reconnecting would drop a working tunnel.
			if errors.Is(err, profile.ErrLeaseLost) {
				return
			}

		case <-health.C:
			// Only liveness can be checked: the transports capture Discord's
			// processes, not this one, so a route probe from here would take
			// the normal network.
			if err := sess.tr.Ready(); err != nil {
				s.set(func(st *Status) { st.LastError, st.LastErrorAt = err, time.Now() })
				return
			}
		}
	}
}

// sleepCtx waits d, or less when ctx is cancelled or a nudge arrives.
func sleepCtx(ctx context.Context, d time.Duration, nudge <-chan struct{}) (alive, nudged bool) {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return false, false
	case <-nudge:
		return true, true
	case <-t.C:
		return true, false
	}
}

func drain(nudge <-chan struct{}) {
	select {
	case <-nudge:
	default:
	}
}
