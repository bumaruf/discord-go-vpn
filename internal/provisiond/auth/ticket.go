package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Ticket is what the panel signs to open the live stream: short-lived and read-only, since
// panel actions still need VPN_PANEL_SECRET.
type Ticket struct {
	UID  string `json:"uid"`
	Role string `json:"role"`
	Exp  int64  `json:"exp"`
}

var (
	errInvalidTicket = errors.New("tíquete inválido")
	errExpiredTicket = errors.New("tíquete expirado")
)

// ValidateTicketAt checks the signature and the expiry at now. The format is
// <payload>.<signature>, both unpadded base64url.
func ValidateTicketAt(secret, raw string, now time.Time) (*Ticket, error) {
	if secret == "" {
		return nil, errInvalidTicket
	}

	body, signature, ok := strings.Cut(raw, ".")
	if !ok {
		return nil, errInvalidTicket
	}

	got, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return nil, errInvalidTicket
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	if !hmac.Equal(got, mac.Sum(nil)) {
		return nil, errInvalidTicket
	}

	payload, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return nil, errInvalidTicket
	}

	var t Ticket
	if err := json.Unmarshal(payload, &t); err != nil {
		return nil, errInvalidTicket
	}
	if now.Unix() > t.Exp {
		return nil, errExpiredTicket
	}
	return &t, nil
}
