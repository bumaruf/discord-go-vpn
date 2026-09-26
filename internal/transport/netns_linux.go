//go:build linux

package transport

import (
	"errors"
	"fmt"

	"github.com/bumaruf/discord-go-vpn/internal/discord"
	"github.com/bumaruf/discord-go-vpn/internal/errtext"
	"github.com/bumaruf/discord-go-vpn/internal/transport/netns"
	"github.com/bumaruf/discord-go-vpn/internal/wg"
)

type nsTransport struct {
	ns          *netns.Namespace
	discordPath string
	report      Reporter
}

func newNetns(discordPath string, report Reporter) (Transport, error) {
	if err := netns.Supported(); err != nil {
		return nil, err
	}
	return &nsTransport{discordPath: discordPath, report: report}, nil
}

// Start creates the namespace on the first connection; later ones reuse it.
func (n *nsTransport) Start(cfg *wg.Config) error {
	if n.ns == nil {
		ns, err := netns.Create()
		if err != nil {
			return err
		}
		n.ns = ns
	}
	return n.ns.Up(cfg)
}

func (n *nsTransport) Ready() error {
	if n.ns == nil || !n.ns.IsUp() {
		return errors.New("o namespace não chegou a subir")
	}
	return nil
}

func (n *nsTransport) Stop() {
	if n.ns != nil {
		n.ns.Down()
	}
}

// Enter only reconnects a Discord already inside the namespace: it loaded
// through the tunnel, so it carries the location of the tunnel's IP. Any other
// is reopened inside, since Discord reads the location again only on a fresh
// load (see wsTransport.Enter).
func (n *nsTransport) Enter() error {
	if !n.ns.Occupied() {
		_, err := n.ns.Open(n.discordPath)
		return err
	}
	n.ns.UseTunnel()
	n.reconnect()
	return nil
}

// Leave gives the namespace the normal network. Should that fail, Discord is
// reopened outside, and Enter brings it back in.
func (n *nsTransport) Leave() error {
	if n.ns == nil || !n.ns.Occupied() {
		return nil
	}
	if err := n.ns.UseHostNetwork(); err != nil {
		n.report.Warn(fmt.Sprintf("não consegui dar a rede normal ao namespace (%s); o Discord foi reaberto fora dele", errtext.Translate(err)))
		_, err := discord.RestartAsUser(n.discordPath)
		return err
	}
	n.reconnect()
	return nil
}

func (n *nsTransport) reconnect() {
	if err := n.ns.Reconnect(); err != nil {
		n.report.Warn(fmt.Sprintf("não consegui derrubar as conexões do Discord (%s); ele vai reconectar sozinho, mais devagar", errtext.Translate(err)))
	}
}

// Close reopens Discord outside when it was inside: deleting the namespace
// leaves whatever runs there without a network.
func (n *nsTransport) Close() {
	if n.ns == nil {
		return
	}
	inside := n.ns.Occupied()
	n.ns.Close()
	n.ns = nil
	if inside {
		_, _ = discord.RestartAsUser(n.discordPath)
	}
}
