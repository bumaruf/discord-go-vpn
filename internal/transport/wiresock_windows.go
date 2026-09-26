//go:build windows

package transport

import (
	"errors"
	"fmt"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/discord"
	"github.com/bumaruf/discord-go-vpn/internal/errtext"
	"github.com/bumaruf/discord-go-vpn/internal/transport/wiresock"
	"github.com/bumaruf/discord-go-vpn/internal/wg"
)

type wsTransport struct {
	tun         *wiresock.Tunnel
	discordPath string
	report      Reporter

	// tunneledDiscord is discord.StartedAt of the Discord this transport opened
	// inside the tunnel, and zero until it opens one.
	tunneledDiscord time.Time
}

func newWiresock(discordPath string, report Reporter) (Transport, error) {
	return &wsTransport{discordPath: discordPath, report: report}, nil
}

func (w *wsTransport) Start(cfg *wg.Config) error {
	tun, err := wiresock.Start(cfg, w.report.Progress, w.report.Warn)
	if err != nil {
		return err
	}
	w.tun = tun
	return nil
}

func (w *wsTransport) Ready() error {
	if w.tun == nil {
		return errors.New("o túnel não chegou a subir")
	}
	return w.tun.Alive()
}

func (w *wsTransport) Stop() {
	if w.tun != nil {
		w.tun.Close()
		w.tun = nil
	}
}

// Enter only reconnects the Discord this transport opened inside the tunnel:
// it already carries the location of the tunnel's IP. Any other Discord loaded
// with the person's own IP, whose location Discord only reads again on a fresh
// load, and in Brazil that location blocks screen sharing and the camera; so it
// is reopened, minimized, to load inside.
func (w *wsTransport) Enter() error {
	if started, running := discord.StartedAt(); running && started.Equal(w.tunneledDiscord) {
		w.dropConnections()
		return nil
	}

	if _, err := discord.RestartAsUser(w.discordPath); err != nil {
		return err
	}
	w.tunneledDiscord, _ = discord.StartedAt()
	return nil
}

// Leave makes Discord reconnect at once on the normal network, where its
// connections went when the filter came down.
func (w *wsTransport) Leave() error {
	if _, running := discord.StartedAt(); running {
		w.dropConnections()
	}
	return nil
}

func (w *wsTransport) dropConnections() {
	skipped, err := discord.DropConnections()
	switch {
	case err != nil:
		w.report.Warn(fmt.Sprintf("não consegui derrubar as conexões do Discord (%s); ele vai reconectar sozinho, mais devagar", errtext.Translate(err)))
	case skipped > 0:
		w.report.Warn(fmt.Sprintf("%d conexões IPv6 do Discord ficaram de fora; ele vai refazê-las sozinho, mais devagar", skipped))
	}
}

func (w *wsTransport) Close() {
	w.Stop()
	_ = w.Leave()
}
