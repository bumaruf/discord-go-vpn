package discord

import (
	"os/exec"
	"runtime"
	"slices"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/platform/tasklist"
)

// Process names are matched exactly (pkill -x, taskkill /IM): a loose pattern
// would also match this program, discord-go-vpn.
var unixNames = []string{"Discord", "DiscordCanary", "DiscordPTB", "vesktop"}
var windowsNames = []string{"Discord.exe", "DiscordCanary.exe", "DiscordPTB.exe", "Vesktop.exe"}

// WindowsProcessNames returns the Windows image names that count as Discord.
func WindowsProcessNames() []string {
	return slices.Clone(windowsNames)
}

const (
	exitTimeout    = 8 * time.Second
	sigtermTimeout = exitTimeout / 2
	pollInterval   = 200 * time.Millisecond

	// lockReleaseDelay covers the single-instance lock outliving the process by
	// a moment.
	lockReleaseDelay = 400 * time.Millisecond
)

// Kill ends every Discord process, children included.
func Kill() {
	if runtime.GOOS == "windows" {
		for _, name := range windowsNames {
			_ = exec.Command("taskkill", "/IM", name, "/T", "/F").Run()
		}
		return
	}

	// SIGTERM first, so the client can save its state.
	for _, name := range unixNames {
		_ = exec.Command("pkill", "-x", name).Run()
	}

	if waitForExit(sigtermTimeout) {
		return
	}

	for _, name := range unixNames {
		_ = exec.Command("pkill", "-9", "-x", name).Run()
	}
}

func IsRunning() bool {
	if runtime.GOOS == "windows" {
		return tasklist.Running(windowsNames...)
	}

	for _, name := range unixNames {
		if err := exec.Command("pgrep", "-x", name).Run(); err == nil {
			return true
		}
	}
	return false
}

// waitForExit reports whether Discord exited within timeout. A new instance
// started while the old one holds the single-instance lock only focuses the old
// one, which stays outside the tunnel.
func waitForExit(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !IsRunning() {
			time.Sleep(lockReleaseDelay)
			return true
		}
		time.Sleep(pollInterval)
	}
	return false
}

// Stop kills Discord and waits for it to exit, so the next instance does not
// just focus the old one.
func Stop() {
	Kill()
	waitForExit(exitTimeout)
}
