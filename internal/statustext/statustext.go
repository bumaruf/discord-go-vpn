// Package statustext holds the words a person reads about the tunnel's state, shared
// by the screen, the plain text mode, the tray and the Android app.
package statustext

import (
	"fmt"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/supervisor"
)

func StateLabel(s supervisor.State) string {
	switch s {
	case supervisor.StateStarting:
		return "subindo"
	case supervisor.StateHealthy:
		return "conectado"
	case supervisor.StateDegraded:
		return "reconectando"
	case supervisor.StateFallback:
		return "sem túnel"
	}
	return "parado"
}

// TokenAskReason says why a new connection token is asked for.
func TokenAskReason(ask supervisor.TokenAsk) string {
	if ask == supervisor.TokenUnanswered {
		return "O servidor deste token não respondeu às últimas tentativas."
	}
	return "O convite deste token foi revogado, ou o token não vale mais."
}

func Duration(d time.Duration) string {
	d = d.Truncate(time.Second)
	h, m, s := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	switch {
	case h > 0:
		return fmt.Sprintf("%dh %02dmin", h, m)
	case m > 0:
		return fmt.Sprintf("%dmin %02ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}
