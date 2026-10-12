package supervisor

import (
	"testing"

	"github.com/bumaruf/discord-go-vpn/internal/routing"
	"github.com/bumaruf/discord-go-vpn/internal/transport"
)

func TestOptionsReplaceTransportAndRestart(t *testing.T) {
	var built, restarted bool
	s := New(Options{
		NewTransport: func(routing.Mode, transport.Reporter) (transport.Transport, error) {
			built = true
			return nil, nil
		},
		RestartDiscord: func(string) (string, error) {
			restarted = true
			return "", nil
		},
	})

	_, _ = s.newTransport("", statusReporter{s})
	_, _ = s.restartDiscord("")
	if !built || !restarted {
		t.Errorf("built=%v restarted=%v, want both from Options", built, restarted)
	}
}
