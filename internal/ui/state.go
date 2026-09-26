package ui

import "github.com/bumaruf/discord-go-vpn/internal/supervisor"

// StateLabel is the name a person sees for a state, in the screen, the plain
// text mode and the tray.
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
