//go:build !windows

package discord

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
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
	if os.Getenv("XDG_RUNTIME_DIR") == "" {
		if uid := os.Getenv("SUDO_UID"); uid != "" {
			vars = append(vars, "XDG_RUNTIME_DIR=/run/user/"+uid)
		}
	}

	return vars
}

func homeOf(username string) string {
	if u, err := user.Lookup(username); err == nil {
		return u.HomeDir
	}
	return "/home/" + username
}
