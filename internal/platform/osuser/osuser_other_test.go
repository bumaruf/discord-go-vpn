//go:build !windows && !android

package osuser

import (
	"os/user"
	"testing"
)

func TestNamePrefersSudoUser(t *testing.T) {
	t.Setenv("SUDO_USER", "ana")
	if got := Name(); got != "ana" {
		t.Errorf("Name() = %q, want the SUDO_USER ana", got)
	}
}

func TestNameFallsBackToCurrentUser(t *testing.T) {
	t.Setenv("SUDO_USER", "")
	u, err := user.Current()
	if err != nil {
		t.Skipf("no current user to compare with: %v", err)
	}
	if got := Name(); got != u.Username {
		t.Errorf("Name() = %q, want the current user %q", got, u.Username)
	}
}
