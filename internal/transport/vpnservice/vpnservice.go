//go:build linux

// Package vpnservice runs the tunnel on Android. The app's VpnService creates the
// interface, routing only Discord's traffic into it, and hands over its descriptor;
// wireguard-go encrypts from there. This app's own sockets stay outside the VPN, so
// the encrypted packets and the lease renewals leave through the normal network.
package vpnservice

import (
	"errors"
	"fmt"
	"net/netip"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"

	"github.com/bumaruf/discord-go-vpn/internal/wg"
)

// Interface is what the VpnService builder needs to create the interface.
type Interface struct {
	Addresses []string // prefixes, such as 10.9.0.10/32
	Routes    []string
	DNS       []string
	MTU       int
}

// Host is the app's VpnService.
type Host interface {
	// Establish creates the interface and returns its descriptor, which the
	// transport then owns: closing it takes the interface down.
	Establish(Interface) (fd int, err error)
	// OpenDiscord brings Discord to the front, which Android allows only while
	// the app is on screen; it never closes a running one.
	OpenDiscord() error
}

func InterfaceFor(cfg *wg.Config) Interface {
	iface := Interface{Routes: cfg.Peer.EffectiveAllowedIPs(), MTU: cfg.MTU}
	if iface.MTU == 0 {
		iface.MTU = wg.DefaultMTU
	}
	for _, addr := range cfg.Addresses {
		iface.Addresses = append(iface.Addresses, wg.HostPrefix(addr))
	}
	dns := cfg.DNS
	if len(dns) == 0 {
		dns = []netip.Addr{wg.DefaultDNS}
	}
	for _, addr := range dns {
		iface.DNS = append(iface.DNS, addr.String())
	}
	return iface
}

type Transport struct {
	host Host
	dev  *device.Device
}

func New(host Host) *Transport { return &Transport{host: host} }

func (t *Transport) Start(cfg *wg.Config) error {
	// Built first: it resolves the endpoint, and a failure here leaves no
	// interface up with nothing behind it.
	uapi, err := cfg.UAPI()
	if err != nil {
		return err
	}

	fd, err := t.host.Establish(InterfaceFor(cfg))
	if err != nil {
		return fmt.Errorf("o Android não criou a VPN: %w", err)
	}
	// On failure the descriptor is left to the os.File wrapping it, which
	// closes it when collected; closing it here too could close a reused number.
	tunDev, _, err := tun.CreateUnmonitoredTUNFromFD(fd)
	if err != nil {
		return fmt.Errorf("abrindo a interface da VPN: %w", err)
	}

	dev := device.NewDevice(tunDev, conn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, ""))
	if err := dev.IpcSet(uapi); err != nil {
		dev.Close()
		return fmt.Errorf("aplicando a configuração: %w", err)
	}
	if err := dev.Up(); err != nil {
		dev.Close()
		return fmt.Errorf("subindo o túnel: %w", err)
	}
	t.dev = dev
	return nil
}

// Ready fails once the device has closed itself, which happens when Android
// takes the interface away, as when another VPN is turned on.
func (t *Transport) Ready() error {
	if t.dev == nil {
		return errors.New("o túnel não chegou a subir")
	}
	select {
	case <-t.dev.Wait():
		return errors.New("o Android desligou a VPN")
	default:
		return nil
	}
}

// Open can't restart Discord as the desktop does, since no app may close another:
// connections Discord already held may stay outside until it reconnects.
func (t *Transport) Open(string) (string, error) {
	return "", t.host.OpenDiscord()
}

func (t *Transport) Close() {
	if t.dev != nil {
		t.dev.Close()
		t.dev = nil
	}
}
