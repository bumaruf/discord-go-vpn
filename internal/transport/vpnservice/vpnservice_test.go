//go:build linux

package vpnservice

import (
	"errors"
	"net/netip"
	"os"
	"reflect"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/bumaruf/discord-go-vpn/internal/transport"
	"github.com/bumaruf/discord-go-vpn/internal/wg"
)

var _ transport.Transport = (*Transport)(nil)

type fakeHost struct {
	establish   func(Interface) (int, error)
	established []Interface
	opened      int
}

func (h *fakeHost) Establish(i Interface) (int, error) {
	h.established = append(h.established, i)
	return h.establish(i)
}

func (h *fakeHost) OpenDiscord() error {
	h.opened++
	return nil
}

func config() *wg.Config {
	return &wg.Config{
		PrivateKey: "yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk=",
		Addresses:  []netip.Addr{netip.MustParseAddr("10.9.0.10"), netip.MustParseAddr("fd00::a")},
		DNS:        []netip.Addr{netip.MustParseAddr("10.9.0.1")},
		MTU:        1380,
		Peer: wg.Peer{
			PublicKey: "xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg=",
			Endpoint:  "127.0.0.1:51820",
		},
	}
}

func TestInterfaceFor(t *testing.T) {
	got := InterfaceFor(config())
	want := Interface{
		Addresses: []string{"10.9.0.10/32", "fd00::a/128"},
		Routes:    []string{"0.0.0.0/0", "::/0"},
		DNS:       []string{"10.9.0.1"},
		MTU:       MaxMTU,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("InterfaceFor = %+v, want %+v", got, want)
	}
}

func TestInterfaceForFillsDefaults(t *testing.T) {
	cfg := config()
	cfg.DNS, cfg.MTU = nil, 0
	cfg.Peer.AllowedIPs = []string{"10.0.0.0/8"}

	got := InterfaceFor(cfg)
	if got.MTU != MaxMTU || !reflect.DeepEqual(got.DNS, []string{wg.DefaultDNS.String()}) {
		t.Errorf("MTU %d, DNS %v: want the defaults", got.MTU, got.DNS)
	}
	if !reflect.DeepEqual(got.Routes, []string{"10.0.0.0/8"}) {
		t.Errorf("Routes = %v, want the .conf's AllowedIPs", got.Routes)
	}
}

func TestInterfaceForKeepsASmallerMTU(t *testing.T) {
	cfg := config()
	cfg.MTU = 1200
	if got := InterfaceFor(cfg).MTU; got != 1200 {
		t.Errorf("MTU = %d, want the .conf's 1200", got)
	}
}

func TestStartLeavesNoInterfaceWhenConfigFails(t *testing.T) {
	cfg := config()
	cfg.Peer.Endpoint = "not an endpoint"
	host := &fakeHost{establish: func(Interface) (int, error) { return -1, errors.New("unexpected") }}

	if err := New(host).Start(cfg); err == nil {
		t.Fatal("Start succeeded with a bad endpoint")
	}
	if len(host.established) != 0 {
		t.Error("Start established the interface before the config was known to work")
	}
}

func TestStartReportsEstablishFailure(t *testing.T) {
	host := &fakeHost{establish: func(Interface) (int, error) { return -1, errors.New("permission revoked") }}
	tr := New(host)

	if err := tr.Start(config()); err == nil {
		t.Fatal("Start succeeded without an interface")
	}
	if err := tr.Ready(); err == nil {
		t.Error("Ready succeeded after a failed Start")
	}
}

func TestOpenAsksHostForDiscord(t *testing.T) {
	host := &fakeHost{}
	if _, err := New(host).Open(""); err != nil || host.opened != 1 {
		t.Errorf("Open: err %v, %d calls to OpenDiscord, want one", err, host.opened)
	}
}

// TestStartOnRealTUN stands in for VpnService with a TUN device opened by hand,
// which needs root.
func TestStartOnRealTUN(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root to create a TUN device")
	}
	host := &fakeHost{establish: func(Interface) (int, error) { return openTUN(t, "dgv-test0") }}
	tr := New(host)

	if err := tr.Start(config()); err != nil {
		t.Skipf("no TUN device here: %v", err)
	}
	if err := tr.Ready(); err != nil {
		t.Errorf("Ready on a running device: %v", err)
	}
	tr.Close()
	if err := tr.Ready(); err == nil {
		t.Error("Ready succeeded after Close")
	}
}

func openTUN(t *testing.T, name string) (int, error) {
	fd, err := unix.Open("/dev/net/tun", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Skipf("no /dev/net/tun: %v", err)
	}
	var req [unix.IFNAMSIZ + 64]byte
	copy(req[:], name)
	*(*uint16)(unsafe.Pointer(&req[unix.IFNAMSIZ])) = unix.IFF_TUN | unix.IFF_NO_PI
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.TUNSETIFF, uintptr(unsafe.Pointer(&req[0]))); errno != 0 {
		unix.Close(fd)
		t.Skipf("TUNSETIFF: %v", errno)
	}
	return fd, nil
}
