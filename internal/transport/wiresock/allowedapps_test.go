package wiresock

import (
	"net/netip"
	"slices"
	"strings"
	"testing"

	"github.com/bumaruf/discord-go-vpn/internal/wg"
)

func TestRenderConfigAddsAllowedAppsToThePeerInCRLF(t *testing.T) {
	cfg := &wg.Config{
		PrivateKey: "priv",
		Addresses:  []netip.Addr{netip.MustParseAddr("10.2.0.2")},
		Peer:       wg.Peer{PublicKey: "pub", Endpoint: "203.0.113.10:51820"},
	}

	got := renderConfig(cfg)

	want := strings.ReplaceAll(cfg.Conf(), "\n", "\r\n") + allowedAppsLine() + "\r\n"
	if got != want {
		t.Errorf("renderConfig =\n%q\nwant\n%q", got, want)
	}
	if peer, apps := strings.Index(got, "[Peer]"), strings.Index(got, "AllowedApps"); peer < 0 || apps < peer {
		t.Errorf("AllowedApps is not in the [Peer] section:\n%s", got)
	}
}

func allowedApps(t *testing.T) []string {
	t.Helper()
	key, value, ok := strings.Cut(allowedAppsLine(), "=")
	if !ok || strings.TrimSpace(key) != "AllowedApps" {
		t.Fatalf("allowedAppsLine() = %q, want an AllowedApps = ... line", allowedAppsLine())
	}
	var apps []string
	for _, app := range strings.Split(value, ",") {
		apps = append(apps, strings.TrimSpace(app))
	}
	return apps
}

func TestAllowedAppsKeepsCurrentEntries(t *testing.T) {
	apps := allowedApps(t)
	for _, want := range []string{"Discord", "Discord.exe", "Update.exe"} {
		if !slices.Contains(apps, want) {
			t.Errorf("AllowedApps %v is missing %q", apps, want)
		}
	}
}

func TestAllowedAppsIncludesDiscordVariants(t *testing.T) {
	apps := allowedApps(t)
	for _, want := range []string{
		"DiscordCanary", "DiscordCanary.exe",
		"DiscordPTB", "DiscordPTB.exe",
		"Vesktop", "Vesktop.exe",
	} {
		if !slices.Contains(apps, want) {
			t.Errorf("AllowedApps %v is missing %q", apps, want)
		}
	}
}
