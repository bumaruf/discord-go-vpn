//go:build linux && !android

// Package trayapp is the Linux tray icon: it runs in the desktop session, as
// the person, and drives the service over the control socket. Opening it
// connects the tunnel, the way logging in does through autostart.
package trayapp

import (
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"fyne.io/systray"
	"golang.org/x/sys/unix"

	"github.com/bumaruf/discord-go-vpn/internal/control"
	"github.com/bumaruf/discord-go-vpn/internal/statustext"
	"github.com/bumaruf/discord-go-vpn/internal/supervisor"
	"github.com/bumaruf/discord-go-vpn/internal/ui"
	"github.com/bumaruf/discord-go-vpn/internal/update"
)

const (
	redialEvery = 3 * time.Second
	iconSize    = 32
)

// The mascot's colour per state, as on the screen.
var (
	idleColor     = color.NRGBA{0x80, 0x84, 0x8E, 0xFF}
	startingColor = color.NRGBA{0x8E, 0x95, 0xD8, 0xFF}
	healthyColor  = color.NRGBA{0x58, 0x65, 0xF2, 0xFF}
	degradedColor = color.NRGBA{0xFE, 0xE7, 0x5C, 0xFF}
	fallbackColor = color.NRGBA{0xED, 0x42, 0x45, 0xFF}
)

type tray struct {
	status, open, toggle, retry, quit *systray.MenuItem

	mu    sync.Mutex
	conn  *control.Conn
	phase string
	// quitting stops the redial loop once the person chose Sair.
	quitting bool
}

// Run shows the icon until the person quits from its menu. Only one runs per
// session.
func Run() error {
	unlock, err := singleInstance()
	if err != nil {
		return err
	}
	defer unlock()

	t := &tray{}
	systray.Run(t.ready, func() {})
	return nil
}

