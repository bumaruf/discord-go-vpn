package wireguard

import (
	"net/netip"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Interface is what the server needs from the WireGuard interface, so tests can run without wg.
type Interface interface {
	AddPeer(publicKey, ip string) error
	RemovePeer(publicKey string) error
	Dump() (map[string]PeerStat, error)
}

// PeerStat is what the WireGuard interface knows about a peer.
type PeerStat struct {
	PubKey     string
	Endpoint   string
	AllowedIPs []netip.Prefix
	RxBytes    uint64
	TxBytes    uint64
	Handshake  time.Time // zero when there was none
}

// CLI drives a real interface through the wg command.
type CLI struct {
	iface string
}

func NewCLI(iface string) CLI {
	return CLI{iface: iface}
}

// AddPeer uses "wg set", which touches only this peer and leaves connected ones alone.
func (w CLI) AddPeer(publicKey, ip string) error {
	return exec.Command("wg", "set", w.iface, "peer", publicKey, "allowed-ips", ip+"/32").Run()
}

func (w CLI) RemovePeer(publicKey string) error {
	return exec.Command("wg", "set", w.iface, "peer", publicKey, "remove").Run()
}

func (w CLI) PublicKey() (string, error) {
	output, err := exec.Command("wg", "show", w.iface, "public-key").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (w CLI) Dump() (map[string]PeerStat, error) {
	output, err := exec.Command("wg", "show", w.iface, "dump").Output()
	if err != nil {
		return nil, err
	}
	return parseDump(string(output)), nil
}

// parseDump reads "wg show <iface> dump". The first line describes the interface; each
// following line is a peer:
//
//	public-key  preshared-key  endpoint  allowed-ips  latest-handshake  rx  tx  keepalive
func parseDump(output string) map[string]PeerStat {
	peers := make(map[string]PeerStat)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return peers
	}

	for _, line := range lines[1:] {
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		p := PeerStat{PubKey: fields[0], Endpoint: fields[2], AllowedIPs: parseAllowedIPs(fields[3])}
		if seconds, err := strconv.ParseInt(fields[4], 10, 64); err == nil && seconds > 0 {
			p.Handshake = time.Unix(seconds, 0)
		}
		p.RxBytes, _ = strconv.ParseUint(fields[5], 10, 64)
		p.TxBytes, _ = strconv.ParseUint(fields[6], 10, 64)
		peers[p.PubKey] = p
	}
	return peers
}

// parseAllowedIPs reads the comma-separated list, which wg prints as "(none)" when empty.
func parseAllowedIPs(field string) []netip.Prefix {
	var prefixes []netip.Prefix
	for _, s := range strings.Split(field, ",") {
		if p, err := netip.ParsePrefix(s); err == nil {
			prefixes = append(prefixes, p)
		}
	}
	return prefixes
}

// ShortKey shortens a public key for logs and identities.
func ShortKey(s string) string {
	if len(s) > 10 {
		return s[:10] + "…"
	}
	return s
}
