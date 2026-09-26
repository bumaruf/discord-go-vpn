package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// TokenMatches compares in constant time, so the response time doesn't reveal how much of
// the token was right.
func TokenMatches(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// BearerToken reads the token from the Authorization header.
func BearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if token, ok := strings.CutPrefix(header, "Bearer "); ok {
		return strings.TrimSpace(token)
	}
	return ""
}
