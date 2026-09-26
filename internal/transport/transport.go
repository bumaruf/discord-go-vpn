// Package transport puts Discord inside the tunnel without the rest of the
// program knowing how: a network namespace on Linux, WireSock on Windows.
//
// A Transport lives as long as the program: each connection brings a tunnel up
// with Start and down with Stop, while Discord stays open and is only pointed
// at the tunnel or at the normal network, which makes it reconnect.
package transport

import (
	"fmt"

	"github.com/bumaruf/discord-go-vpn/internal/routing"
	"github.com/bumaruf/discord-go-vpn/internal/wg"
)

type Transport interface {
	// Start brings up one connection's tunnel.
	Start(cfg *wg.Config) error

	// Ready reports whether the tunnel is still up. It is separate from Start
	// because a transport can fail right after starting, as wiresock-client does.
	Ready() error

	// Stop takes the tunnel Start brought up down again.
	Stop()

	// Enter points Discord at the tunnel, opening it when it isn't running.
	Enter() error

	// Leave points Discord at the normal network while no tunnel is up.
	Leave() error

	// Close undoes everything, leaving Discord on the normal network.
	Close()
}

// Reporter receives what the transport has to tell the person while it works.
type Reporter interface {
	// Progress is a step of a slow setup, such as installing WireSock.
	Progress(msg string)
	// Warn is a problem the tunnel keeps running despite.
	Warn(msg string)
}

// New returns mode's transport. discordPath is the client to open, or "" to
// find it.
func New(mode routing.Mode, discordPath string, report Reporter) (Transport, error) {
	switch mode {
	case routing.ModeNetns:
		return newNetns(discordPath, report)
	case routing.ModeWiresock:
		return newWiresock(discordPath, report)
	}
	return nil, fmt.Errorf("modo %q não tem transporte", mode)
}
