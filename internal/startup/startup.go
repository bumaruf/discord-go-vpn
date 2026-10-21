// Package startup runs what comes before the tunnel, the update and getting
// access, reporting each step to whoever draws it: the screen or plain text.
package startup

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/bumaruf/discord-go-vpn/internal/connectiontoken"
	"github.com/bumaruf/discord-go-vpn/internal/errtext"
	"github.com/bumaruf/discord-go-vpn/internal/profile"
	"github.com/bumaruf/discord-go-vpn/internal/update"
)

type Step int

const (
	StepUpdate Step = iota
	StepAccess
)

type State int

const (
	Running State = iota
	Done
	Skipped
	// Failed is a step that went wrong; only a failed StepAccess stops startup.
	Failed
)

// NoFraction is a step whose share done is unknown.
const NoFraction = -1

// Reporter receives the steps as they go, and asks for a token when none works.
type Reporter interface {
	// Step reports a step's state with a line about it; fraction is the share
	// done, from 0 to 1, or NoFraction.
	Step(step Step, state State, detail string, fraction float64)
	// Tell and Ask are connectiontoken.Prompter's.
	Tell(msg string)
	Ask() (string, error)
}

type Options struct {
	// ConfigPath is a .conf to use, which takes no token; empty leases.
	ConfigPath  string
	TokenFlag   string
	CheckUpdate bool
}

type Result struct {
	// Restart is a newer release already installed: the caller relaunches it
	// instead of connecting.
	Restart *update.Installed

	Provision profile.Provisioning
	// Source is the profile opened along the way, or nil when the server could
	// not be reached; the caller owns it.
	Source *profile.Source
	Origin string
}

// Run fails only when there is no way to get access.
func Run(ctx context.Context, opts Options, r Reporter) (Result, error) {
	if installed := checkUpdate(ctx, opts.CheckUpdate, r); installed != nil {
		return Result{Restart: installed}, nil
	}
	return access(opts, r)
}

// checkUpdate runs before the tunnel exists, so updating never drops a call.
func checkUpdate(ctx context.Context, enabled bool, r Reporter) *update.Installed {
	switch {
	case !enabled:
		r.Step(StepUpdate, Skipped, "desligada pela opção -no-update", NoFraction)
		return nil
	case !update.Enabled():
		r.Step(StepUpdate, Skipped, "versão de desenvolvimento ("+update.Version+")", NoFraction)
		return nil
	}

	r.Step(StepUpdate, Running, "procurando versão nova…", NoFraction)
	// The download reports every read; only a new percent is worth a frame.
	shown := -1
	installed, err := update.Install(ctx, func(version string, done, total int64) {
		if total <= 0 {
			return
		}
		if pct := int(done * 100 / total); pct != shown {
			shown = pct
			r.Step(StepUpdate, Running, "baixando "+version, float64(done)/float64(total))
		}
	})
	switch {
	case err != nil:
		r.Step(StepUpdate, Failed, "não consegui atualizar: "+errtext.Translate(err), NoFraction)
		return nil
	case installed != nil:
		r.Step(StepUpdate, Done, "atualizado para "+installed.Version+", reabrindo…", 1)
		return installed
	}
	r.Step(StepUpdate, Done, "em dia ("+update.Version+")", NoFraction)
	return nil
}

func access(opts Options, r Reporter) (Result, error) {
	if opts.ConfigPath != "" {
		r.Step(StepAccess, Running, "lendo "+filepath.Base(opts.ConfigPath), NoFraction)
		source, err := profile.Open(opts.ConfigPath, profile.Provisioning{})
		if err != nil {
			r.Step(StepAccess, Failed, errtext.Translate(err), NoFraction)
			return Result{}, err
		}
		r.Step(StepAccess, Done, source.Origin, NoFraction)
		return Result{Source: source, Origin: source.Origin}, nil
	}

	r.Step(StepAccess, Running, "procurando o token de conexão…", NoFraction)
	token, lease, err := connectiontoken.Ensure(opts.TokenFlag, checking{r})
	if err != nil {
		r.Step(StepAccess, Failed, errtext.Translate(err), NoFraction)
		return Result{}, err
	}

	res := Result{Provision: token.Provisioning(), Source: lease, Origin: profile.LeaseOrigin}
	if lease == nil {
		// The supervisor keeps trying, so this is no reason to stop.
		r.Step(StepAccess, Failed, "o servidor não respondeu; sigo tentando ao conectar", NoFraction)
		return res, nil
	}
	r.Step(StepAccess, Done, "token aceito pelo servidor", NoFraction)
	return res, nil
}

// checking turns Ensure's announcement of a validation into a step update.
type checking struct{ Reporter }

func (c checking) Checking(origin string) {
	c.Step(StepAccess, Running, fmt.Sprintf("token %s · validando com o servidor…", origin), NoFraction)
}
