package statustext

import (
	"testing"
	"time"

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

func TestDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		0:                                     "0s",
		42*time.Second + 900*time.Millisecond: "42s",
		12*time.Minute + 4*time.Second:        "12min 04s",
		2*time.Hour + 5*time.Minute + 59*time.Second: "2h 05min",
	} {
		if got := Duration(d); got != want {
			t.Errorf("Duration(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestTokenAskReason(t *testing.T) {
	if got := TokenAskReason(supervisor.TokenUnanswered); got != "O servidor deste token não respondeu às últimas tentativas." {
		t.Errorf("unanswered: %q", got)
	}
	if got := TokenAskReason(supervisor.TokenRefused); got != "O convite deste token foi revogado, ou o token não vale mais." {
		t.Errorf("refused: %q", got)
	}
}
