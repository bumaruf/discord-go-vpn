package config

import (
	"fmt"
	"net/netip"
	"os"
)

const defaultMongoDatabase = "vpn"

// DefaultWGSubnet is also the default of server/wg-up.sh, which gives the interface its .1.
var DefaultWGSubnet = netip.MustParsePrefix("10.9.0.0/24")

// Config holds the environment, loaded once at startup. Empty fields mean the
// default, so a zero Config is what an empty environment loads.
type Config struct {
	PanelSecret string
	LiveSecret  string
	PanelOrigin string

	MongoURI      string
	MongoDatabase string

	WGSubnet string
}

// Load is the only reader of the environment.
func Load() Config {
	return Config{
		PanelSecret:   os.Getenv("VPN_PANEL_SECRET"),
		LiveSecret:    os.Getenv("VPN_LIVE_SECRET"),
		PanelOrigin:   os.Getenv("PANEL_ORIGIN"),
		MongoURI:      os.Getenv("MONGODB_URI"),
		MongoDatabase: os.Getenv("MONGODB_DB"),
		WGSubnet:      os.Getenv("WG_SUBNET"),
	}
}

func (c Config) MongoDatabaseName() string {
	if c.MongoDatabase == "" {
		return defaultMongoDatabase
	}
	return c.MongoDatabase
}

// WGSubnetPrefix is the tunnel subnet the lease addresses come from. It must be an IPv4 /24,
// the only shape server/wg-up.sh writes.
func (c Config) WGSubnetPrefix() (netip.Prefix, error) {
	if c.WGSubnet == "" {
		return DefaultWGSubnet, nil
	}
	prefix, err := netip.ParsePrefix(c.WGSubnet)
	if err != nil || !prefix.Addr().Is4() || prefix.Bits() != 24 || prefix != prefix.Masked() {
		return netip.Prefix{}, fmt.Errorf("WG_SUBNET = %q, want an IPv4 /24 such as %s", c.WGSubnet, DefaultWGSubnet)
	}
	return prefix, nil
}
