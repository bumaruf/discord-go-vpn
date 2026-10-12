// Package profile decides where the tunnel configuration comes from: a .conf
// file, which wins when present, or a lease from provisiond.
package profile

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/bumaruf/discord-go-vpn/internal/wg"
)

type Source struct {
	Config *wg.Config
	Origin string

	// RenewFailures delivers the lease's failed renewals; nil for a file profile.
	RenewFailures <-chan error

	closeFn func()
}

// Close releases the lease, if any. It is safe to call more than once.
func (s *Source) Close() {
	if s.closeFn != nil {
		s.closeFn()
		s.closeFn = nil
	}
}

// Provisioning is where to lease from, as the connection token carries it: the server, its
// certificate fingerprint and the invite to present. Label is the device's name in the
// panel, which the token doesn't carry; empty means the hostname.
type Provisioning struct {
	Server      string
	Token       string
	Fingerprint string
	Label       string
}

func (p Provisioning) Configured() bool {
	return p.Server != "" && p.Token != "" && p.Fingerprint != ""
}

func Open(path string, prov Provisioning) (*Source, error) {
	if path != "" {
		cfg, err := wg.Load(path)
		if err != nil {
			return nil, err
		}
		return &Source{Config: cfg, Origin: "arquivo próprio (" + filepath.Base(path) + ")"}, nil
	}

	if prov.Configured() {
		client := &Client{Server: prov.Server, Token: prov.Token, Fingerprint: prov.Fingerprint, Label: prov.Label}
		lease, err := client.Acquire()
		if err != nil {
			return nil, fmt.Errorf("não consegui obter um acesso: %w", err)
		}
		return &Source{
			Config:        lease.Config,
			Origin:        LeaseOrigin,
			RenewFailures: lease.RenewFailures(),
			closeFn:       lease.Release,
		}, nil
	}

	return nil, ErrNoProfile
}

// LeaseOrigin names a profile leased from provisiond on screen.
const LeaseOrigin = "acesso do servidor"

// ErrNoProfile separates "there is nowhere to get a configuration from" from a
// read failure, because the advice differs in each case.
var ErrNoProfile = errors.New("nenhum perfil disponível")

// HelpText is shown when there is neither a profile nor a connection token.
const HelpText = `nenhum perfil .conf encontrado e nenhum token de conexão informado.

  o que fazer: cole o token de conexão (dgv1_...) que você recebeu quando o
  programa pedir, ou passe com -token dgv1_...

  se você tem um servidor WireGuard próprio, deixe o .conf na pasta deste
  programa ou aponte o caminho:  -c /caminho/arquivo.conf`
