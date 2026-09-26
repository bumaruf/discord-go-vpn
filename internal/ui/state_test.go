package ui

import (
	"testing"

	"github.com/bumaruf/discord-go-vpn/internal/supervisor"
)

func TestStateLabel(t *testing.T) {
	for state, want := range map[supervisor.State]string{
		supervisor.StateStarting: "subindo",
		supervisor.StateHealthy:  "conectado",
		supervisor.StateDegraded: "reconectando",
		supervisor.StateFallback: "sem túnel",
		supervisor.StateStopped:  "parado",
	} {
		if got := StateLabel(state); got != want {
			t.Errorf("StateLabel(%d) = %q, want %q", state, got, want)
		}
	}
}
