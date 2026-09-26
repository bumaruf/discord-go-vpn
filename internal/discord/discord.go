// Package discord finds the installed client, closes it and opens it again.
package discord

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// Resolve returns explicit, the path the person gave, or else the one Find finds.
func Resolve(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	return Find()
}

func Find() (string, error) {
	for _, name := range namesInPath() {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}

	for _, pattern := range knownPaths() {
		// Windows keeps the client in a versioned folder, such as app-1.0.9042.
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		if chosen := newest(matches); chosen != "" {
			return chosen, nil
		}
	}

	return "", fmt.Errorf("não encontrei o Discord instalado.\n" +
		"  se ele existe num lugar fora do comum, aponte o caminho:\n" +
		"    -discord /caminho/do/executável\n" +
		"  no Windows costuma ser  %%LOCALAPPDATA%%\\Discord\\app-1.0.xxxx\\Discord.exe")
}

// newest picks the most recently modified candidate. Old version folders stay
// behind on Windows, and lexical order would rank 1.0.9042 after 1.0.10123.
func newest(candidates []string) string {
	var chosen string
	var newestTime time.Time

	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		if chosen == "" || info.ModTime().After(newestTime) {
			chosen, newestTime = candidate, info.ModTime()
		}
	}
	return chosen
}

func namesInPath() []string {
	if runtime.GOOS == "windows" {
		return nil
	}
	return []string{"discord", "Discord", "discord-canary", "discord-ptb", "vesktop"}
}

func knownPaths() []string {
	home, _ := os.UserHomeDir()

	switch runtime.GOOS {
	case "windows":
		local := os.Getenv("LOCALAPPDATA")
		if local == "" && home != "" {
			local = filepath.Join(home, "AppData", "Local")
		}
		return []string{
			filepath.Join(local, "Discord", "app-*", "Discord.exe"),
			filepath.Join(local, "DiscordCanary", "app-*", "DiscordCanary.exe"),
			filepath.Join(local, "DiscordPTB", "app-*", "DiscordPTB.exe"),
		}
	case "darwin":
		paths := []string{"/Applications/Discord.app/Contents/MacOS/Discord"}
		if home != "" {
			paths = append(paths, filepath.Join(home, "Applications", "Discord.app", "Contents", "MacOS", "Discord"))
		}
		return paths
	default:
		paths := []string{
			"/usr/bin/discord",
			"/usr/share/discord/Discord",
			"/opt/discord/Discord",
			"/opt/Discord/Discord",
			"/var/lib/flatpak/exports/bin/com.discordapp.Discord",
		}
		if home != "" {
			paths = append(paths,
				filepath.Join(home, ".local", "share", "flatpak", "exports", "bin", "com.discordapp.Discord"),
				filepath.Join(home, "Applications", "Discord*.AppImage"),
			)
		}
		return paths
	}
}
