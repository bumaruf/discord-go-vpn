//go:build linux

// Package netns runs Discord in its own network namespace, whose only exit is a
// WireGuard interface.
//
// The namespace outlives each connection, so Discord stays open inside it
// across reconnections: only the interface is replaced, and cutting Discord's
// connections makes it reconnect through the new one. While no tunnel can be
// had, a veth pair NATed by the host gives the namespace the normal network.
//
// The TUN device is created in the original namespace and then moved in. Its
// descriptor stays valid, so wireguard-go keeps encrypting from outside, and the
// encrypted packets leave through the normal network.
package netns

import (
	"bytes"
	"fmt"
	"net/netip"
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

	// The veth pair of the normal-network fallback, on a /30 of its own.
	hostVeth     = "dgv-host"
	nsVeth       = "dgv-ns"
	vethSubnet   = "10.254.254.0/30"
	vethHostAddr = "10.254.254.1"
	vethNSAddr   = "10.254.254.2"

	ipForward = "/proc/sys/net/ipv4/ip_forward"

	startupGrace   = 3 * time.Second
	maxStderrLines = 6
)

// fallbackDNS replaces the tunnel's resolver while the fallback is up, since
// that one is often an address inside the tunnel.
var fallbackDNS = netip.MustParseAddr("1.1.1.1")

// fallbackRules are the host's iptables rules for the veth, inserted with -I
// and removed with -D.
var fallbackRules = [][]string{
	{"-t", "nat", "POSTROUTING", "-s", vethSubnet, "-j", "MASQUERADE"},
	{"FORWARD", "-i", hostVeth, "-j", "ACCEPT"},
	{"FORWARD", "-o", hostVeth, "-j", "ACCEPT"},
}

type Namespace struct {
	dev  *device.Device
	name string

	// forwardBefore is the host's ip_forward before the fallback set it, and ""
	// while the fallback is down.
	forwardBefore string
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

// Create makes the namespace, still without a way out.
func Create() (*Namespace, error) {
	if err := Supported(); err != nil {
		return nil, err
	}

	// Leftovers of a run that never cleaned up (crash, kill -9) would fail the
	// setup with "file exists".
	cleanup()

	n := &Namespace{name: nsName}
	err := run(
		[]string{"ip", "netns", "add", n.name},
		// Chromium's processes talk over loopback; without it Discord won't start.
		n.exec("ip", "link", "set", "lo", "up"),
	)
	if err != nil {
		cleanup()
		return nil, err
	}
	return n, nil
}

// Up brings up the tunnel as the namespace's default route, taking over from
// the fallback when it is up.
func (n *Namespace) Up(cfg *wg.Config) error {
	tunDev, err := tun.CreateTUN(devName, cfg.MTU)
	if err != nil {
		return fmt.Errorf("criando a interface %s: %w", devName, err)
	}

	dev := device.NewDevice(tunDev, conn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, ""))

	uapi, err := cfg.UAPI()
	if err != nil {
		dev.Close()
		return err
	}
	if err := dev.IpcSet(uapi); err != nil {
		dev.Close()
		return fmt.Errorf("aplicando a configuração: %w", err)
	}
	if err := dev.Up(); err != nil {
		dev.Close()
		return fmt.Errorf("subindo o túnel: %w", err)
	}
	n.dev = dev

	steps := [][]string{
		{"ip", "link", "set", devName, "netns", n.name},
		n.exec("ip", "link", "set", devName, "up"),
	}
	for _, addr := range cfg.Addresses {
		steps = append(steps, n.exec("ip", "addr", "add", wg.HostPrefix(addr), "dev", devName))
	}
	steps = append(steps, n.exec("ip", "route", "replace", "default", "dev", devName))

	if err := run(steps...); err != nil {
		n.Down()
		return err
	}
	if err := n.writeDNS(cfg.DNS); err != nil {
		n.Down()
		return err
	}
	return nil
}

// Down takes the tunnel away. Whatever runs inside keeps running, without a
// network until Up or UseHostNetwork gives it one.
func (n *Namespace) Down() {
	// Closing the TUN deletes the interface, and its routes with it.
	if n.dev != nil {
		n.dev.Close()
		n.dev = nil
	}
}

// IsUp reports whether the tunnel is up.
func (n *Namespace) IsUp() bool { return n.dev != nil }

// writeDNS writes the resolv.conf that "ip netns exec" mounts from
// /etc/netns/<name>/. Without it the namespace inherits the host's resolver,
// usually reachable only from outside. It is rewritten in place, so processes
// already inside see the change.
func (n *Namespace) writeDNS(servers []netip.Addr) error {
	dir := filepath.Join("/etc/netns", n.name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("criando %s: %w", dir, err)
	}

	var b strings.Builder
	for _, dns := range servers {
		fmt.Fprintf(&b, "nameserver %s\n", dns)
	}
	return os.WriteFile(filepath.Join(dir, "resolv.conf"), []byte(b.String()), 0o644)
}

