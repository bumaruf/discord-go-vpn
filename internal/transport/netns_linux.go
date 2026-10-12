//go:build linux && !android

package transport

import (
	"errors"

	"github.com/bumaruf/discord-go-vpn/internal/transport/netns"
	"github.com/bumaruf/discord-go-vpn/internal/wg"
)

type nsTransport struct {
	ns *netns.Namespace
}

func newNetns() (Transport, error) {
	if err := netns.Supported(); err != nil {
		return nil, err
	}
	return &nsTransport{}, nil
}

func (n *nsTransport) Start(cfg *wg.Config) error {
	ns, err := netns.Start(cfg)
	if err != nil {
		return err
	}
	n.ns = ns
	return nil
}

func (n *nsTransport) Ready() error {
	if n.ns == nil {
		return errors.New("o namespace não chegou a subir")
	}
	return nil
}

func (n *nsTransport) Open(discordPath string) (string, error) { return n.ns.Open(discordPath) }

func (n *nsTransport) Close() {
	if n.ns != nil {
		n.ns.Close()
		n.ns = nil
	}
}
