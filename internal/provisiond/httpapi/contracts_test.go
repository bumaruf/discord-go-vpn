package httpapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/contracttest"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/auth"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/auth/authtest"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/metrics"
)

func TestContractFixtures(t *testing.T) {
	t.Run("lease request", func(t *testing.T) {
		contracttest.AssertRoundTrip[acquireRequest](t, "lease-request.json")
	})
	t.Run("lease response", func(t *testing.T) {
		contracttest.AssertRoundTrip[acquireResponse](t, "lease-response.json")
	})
	t.Run("action response", func(t *testing.T) {
		contracttest.AssertRoundTrip[actionResponse](t, "action-response.json")
	})
	t.Run("live snapshot", func(t *testing.T) {
		contracttest.AssertRoundTrip[metrics.Snapshot](t, "live-snapshot.json")
	})
	t.Run("ticket", func(t *testing.T) {
		contracttest.AssertRoundTrip[auth.Ticket](t, "ticket.json")
	})

	t.Run("ticket validates", func(t *testing.T) {
		const secret = "contract-secret"
		fixture := contracttest.Read(t, "ticket.json")
		var payload struct{ Exp int64 }
		if err := json.Unmarshal(fixture, &payload); err != nil {
			t.Fatalf("parsing ticket.json: %v", err)
		}

		raw := authtest.SignTicket(secret, json.RawMessage(fixture))
		got, err := auth.ValidateTicketAt(secret, raw, time.Unix(payload.Exp-60, 0))
		if err != nil {
			t.Fatalf("ValidateTicketAt: %v", err)
		}
		if want := (auth.Ticket{UID: "admin", Role: "admin", Exp: payload.Exp}); *got != want {
			t.Errorf("ticket = %+v, want %+v", *got, want)
		}
	})
}