// UseHostNetwork routes the namespace through the host's normal network, NATed
// over a veth pair, while the tunnel is down.
func (n *Namespace) UseHostNetwork() error {
	if n.forwardBefore == "" {
		if err := n.addFallback(); err != nil {
			n.removeFallback()
			return err
		}
	}
	return n.writeDNS([]netip.Addr{fallbackDNS})
}

func (n *Namespace) addFallback() error {
	before, err := os.ReadFile(ipForward)
	if err != nil {
		return fmt.Errorf("lendo %s: %w", ipForward, err)
	}

	err = run(
		[]string{"ip", "link", "add", hostVeth, "type", "veth", "peer", "name", nsVeth, "netns", n.name},
		[]string{"ip", "addr", "add", vethHostAddr + "/30", "dev", hostVeth},
		[]string{"ip", "link", "set", hostVeth, "up"},
		n.exec("ip", "addr", "add", vethNSAddr+"/30", "dev", nsVeth),
		n.exec("ip", "link", "set", nsVeth, "up"),
		n.exec("ip", "route", "replace", "default", "via", vethHostAddr),
	)
	if err != nil {
		return err
	}

	if err := os.WriteFile(ipForward, []byte("1"), 0o644); err != nil {
		return fmt.Errorf("ligando %s: %w", ipForward, err)
	}
	n.forwardBefore = strings.TrimSpace(string(before))

	for _, rule := range fallbackRules {
		if err := run(iptables("-I", rule)); err != nil {
			return err
		}
	}
	return nil
}

// UseTunnel drops the fallback once Up has put the tunnel back as the route.
func (n *Namespace) UseTunnel() { n.removeFallback() }

func (n *Namespace) removeFallback() {
	removeVeth()
	if n.forwardBefore != "" {
		_ = os.WriteFile(ipForward, []byte(n.forwardBefore), 0o644)
		n.forwardBefore = ""
	}
}

// removeVeth deletes the pair, which takes the namespace's end and its route
// with it, and the host's rules for it.
func removeVeth() {
	_ = exec.Command("ip", "link", "del", hostVeth).Run()
	for _, rule := range fallbackRules {
		_ = run(iptables("-D", rule))
	}
}

// iptables builds the command that applies op (-I or -D) to rule, whose table,
// when given, comes before the chain.
func iptables(op string, rule []string) []string {
	cmd := []string{"iptables"}
	if rule[0] == "-t" {
		cmd, rule = append(cmd, rule[:2]...), rule[2:]
	}
	return append(append(cmd, op, rule[0]), rule[1:]...)
}

// Occupied reports whether any process runs inside, which only Discord does.
func (n *Namespace) Occupied() bool {
	out, err := exec.Command("ip", "netns", "pids", n.name).Output()
	return err == nil && len(bytes.TrimSpace(out)) > 0
}

// Reconnect cuts the TCP connections of whatever runs inside, so Discord opens
// them again over the current route instead of waiting for them to time out.
// The listening sockets, such as Discord's local RPC server, are left alone.
func (n *Namespace) Reconnect() error {
	return run(n.exec("ss", "-K", "-t", "state", "established"))
}

// Command runs exe inside the namespace as the user who invoked sudo. The X and
// PulseAudio sockets are files, so the session stays reachable from inside.
func (n *Namespace) Command(exe string, args ...string) *exec.Cmd {
	argv := append([]string{"netns", "exec", n.name}, discord.AsInvokingUser(append([]string{exe}, args...)...)...)
	return exec.Command("ip", argv...)
}

func (n *Namespace) Name() string { return n.name }

// Open restarts the client minimized inside the namespace and fails if it exits
// within startupGrace.
func (n *Namespace) Open(discordPath string) (string, error) {
	exe, err := discord.Resolve(discordPath)
	if err != nil {
		return "", err
	}

	discord.Stop()

	cmd := n.Command(exe, discord.MinimizedFlag)
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
// Whatever still runs inside is left without a network.
func (n *Namespace) Close() {
	n.Down()
	n.removeFallback()
	cleanup()
}

// exec builds the command that runs argv inside the namespace.
func (n *Namespace) exec(argv ...string) []string {
	return append([]string{"ip", "netns", "exec", n.name}, argv...)
}

// run runs each command in order and stops at the first failure.
func run(cmds ...[]string) error {
	for _, c := range cmds {
		if output, err := exec.Command(c[0], c[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %w: %s", strings.Join(c, " "), err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

// cleanup removes what a run left behind. ip_forward stays as it is: a run
// that crashed never recorded the value to restore.
func cleanup() {
	_ = exec.Command("ip", "netns", "del", nsName).Run()
	_ = os.RemoveAll(filepath.Join("/etc/netns", nsName))
	// Covers an interface left behind in the original namespace.
	_ = exec.Command("ip", "link", "del", devName).Run()
	removeVeth()
}
