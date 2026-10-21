//go:build linux

// Package netns runs Discord in its own network namespace, whose only exit is a
// WireGuard interface.
//
// The TUN device is created in the original namespace and then moved in. Its
// descriptor stays valid, so wireguard-go keeps encrypting from outside, and the
// encrypted packets leave through the normal network.
package netns

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"

	"github.com/bumaruf/discord-go-vpn/internal/discord"
	"github.com/bumaruf/discord-go-vpn/internal/wg"
)

const (
	nsName  = "discord-vpn"
	devName = "wg-discord"

	startupGrace   = 3 * time.Second
	maxStderrLines = 6
)

type Namespace struct {
	dev  *device.Device
	name string
}

// Supported returns why this mode cannot run here, or nil.
func Supported() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("este modo precisa de root: criar interface de rede e namespace são operações privilegiadas.\n" +
			"\n  rode assim:  sudo ./discord-go-vpn\n" +
			"\n  (é o que leva a mídia do vídeo para dentro do túnel; sem isso ela escapa\n" +
			"   pelo seu IP real e o Discord não libera a transmissão)")
	}
	if _, err := exec.LookPath("ip"); err != nil {
		return fmt.Errorf("o comando \"ip\" não foi encontrado (pacote iproute2)")
	}
	return nil
}

func Start(cfg *wg.Config) (*Namespace, error) {
	if err := Supported(); err != nil {
		return nil, err
	}

	// Leftovers of a run that never cleaned up (crash, kill -9) would fail the
	// setup with "file exists".
	cleanup()

	tunDev, err := tun.CreateTUN(devName, cfg.MTU)
	if err != nil {
		return nil, fmt.Errorf("criando a interface %s: %w", devName, err)
	}

	dev := device.NewDevice(tunDev, conn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, ""))

	uapi, err := cfg.UAPI()
	if err != nil {
		dev.Close()
		return nil, err
	}
	if err := dev.IpcSet(uapi); err != nil {
		dev.Close()
		return nil, fmt.Errorf("aplicando a configuração: %w", err)
	}
	if err := dev.Up(); err != nil {
		dev.Close()
		return nil, fmt.Errorf("subindo o túnel: %w", err)
	}

	ns := &Namespace{dev: dev, name: nsName}
	if err := ns.setup(cfg); err != nil {
		ns.Close()
		return nil, err
	}
	return ns, nil
}

func (n *Namespace) setup(cfg *wg.Config) error {
	steps := [][]string{
		{"ip", "netns", "add", n.name},
		{"ip", "link", "set", devName, "netns", n.name},
		// Chromium's processes talk over loopback; without it Discord won't start.
		{"ip", "netns", "exec", n.name, "ip", "link", "set", "lo", "up"},
		{"ip", "netns", "exec", n.name, "ip", "link", "set", devName, "up"},
	}

	for _, addr := range cfg.Addresses {
		steps = append(steps, []string{"ip", "netns", "exec", n.name,
			"ip", "addr", "add", wg.HostPrefix(addr), "dev", devName})
	}

	steps = append(steps,
		[]string{"ip", "netns", "exec", n.name, "ip", "route", "add", "default", "dev", devName},
	)

	for _, p := range steps {
		if output, err := exec.Command(p[0], p[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %w: %s", strings.Join(p, " "), err, strings.TrimSpace(string(output)))
		}
	}

	return n.writeDNS(cfg)
}

// writeDNS writes the resolv.conf that "ip netns exec" mounts from
// /etc/netns/<name>/. Without it the namespace inherits the host's resolver,
// usually reachable only from outside.
func (n *Namespace) writeDNS(cfg *wg.Config) error {
	dir := filepath.Join("/etc/netns", n.name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("criando %s: %w", dir, err)
	}

	var b strings.Builder
	for _, dns := range cfg.DNS {
		fmt.Fprintf(&b, "nameserver %s\n", dns)
	}
	return os.WriteFile(filepath.Join(dir, "resolv.conf"), []byte(b.String()), 0o644)
}

// Command runs exe inside the namespace as the user who invoked sudo. The X and
// PulseAudio sockets are files, so the session stays reachable from inside.
func (n *Namespace) Command(exe string, args ...string) *exec.Cmd {
	argv := append([]string{"netns", "exec", n.name}, discord.AsInvokingUser(append([]string{exe}, args...)...)...)
	return exec.Command("ip", argv...)
}

func (n *Namespace) Name() string { return n.name }

func (n *Namespace) Traffic() (wg.Traffic, error) {
	uapi, err := n.dev.IpcGet()
	if err != nil {
		return wg.Traffic{}, err
	}
	return wg.ParseTraffic(uapi), nil
}

// Open restarts the client inside the namespace and fails if it exits within
// startupGrace.
func (n *Namespace) Open(discordPath string) (string, error) {
	exe, err := discord.Resolve(discordPath)
	if err != nil {
		return "", err
	}

	discord.Stop()

	cmd := n.Command(exe)
	discord.Detach(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("abrindo o Discord: %w", err)
	}

	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	select {
	case err := <-exited:
		text := strings.TrimSpace(stderr.String())
		if text != "" {
			return "", fmt.Errorf("o Discord não subiu: %w: %s", err, firstLines(text, maxStderrLines))
		}
		return "", fmt.Errorf("o Discord não subiu: %w", err)
	case <-time.After(startupGrace):
	}

	return exe, nil
}

func firstLines(text string, n int) string {
	lines := strings.Split(text, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

// Close deletes the namespace, which would otherwise outlive the process.
func (n *Namespace) Close() {
	cleanup()
	if n.dev != nil {
		n.dev.Close()
		n.dev = nil
	}
}

func cleanup() {
	_ = exec.Command("ip", "netns", "del", nsName).Run()
	_ = os.RemoveAll(filepath.Join("/etc/netns", nsName))
	// Covers an interface left behind in the original namespace.
	_ = exec.Command("ip", "link", "del", devName).Run()
}