// singleInstance holds a lock in the runtime dir, so autostart and a click on
// the menu entry don't put two icons up.
func singleInstance() (func(), error) {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	f, err := os.OpenFile(filepath.Join(dir, "discord-go-vpn-tray.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("a bandeja já está aberta")
	}
	return func() { _ = f.Close() }, nil
}

func (t *tray) ready() {
	systray.SetIcon(ui.MascotPNG(idleColor, iconSize))
	systray.SetTitle("discord-go-vpn")
	systray.SetTooltip("discord-go-vpn")

	t.status = systray.AddMenuItem("procurando o serviço…", "")
	t.status.Disable()
	systray.AddSeparator()
	t.open = systray.AddMenuItem("Abrir painel", "o painel no terminal")
	t.toggle = systray.AddMenuItem("Conectar", "")
	t.retry = systray.AddMenuItem("Tentar agora", "reconecta sem esperar")
	t.retry.Hide()
	systray.AddSeparator()
	t.quit = systray.AddMenuItem("Sair", "desconecta e fecha a bandeja")

	go t.clicks()
	go t.follow()
}

func (t *tray) clicks() {
	for {
		select {
		case <-t.open.ClickedCh:
			openPanel()
		case <-t.toggle.ClickedCh:
			t.mu.Lock()
			op := control.OpConnect
			if t.phase != control.PhaseIdle {
				op = control.OpDisconnect
			}
			t.mu.Unlock()
			t.send(control.Request{Op: op, Env: control.SessionEnv()})
		case <-t.retry.ClickedCh:
			t.send(control.Request{Op: control.OpReconnect})
		case <-t.quit.ClickedCh:
			t.mu.Lock()
			t.quitting = true
			t.mu.Unlock()
			t.send(control.Request{Op: control.OpDisconnect})
			systray.Quit()
			return
		}
	}
}

func (t *tray) send(req control.Request) {
	t.mu.Lock()
	conn := t.conn
	t.mu.Unlock()
	if conn != nil {
		_ = conn.Send(req)
	}
}

// follow keeps a connection to the service, redialling while it is down or
// restarting into a new release.
func (t *tray) follow() {
	connected := false
	for {
		t.mu.Lock()
		quitting := t.quitting
		t.mu.Unlock()
		if quitting {
			return
		}

		conn, err := control.Dial()
		if err != nil {
			t.show("serviço fora do ar", idleColor)
			time.Sleep(redialEvery)
			continue
		}
		t.mu.Lock()
		t.conn = conn
		t.mu.Unlock()

		// Opening the tray connects; a redial after a restart picks up
		// whatever the person chose since.
		t.listen(conn, !connected)
		connected = true

		t.mu.Lock()
		t.conn = nil
		t.mu.Unlock()
		_ = conn.Close()
	}
}

func (t *tray) listen(conn *control.Conn, autoConnect bool) {
	for {
		var ev control.Event
		if err := conn.Receive(&ev); err != nil {
			return
		}
		switch {
		case ev.Hello != nil:
			if ev.Hello.Version != update.Version {
				relaunch(ev.Hello.Version)
			}
			t.setPhase(ev.Hello.Phase)
			if autoConnect && ev.Hello.Phase == control.PhaseIdle {
				_ = conn.Send(control.Request{Op: control.OpConnect, Env: control.SessionEnv()})
			}
			autoConnect = false
		case ev.Ask:
			// The token field lives on the screen.
			openPanel()
		case ev.Session != nil && ev.Session.Error != "":
			t.show(ev.Session.Error, fallbackColor)
		case ev.Status != nil:
			t.setStatus(ev.Status.Supervisor())
		}
	}
}

func (t *tray) setPhase(phase string) {
	t.mu.Lock()
	t.phase = phase
	t.mu.Unlock()
	switch phase {
	case control.PhaseIdle:
		t.toggle.SetTitle("Conectar")
		t.retry.Hide()
		t.show("desconectado", idleColor)
	case control.PhaseStarting:
		t.toggle.SetTitle("Desconectar")
		t.show("iniciando…", startingColor)
	default:
		t.toggle.SetTitle("Desconectar")
	}
}

func (t *tray) setStatus(st supervisor.Status) {
	label := statustext.StateLabel(st.State)
	c := startingColor
	switch st.State {
	case supervisor.StateHealthy:
		c = healthyColor
		if st.DiscordTunneled {
			label += " · Discord no túnel"
		}
	case supervisor.StateDegraded:
		c = degradedColor
		label += fmt.Sprintf(" · tentativa %d", st.Attempt)
	case supervisor.StateFallback:
		c = fallbackColor
		label += " · Discord na rede normal"
	case supervisor.StateStopped:
		c = idleColor
	}
	if st.State == supervisor.StateDegraded || st.State == supervisor.StateFallback {
		t.retry.Show()
	} else {
		t.retry.Hide()
	}
	t.show(label, c)
}

func (t *tray) show(label string, c color.Color) {
	t.status.SetTitle(label)
	systray.SetTooltip("discord-go-vpn — " + label)
	systray.SetIcon(ui.MascotPNG(c, iconSize))
}

// relaunchedEnv names the version a relaunch went for: when the binary on disk
// still isn't it, as with a dev build, another relaunch would only loop.
const relaunchedEnv = "VPN_TRAY_RELAUNCHED"

// relaunch runs the binary on disk, which the service's update replaced, in
// this process's place.
func relaunch(version string) {
	if os.Getenv(relaunchedEnv) == version {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	_ = syscall.Exec(exe, os.Args, append(os.Environ(), relaunchedEnv+"="+version))
}

// terminals are tried in order when xdg-terminal-exec is missing; each takes the
// command after its last argument.
var terminals = [][]string{
	{"xdg-terminal-exec"},
	{"x-terminal-emulator", "-e"},
	{"kitty"},
	{"alacritty", "-e"},
	{"foot"},
	{"ghostty", "-e"},
	{"wezterm", "start", "--"},
	{"gnome-terminal", "--"},
	{"konsole", "-e"},
	{"xfce4-terminal", "-x"},
	{"xterm", "-e"},
}

// openPanel opens the screen in a terminal; it follows the service's tunnel.
func openPanel() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	candidates := terminals
	if term := os.Getenv("TERMINAL"); term != "" {
		candidates = append([][]string{{term, "-e"}}, terminals...)
	}
	for _, argv := range candidates {
		path, err := exec.LookPath(argv[0])
		if err != nil {
			continue
		}
		cmd := exec.Command(path, append(argv[1:], exe)...)
		if cmd.Start() == nil {
			go func() { _ = cmd.Wait() }()
			return
		}
	}
}
