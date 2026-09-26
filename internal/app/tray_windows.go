//go:build windows

package app

import (
	"fmt"
	"runtime"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/platform/tray"
	"github.com/bumaruf/discord-go-vpn/internal/supervisor"
	"github.com/bumaruf/discord-go-vpn/internal/ui"
)

const tooltipRefresh = 2 * time.Second

// startTray shows the tunnel state in the tray icon's tooltip. It returns the
// cleanup, or why there is no tray.
func startTray(sup *supervisor.Supervisor) (func(), error) {
	type started struct {
		tray *tray.Tray
		err  error
	}
	ready := make(chan started, 1)

	go func() {
		// Windows ties a window to the thread that created it, so the icon, its
		// hidden window and the message loop share one OS thread.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		t, err := tray.New("discord-go-vpn")
		ready <- started{t, err}
		if err == nil {
			t.Loop()
		}
	}()

	s := <-ready
	if s.err != nil {
		return nil, s.err
	}
	t := s.tray

	stop := make(chan struct{})

	// Polls Status instead of reading Updates: a channel hands each value to a
	// single receiver, so the tray would steal changes from the screen.
	go func() {
		tick := time.NewTicker(tooltipRefresh)
		defer tick.Stop()

		previous := ""
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				st := sup.Status()
				tooltip := fmt.Sprintf("discord-go-vpn — %s", ui.StateLabel(st.State))
				if tooltip != previous {
					t.SetTooltip(tooltip)
					previous = tooltip
				}
			}
		}
	}()

	go t.WatchMinimize(stop)

	return func() {
		close(stop)
		t.Close()
	}, nil
}
