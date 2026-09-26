package auth

import (
	"os"
	"testing"
	"time"
)

// TestTicketInterop checks that a ticket signed by the panel (panel/lib/ticket.ts) is accepted
// here. To get a ticket, in panel/:
//
//	VPN_LIVE_SECRET=<secret> npx tsx -e \
//	  "import{issueTicket}from'./lib/ticket';console.log(issueTicket('u1','admin'))"
//
// then run:
//
//	TICKET_INTEROP=<ticket> TICKET_SECRET=<secret> go test ./internal/provisiond/auth/ -run TestTicketInterop
func TestTicketInterop(t *testing.T) {
	raw := os.Getenv("TICKET_INTEROP")
	secret := os.Getenv("TICKET_SECRET")
	if raw == "" || secret == "" {
		t.Skip("set TICKET_INTEROP and TICKET_SECRET to check a ticket signed by the panel")
	}

	got, err := ValidateTicketAt(secret, raw, time.Now())
	if err != nil {
		t.Fatalf("the panel's ticket was refused: %v", err)
	}
	if got.UID == "" || got.Role == "" {
		t.Errorf("decoded %+v, want uid and role set", got)
	}
	t.Logf("accepted: uid=%s role=%s", got.UID, got.Role)
}
