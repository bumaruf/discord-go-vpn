//go:build !windows

package discord

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
)

// RestartAsUser reopens Discord on the normal network as the user who invoked
// sudo, without waiting for it, and returns the path used.
func RestartAsUser(explicit string) (string, error) {
	Stop()

	exe, err := Resolve(explicit)
	if err != nil {
		return "", err
	}

	argv := AsInvokingUser(exe)
	cmd := exec.Command(argv[0], argv[1:]...)
	Detach(cmd)

	// Discord's output would garble the screen.
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("abrindo %s: %w", exe, err)
	}

	// Reaps Discord when it exits before this program.
	go func() { _ = cmd.Wait() }()

	return exe, nil
}

// Detach starts Discord in a session of its own. In this program's session,
// closing the terminal would send it SIGHUP, which, like SIGTERM, makes it die
// of SIGTRAP and reach the desktop as a crash.
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// AsInvokingUser prefixes argv to run it as SUDO_USER, when set. As root,
// Chromium refuses to start without --no-sandbox, and Discord would create its
// settings files owned by root.
func AsInvokingUser(argv ...string) []string {
	username := os.Getenv("SUDO_USER")
	if username == "" {
		return argv
	}

	// env instead of sudo -E, which needs SETENV in sudoers; without it sudo
	// refuses the whole command.
	prefix := append([]string{"sudo", "-u", username, "env"}, graphicalEnv(username)...)
	return append(prefix, argv...)
}

// graphicalEnv is what a GUI app needs to find the user's session.
func graphicalEnv(username string) []string {
	vars := []string{
		"HOME=" + homeOf(username),
		"USER=" + username,
		"LOGNAME=" + username,
	}

	for _, name := range []string{"DISPLAY", "XAUTHORITY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "XDG_SESSION_TYPE", "DBUS_SESSION_BUS_ADDRESS", "LANG"} {
		if value := os.Getenv(name); value != "" {
			vars = append(vars, name+"="+value)
		}
	}

	// sudo usually clears these two, and without them no window opens.
	if os.Getenv("DISPLAY") == "" {
		vars = append(vars, "DISPLAY=:0")
	}
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		if uid := os.Getenv("SUDO_UID"); uid != "" {
			runtimeDir = "/run/user/" + uid
			vars = append(vars, "XDG_RUNTIME_DIR="+runtimeDir)
		}
	}

	return append(vars, sessionFromRuntimeDir(runtimeDir)...)
}

// sessionFromRuntimeDir recovers from the user's runtime dir what sudo cleared
// of a Wayland session. Without it Discord starts as plain X11 and screen
// sharing on Hyprland shows a black screen, since only the PipeWire capturer
// behind the desktop portal can see Wayland windows.
func sessionFromRuntimeDir(runtimeDir string) []string {
	if runtimeDir == "" {
		return nil
	}

	var vars []string

	display := os.Getenv("WAYLAND_DISPLAY")
	if display == "" {
		display = waylandSocket(runtimeDir)
		if display != "" {
			vars = append(vars, "WAYLAND_DISPLAY="+display)
		}
	}
	if os.Getenv("XDG_SESSION_TYPE") == "" && display != "" {
		vars = append(vars, "XDG_SESSION_TYPE=wayland")
	}

	// The portal is reached over the session bus.
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		bus := filepath.Join(runtimeDir, "bus")
		if _, err := os.Stat(bus); err == nil {
			vars = append(vars, "DBUS_SESSION_BUS_ADDRESS=unix:path="+bus)
		}
	}

	return vars
}

// waylandSocket is the name of the first compositor socket in runtimeDir, or
// "" when there is none.
func waylandSocket(runtimeDir string) string {
	matches, _ := filepath.Glob(filepath.Join(runtimeDir, "wayland-*"))
	for _, path := range matches {
		if strings.HasSuffix(path, ".lock") {
			continue
		}
		return filepath.Base(path)
	}
	return ""
}

func homeOf(username string) string {
	if u, err := user.Lookup(username); err == nil {
		return u.HomeDir
	}
	return "/home/" + username
}
