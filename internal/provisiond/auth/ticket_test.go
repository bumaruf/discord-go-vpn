package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/auth/authtest"
)

func TestValidateTicket(t *testing.T) {
	const secret = "a-long-enough-secret-for-the-test"
	soon := time.Now().Add(5 * time.Minute).Unix()

	roleClaims := []struct {
		name    string
		payload map[string]any
		want    string
	}{
		{"role", map[string]any{"uid": "u1", "role": "admin", "exp": soon}, "admin"},
		{"neither", map[string]any{"uid": "u1", "exp": soon}, ""},
	}
	for _, c := range roleClaims {
		t.Run("a good ticket passes with "+c.name, func(t *testing.T) {
			raw := authtest.SignTicket(secret, c.payload)

			got, err := ValidateTicketAt(secret, raw, time.Now())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.UID != "u1" || got.Role != c.want {
				t.Errorf("got %+v, want uid=u1 role=%q", got, c.want)
			}
		})
	}

	t.Run("a signature from another secret is refused", func(t *testing.T) {
		raw := authtest.SignTicket("some-other-secret", Ticket{UID: "u1", Role: "admin", Exp: soon})

		if _, err := ValidateTicketAt(secret, raw, time.Now()); err != errInvalidTicket {
			t.Errorf("error = %v, want errInvalidTicket", err)
		}
	})

	// Escalation attempt: an admin body under the signature of a ticket issued for a plain user.
	t.Run("a swapped body under a valid signature is refused", func(t *testing.T) {
		mine := authtest.SignTicket(secret, Ticket{UID: "u1", Role: "user", Exp: soon})
		wanted := authtest.SignTicket(secret, Ticket{UID: "u1", Role: "admin", Exp: soon})

		_, mySignature, _ := strings.Cut(mine, ".")
		adminBody, _, _ := strings.Cut(wanted, ".")

		if _, err := ValidateTicketAt(secret, adminBody+"."+mySignature, time.Now()); err != errInvalidTicket {
			t.Errorf("error = %v, want errInvalidTicket", err)
		}
	})

	t.Run("an expired ticket is refused", func(t *testing.T) {
		raw := authtest.SignTicket(secret, Ticket{UID: "u1", Role: "admin", Exp: time.Now().Add(-time.Minute).Unix()})

		if _, err := ValidateTicketAt(secret, raw, time.Now()); err != errExpiredTicket {
			t.Errorf("error = %v, want errExpiredTicket", err)
		}
	})

	t.Run("nothing passes without a configured secret", func(t *testing.T) {
		raw := authtest.SignTicket("", Ticket{UID: "u1", Role: "admin", Exp: soon})

		if _, err := ValidateTicketAt("", raw, time.Now()); err != errInvalidTicket {
			t.Errorf("error = %v, want errInvalidTicket", err)
		}
	})

	t.Run("garbage is refused without panicking", func(t *testing.T) {
		for _, raw := range []string{"", ".", "abc", "a.b", "nodot"} {
			if _, err := ValidateTicketAt(secret, raw, time.Now()); err == nil {
				t.Errorf("accepted %q", raw)
			}
		}
	})
}
