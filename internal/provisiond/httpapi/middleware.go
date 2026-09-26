package httpapi

import (
	"net/http"
	"strings"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/auth"
)

// handleGroup registers routes, keyed by "METHOD /path" pattern, behind guard. The group has
// its own mux, so guard runs before a wrong method can get 405.
func handleGroup(mux *http.ServeMux, guard func(http.Handler) http.Handler, routes map[string]http.Handler) {
	group := http.NewServeMux()
	guarded := guard(group)
	paths := make(map[string]bool)
	for pattern, h := range routes {
		group.Handle(pattern, h)
		_, path, _ := strings.Cut(pattern, " ")
		if !paths[path] {
			paths[path] = true
			mux.Handle(path, guarded)
		}
	}
}

// requireSecret answers 404 to every method while secret isn't configured, so a server
// without the panel doesn't reveal the route.
func requireSecret(secret, disabledMessage string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if secret == "" {
				http.Error(w, disabledMessage, http.StatusNotFound)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// requireBearer answers 401 unless the request's bearer token is secret. An empty secret
// would match a request without one.
func requireBearer(secret, refusal string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if secret == "" || !auth.TokenMatches(auth.BearerToken(r), secret) {
				http.Error(w, refusal, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
