// Package wg reads a standard WireGuard .conf file, translates it into the UAPI
// format wireguard-go consumes, and writes it back as a .conf.
package wg

import (
	"bufio"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

// DefaultMTU is WireGuard's: 1420 fits a 1500-byte Ethernet frame after the
// tunnel overhead. An MTU in the .conf overrides it.
const DefaultMTU = 1420

// DefaultDNS applies when a config has no usable DNS: without a resolver
// nothing resolves inside the tunnel.
var DefaultDNS = netip.MustParseAddr("1.1.1.1")

// DefaultAllowedIPs sends all traffic through the tunnel. Each call returns a
// new slice, so one config never shares its backing array with another.
func DefaultAllowedIPs() []string { return []string{"0.0.0.0/0", "::/0"} }

// HostPrefix writes addr as a single-host prefix, such as 10.2.0.2/32.
func HostPrefix(addr netip.Addr) string {
	if addr.Is6() {
		return addr.String() + "/128"
	}
	return addr.String() + "/32"
}

type Peer struct {
	PublicKey    string
	PresharedKey string
	Endpoint     string
	AllowedIPs   []string
	Keepalive    int
}

type Config struct {
	PrivateKey string
	Addresses  []netip.Addr
	DNS        []netip.Addr
	MTU        int
	Peer       Peer
}

func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("abrindo %s: %w", path, err)
	}
	defer f.Close()

	cfg, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("lendo %s: %w", path, err)
	}
	return cfg, nil
}

func Parse(r io.Reader) (*Config, error) {
	cfg := &Config{MTU: DefaultMTU}

	var section string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if i := strings.IndexAny(line, "#;"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.Trim(line, "[]"))
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}

		if err := cfg.set(section, key, value); err != nil {
			return nil, err
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if len(cfg.DNS) == 0 {
		cfg.DNS = []netip.Addr{DefaultDNS}
	}
	return cfg, nil
}

func (c *Config) set(section, key, value string) error {
	switch section {
	case "interface":
		switch key {
		case "privatekey":
			c.PrivateKey = value
		case "address":
			addrs, err := parseAddrList(value)
			if err != nil {
				return fmt.Errorf("campo Address inválido: %w", err)
			}
			c.Addresses = append(c.Addresses, addrs...)
		case "dns":
			// A resolver given by name is valid in the format; skip it rather
			// than reject the file over an optional line.
			for _, field := range splitList(value) {
				if addr, err := netip.ParseAddr(field); err == nil {
					c.DNS = append(c.DNS, addr)
				}
			}
		case "mtu":
			mtu, err := strconv.Atoi(value)
			if err != nil || mtu <= 0 {
				return fmt.Errorf("campo MTU inválido: %q", value)
			}
			c.MTU = mtu
		}
	case "peer":
		switch key {
		case "publickey":
			c.Peer.PublicKey = value
		case "presharedkey":
			c.Peer.PresharedKey = value
		case "endpoint":
			c.Peer.Endpoint = value
		case "allowedips":
			c.Peer.AllowedIPs = append(c.Peer.AllowedIPs, splitList(value)...)
		case "persistentkeepalive":
			ka, err := strconv.Atoi(value)
			if err != nil || ka < 0 {
				return fmt.Errorf("campo PersistentKeepalive inválido: %q", value)
			}
			c.Peer.Keepalive = ka
		}
	}
	return nil
}

func (c *Config) validate() error {
	switch {
	case c.PrivateKey == "":
		return errors.New("falta PrivateKey na seção [Interface]")
	case len(c.Addresses) == 0:
		return errors.New("falta Address na seção [Interface]")
	case c.Peer.PublicKey == "":
		return errors.New("falta PublicKey na seção [Peer]")
	case c.Peer.Endpoint == "":
		return errors.New("falta Endpoint na seção [Peer]")
	}

	if _, err := decodeKey(c.PrivateKey); err != nil {
		return fmt.Errorf("campo PrivateKey inválido: %w", err)
	}
	if _, err := decodeKey(c.Peer.PublicKey); err != nil {
		return fmt.Errorf("campo PublicKey inválido: %w", err)
	}
	if c.Peer.PresharedKey != "" {
		if _, err := decodeKey(c.Peer.PresharedKey); err != nil {
			return fmt.Errorf("campo PresharedKey inválido: %w", err)
		}
	}
	return nil
}

