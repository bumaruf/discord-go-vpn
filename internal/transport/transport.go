// Package transport puts Discord inside the tunnel without the rest of the
// program knowing how: a network namespace on Linux, WireSock on Windows.
package transport

import (
	"fmt"

	"github.com/bumaruf/discord-go-vpn/internal/routing"
	"github.com/bumaruf/discord-go-vpn/internal/wg"
)

type Transport interface {
	Start(cfg *wg.Config) error

	// Ready reports whether the tunnel is still up. It is separate from Start
	// because a transport can fail right after starting, as wiresock-client does.
	Ready() error

	// Open restarts Discord inside the tunnel.
	Open(discordPath string) (string, error)

	// Close undoes everything Start created.
	Close()
}

// Meter is implemented by the transports that run WireGuard in this process;
// WireSock runs it in another and reports no counters.
type Meter interface {
	Traffic() (wg.Traffic, error)
}

// Reporter receives what Start has to tell the person while it works.
type Reporter interface {
	// Progress is a step of a slow setup, such as installing WireSock.
	Progress(msg string)
	// Warn is a problem the tunnel keeps running despite.
	Warn(msg string)
}

func New(mode routing.Mode, report Reporter) (Transport, error) {
	switch mode {
	case routing.ModeNetns:
		return newNetns()
	case routing.ModeWiresock:
		return newWiresock(report)
	}
	return nil, fmt.Errorf("modo %q não tem transporte", mode)
}
