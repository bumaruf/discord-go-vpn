// Package authtest signs tickets for tests of packages that validate them.
package authtest

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
)

// SignTicket signs payload the way panel/lib/ticket.ts does, so tests exercise the real
// format rather than a parallel version of it.
func SignTicket(secret string, payload any) string {
	body, _ := json.Marshal(payload)
	encoded := base64.RawURLEncoding.EncodeToString(body)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
