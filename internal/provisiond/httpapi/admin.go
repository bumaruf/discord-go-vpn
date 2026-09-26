package httpapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/store"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/wireguard"
)

type actionResponse struct {
	OK         bool   `json:"ok"`
	Message    string `json:"message"`
	Affected   int    `json:"affected"`
	ExecutedAt string `json:"executed_at"`
}

// handleDisconnect removes one lease; the client may lease again with the same invite.
func (s *server) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	l, ok := s.registry.Disconnect(r.PathValue("id"))
	if !ok {
		// The lease may have expired between the panel loading and the click.
		s.respondAction(w, actionResponse{OK: true, Message: "esse acesso já não existia"})
		return
	}

	log.Printf("panel: lease %s disconnected", wireguard.ShortKey(l.PubKey))
	s.respondAction(w, actionResponse{OK: true, Message: "Acesso derrubado", Affected: 1})
}

func (s *server) handleRevoke(w http.ResponseWriter, r *http.Request) {
	s.setInviteStatus(w, r, store.InviteRevoked, "Convite revogado", func(ctx context.Context, inviteID string) error {
		return s.store.RevokeInvite(ctx, inviteID, s.now())
	})
}

func (s *server) handleReactivate(w http.ResponseWriter, r *http.Request) {
	s.setInviteStatus(w, r, store.InviteActive, "Convite reativado", func(ctx context.Context, inviteID string) error {
		return s.store.ReactivateInvite(ctx, inviteID)
	})
}

// setInviteStatus stores the new status, then drops the invite from the auth cache, then, on
// revoke, disconnects its leases. That order makes a revoke take effect at once: clients
// can't lease again between the steps.
func (s *server) setInviteStatus(w http.ResponseWriter, r *http.Request, status, message string, update func(ctx context.Context, inviteID string) error) {
	inviteID := r.PathValue("id")
	if s.store == nil {
		http.Error(w, "sem banco: não há convites neste servidor", http.StatusServiceUnavailable)
		return
	}

	err := update(r.Context(), inviteID)
	if errors.Is(err, store.ErrInvalidInviteID) {
		http.Error(w, "convite inválido", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "não consegui atualizar o convite", http.StatusInternalServerError)
		return
	}

	s.auth.Invalidate(inviteID)

	disconnected := 0
	if status == store.InviteRevoked {
		disconnected = s.registry.DisconnectInvite(inviteID)
	}

	log.Printf("panel: invite %s is now %s (%d leases disconnected)", inviteID, status, disconnected)
	s.respondAction(w, actionResponse{OK: true, Message: message, Affected: disconnected})
}

func (s *server) respondAction(w http.ResponseWriter, resp actionResponse) {
	resp.ExecutedAt = s.now().UTC().Format(time.RFC3339)
	respond(w, resp)
}
