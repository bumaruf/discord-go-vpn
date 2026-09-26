//go:build windows

package discord

import (
	"fmt"
	"os/exec"
)

// RestartAsUser reopens Discord without this process's elevation, which an
// elevated Discord would inherit, losing file drag-and-drop. Explorer opens it,
// since Explorer runs as the session user.
func RestartAsUser(explicit string) (string, error) {
	exe, err := Resolve(explicit)
	if err != nil {
		return "", err
	}

	Stop()

	// explorer.exe returns at once and reports nothing about what it opens.
	if err := exec.Command("explorer.exe", exe).Start(); err != nil {
		return "", fmt.Errorf("pedindo ao Explorer para abrir %s: %w", exe, err)
	}
	return exe, nil
}
