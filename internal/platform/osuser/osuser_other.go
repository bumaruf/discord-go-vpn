//go:build !windows

package osuser

import (
	"os"
	"os/user"
)

// Name returns the user who invoked sudo, since the client runs as root on Linux,
// or the current user; "" when neither is known.
func Name() string {
	if name := os.Getenv("SUDO_USER"); name != "" {
		return name
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}
