//go:build windows

package transport

import (
	"errors"

	"github.com/bumaruf/discord-go-vpn/internal/discord"
	"github.com/bumaruf/discord-go-vpn/internal/transport/wiresock"
	"github.com/bumaruf/discord-go-vpn/internal/wg"
)

type wsTransport struct {
	tun    *wiresock.Tunnel
	report Reporter
}

func newWiresock(report Reporter) (Transport, error) { return &wsTransport{report: report}, nil }

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

func (w *wsTransport) Open(discordPath string) (string, error) {
	return discord.RestartAsUser(discordPath)
}

func (w *wsTransport) Close() {
	if w.tun != nil {
		w.tun.Close()
		w.tun = nil
	}
}
