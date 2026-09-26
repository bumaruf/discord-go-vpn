package httpapi

import (
	"context"
	"log"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/store"
)

// registerDevice records the machine and returns its device id, or empty without a database.
// A device is the label the client sends (the computer name) plus its os user, within the
// lease's invite when it has one; not an IP, which changes with every lease and network.
func (s *server) registerDevice(ctx context.Context, d store.Device) string {
	if s.store == nil {
		return ""
	}

	d.LastSeenAt = s.now()
	id, err := s.store.RegisterDevice(ctx, d)
	if err != nil {
		// The lease is granted anyway; only the panel loses the device.
		log.Printf("registering device %q: %v", d.Label, err)
		return ""
	}
	return id
}
