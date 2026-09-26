package wiresock

import (
	"strings"

	"github.com/bumaruf/discord-go-vpn/internal/discord"
	"github.com/bumaruf/discord-go-vpn/internal/wg"
)

// renderConfig is the config wiresock-client runs, in Windows line endings.
// AllowedApps goes last, so it lands in the [Peer] section wg.Config.Conf ends with.
func renderConfig(cfg *wg.Config) string {
	return strings.ReplaceAll(cfg.Conf()+allowedAppsLine()+"\n", "\n", "\r\n")
}

// allowedAppsLine is the AllowedApps line that makes the tunnel per process.
// Each name goes in bare, the form WireSock documents, and with .exe, as it
// always did. Update.exe is included because it launches Discord on Windows;
// outside the tunnel, Discord would start on the normal network.
func allowedAppsLine() string {
	var apps []string
	for _, name := range discord.WindowsProcessNames() {
		apps = append(apps, strings.TrimSuffix(name, ".exe"), name)
	}
	apps = append(apps, "Update.exe")
	return "AllowedApps = " + strings.Join(apps, ", ")
}