// UAPI builds the wireguard-go device config, where keys are hex. It resolves
// the Endpoint outside the tunnel: resolving the tunnel's own server through
// the tunnel would need the tunnel up first.
func (c *Config) UAPI() (string, error) {
	priv, err := decodeKey(c.PrivateKey)
	if err != nil {
		return "", err
	}
	pub, err := decodeKey(c.Peer.PublicKey)
	if err != nil {
		return "", err
	}

	endpoint, err := net.ResolveUDPAddr("udp", c.Peer.Endpoint)
	if err != nil {
		return "", fmt.Errorf("resolvendo Endpoint %q: %w", c.Peer.Endpoint, err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "private_key=%s\n", hex.EncodeToString(priv))
	fmt.Fprintf(&b, "public_key=%s\n", hex.EncodeToString(pub))
	if c.Peer.PresharedKey != "" {
		psk, err := decodeKey(c.Peer.PresharedKey)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "preshared_key=%s\n", hex.EncodeToString(psk))
	}
	fmt.Fprintf(&b, "endpoint=%s\n", endpoint.String())

	for _, cidr := range c.Peer.EffectiveAllowedIPs() {
		fmt.Fprintf(&b, "allowed_ip=%s\n", cidr)
	}
	if c.Peer.Keepalive > 0 {
		fmt.Fprintf(&b, "persistent_keepalive_interval=%d\n", c.Peer.Keepalive)
	}
	return b.String(), nil
}

// Conf writes the config back in .conf form. It leaves MTU out, which the
// WireSock config built from it has never carried.
func (c *Config) Conf() string {
	var b strings.Builder
	b.WriteString("[Interface]\n")
	fmt.Fprintf(&b, "PrivateKey = %s\n", c.PrivateKey)
	for _, addr := range c.Addresses {
		fmt.Fprintf(&b, "Address = %s\n", HostPrefix(addr))
	}
	for _, dns := range c.DNS {
		fmt.Fprintf(&b, "DNS = %s\n", dns)
	}
	fmt.Fprintf(&b, "\n[Peer]\nPublicKey = %s\n", c.Peer.PublicKey)
	if c.Peer.PresharedKey != "" {
		fmt.Fprintf(&b, "PresharedKey = %s\n", c.Peer.PresharedKey)
	}
	fmt.Fprintf(&b, "AllowedIPs = %s\n", strings.Join(c.Peer.EffectiveAllowedIPs(), ", "))
	fmt.Fprintf(&b, "Endpoint = %s\n", c.Peer.Endpoint)
	if c.Peer.Keepalive > 0 {
		fmt.Fprintf(&b, "PersistentKeepalive = %d\n", c.Peer.Keepalive)
	}
	return b.String()
}

// EffectiveAllowedIPs is AllowedIPs, or every address when the .conf leaves it out.
func (p Peer) EffectiveAllowedIPs() []string {
	if len(p.AllowedIPs) == 0 {
		return DefaultAllowedIPs()
	}
	return p.AllowedIPs
}

// parseAddrList keeps only the address of each "10.2.0.2/32", and also accepts
// a bare IP, as some providers write it.
func parseAddrList(value string) ([]netip.Addr, error) {
	var addrs []netip.Addr
	for _, field := range splitList(value) {
		if prefix, err := netip.ParsePrefix(field); err == nil {
			addrs = append(addrs, prefix.Addr())
			continue
		}
		addr, err := netip.ParseAddr(field)
		if err != nil {
			return nil, fmt.Errorf("%q não é um endereço", field)
		}
		addrs = append(addrs, addr)
	}
	return addrs, nil
}

func splitList(value string) []string {
	var out []string
	for _, field := range strings.Split(value, ",") {
		if field = strings.TrimSpace(field); field != "" {
			out = append(out, field)
		}
	}
	return out
}

// decodeKey checks the 32-byte length up front, which turns a handshake that
// never completes into an error that says why.
func decodeKey(value string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("não é base64 válido")
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("tem %d bytes, esperava 32", len(raw))
	}
	return raw, nil
}
