package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/auth"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/auth/authtest"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/config"
	"github.com/bumaruf/discord-go-vpn/internal/provisiond/store"
)

func adminRequest(method, path, bearer string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	return req
}

const testInviteID = "65f1a2b3c4d5e6f7a8b9c0d1"

func disconnectPath(leaseID string) string { return "/admin/leases/" + leaseID + "/disconnect" }

func revokePath(inviteID string) string { return "/admin/invites/" + inviteID + "/revoke" }

func reactivatePath(inviteID string) string { return "/admin/invites/" + inviteID + "/reactivate" }

// actionRoutes lists every panel action path, with a lease or invite id filled in.
func actionRoutes() []string {
	return []string{disconnectPath("some-lease"), revokePath(testInviteID), reactivatePath(testInviteID)}
}

func TestAdminRoutesRequireTheirSecret(t *testing.T) {
	const secret = "admin-secret-for-tests"

	type route struct {
		method    string
		path      string
		setSecret func(*config.Config)
	}
	routes := []route{
		{http.MethodGet, "/admin/live", func(c *config.Config) { c.LiveSecret = secret }},
	}
	for _, path := range actionRoutes() {
		routes = append(routes, route{http.MethodPost, path, func(c *config.Config) { c.PanelSecret = secret }})
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			t.Run("404 without the secret configured", func(t *testing.T) {
				ts := newTestServer(t)
				assertStatus(t, ts.serve(adminRequest(route.method, route.path, secret)), http.StatusNotFound)
			})

			t.Run("401 with a wrong bearer", func(t *testing.T) {
				ts := newTestServer(t)
				route.setSecret(&ts.config)
				assertStatus(t, ts.serve(adminRequest(route.method, route.path, "wrong-secret")), http.StatusUnauthorized)
			})

			t.Run("401 without a bearer", func(t *testing.T) {
				ts := newTestServer(t)
				route.setSecret(&ts.config)
				assertStatus(t, ts.serve(adminRequest(route.method, route.path, "")), http.StatusUnauthorized)
			})
		})
	}
}

var allMethods = []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions}

var corsHeaders = []string{
	"Access-Control-Allow-Origin",
	"Access-Control-Allow-Headers",
	"Access-Control-Allow-Methods",
	"Access-Control-Max-Age",
	"Vary",
}

func assertNoCORS(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	for _, header := range corsHeaders {
		if got := rec.Header().Get(header); got != "" {
			t.Errorf("%s = %q, want none", header, got)
		}
	}
}

func TestAdminRoutesHiddenWithoutSecret(t *testing.T) {
	for _, path := range actionRoutes() {
		for _, method := range allMethods {
			t.Run(method+" "+path, func(t *testing.T) {
				ts := newTestServer(t)
				assertStatus(t, ts.serve(adminRequest(method, path, "")), http.StatusNotFound)
			})
		}
	}
}

func TestAdminDisconnect(t *testing.T) {
	const secret = "panel-secret-for-tests"

	t.Run("removes the lease", func(t *testing.T) {
		ts := newTestServer(t)
		ts.config.PanelSecret = secret
		lease := ts.acquire(t, testPublicKey(1))

		rec := ts.serve(adminRequest(http.MethodPost, disconnectPath(lease.ID), secret))
		assertStatus(t, rec, http.StatusOK)

		if resp := decodeAction(t, rec); !resp.OK || resp.Affected != 1 {
			t.Errorf("response = %+v, want ok with 1 affected", resp)
		}
		if _, kept := ts.leases()[lease.ID]; kept {
			t.Error("lease still stored")
		}
		if _, kept := ts.wg.Peers[testPublicKey(1)]; kept {
			t.Error("peer still on the interface")
		}
	})

	t.Run("an unknown lease answers ok with nothing affected", func(t *testing.T) {
		ts := newTestServer(t)
		ts.config.PanelSecret = secret

		rec := ts.serve(adminRequest(http.MethodPost, disconnectPath("missing"), secret))
		assertStatus(t, rec, http.StatusOK)

		if resp := decodeAction(t, rec); !resp.OK || resp.Affected != 0 {
			t.Errorf("response = %+v, want ok with 0 affected", resp)
		}
	})

	t.Run("405 for GET", func(t *testing.T) {
		ts := newTestServer(t)
		ts.config.PanelSecret = secret
		assertStatus(t, ts.serve(adminRequest(http.MethodGet, disconnectPath("x"), secret)), http.StatusMethodNotAllowed)
	})
}

func decodeAction(t *testing.T, rec *httptest.ResponseRecorder) actionResponse {
	t.Helper()
	var resp actionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	return resp
}

func TestAdminInviteStatusWithoutDatabase(t *testing.T) {
	const secret = "panel-secret-for-tests"
	ts := newTestServer(t)
	ts.config.PanelSecret = secret

	for _, path := range []string{revokePath(testInviteID), reactivatePath(testInviteID)} {
		if rec := ts.serve(adminRequest(http.MethodPost, path, secret)); rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: status = %d, want 503", path, rec.Code)
		}
	}
}

// inviteStatusStore records invite status changes and answers them with err; any other
// store call panics on the nil embedded store.
type inviteStatusStore struct {
	store.Store
	err   error
	calls []string
}

func (f *inviteStatusStore) RevokeInvite(_ context.Context, inviteID string, at time.Time) error {
	f.calls = append(f.calls, "revoke "+inviteID+" at "+at.Format(time.RFC3339))
	return f.err
}

func (f *inviteStatusStore) ReactivateInvite(_ context.Context, inviteID string) error {
	f.calls = append(f.calls, "reactivate "+inviteID)
	return f.err
}

func TestAdminInviteStatus(t *testing.T) {
	const (
		secret      = "panel-secret-for-tests"
		inviteToken = "vpn_invite-token-for-tests"
		inviteID    = testInviteID
	)

	// newServer grants a lease through the invite and another through the default test invite, then
	// swaps in the fake store.
	newServer := func(t *testing.T) (*testServer, *inviteStatusStore, acquireResponse) {
		t.Helper()
		ts := newTestServer(t)
		ts.config.PanelSecret = secret
		ts.addInvite(inviteToken, inviteID)
		ts.acquireWith(t, inviteToken, testPublicKey(1))
		other := ts.acquire(t, testPublicKey(2))
		st := &inviteStatusStore{}
		ts.store = st
		return ts, st, other
	}

	t.Run("revoking stores the time, drops the invite's leases and forgets its cached token", func(t *testing.T) {
		ts, st, other := newServer(t)

		rec := ts.serve(adminRequest(http.MethodPost, revokePath(inviteID), secret))
		assertStatus(t, rec, http.StatusOK)

		now := ts.clock.now().UTC().Format(time.RFC3339)
		if resp := decodeAction(t, rec); !resp.OK || resp.Affected != 1 || resp.ExecutedAt != now {
			t.Errorf("response = %+v, want ok with 1 affected, executed at %s", resp, now)
		}
		if want := []string{"revoke " + inviteID + " at " + now}; !slices.Equal(st.calls, want) {
			t.Errorf("store calls = %q, want %q", st.calls, want)
		}
		if len(ts.leases()) != 1 || ts.leases()[other.ID] == nil {
			t.Errorf("leases = %v, want only the other invite's lease", ts.leases())
		}
		if ts.inviteCached(t, inviteToken) {
			t.Error("the revoked invite is still cached")
		}
	})

	t.Run("reactivating keeps the leases", func(t *testing.T) {
		ts, st, _ := newServer(t)

		rec := ts.serve(adminRequest(http.MethodPost, reactivatePath(inviteID), secret))
		assertStatus(t, rec, http.StatusOK)

		if resp := decodeAction(t, rec); !resp.OK || resp.Affected != 0 {
			t.Errorf("response = %+v, want ok with 0 affected", resp)
		}
		if want := []string{"reactivate " + inviteID}; !slices.Equal(st.calls, want) {
			t.Errorf("store calls = %q, want %q", st.calls, want)
		}
		if len(ts.leases()) != 2 {
			t.Errorf("leases = %v, want both kept", ts.leases())
		}
	})

	failures := []struct {
		name   string
		err    error
		status int
	}{
		{"400 for an id no invite can have", store.ErrInvalidInviteID, http.StatusBadRequest},
		{"500 when the store fails", errors.New("server selection error"), http.StatusInternalServerError},
	}
	for _, f := range failures {
		t.Run(f.name+", keeping the leases and the cache", func(t *testing.T) {
			ts, st, _ := newServer(t)
			st.err = f.err

			rec := ts.serve(adminRequest(http.MethodPost, revokePath(inviteID), secret))
			assertStatus(t, rec, f.status)
			if len(ts.leases()) != 2 {
				t.Errorf("leases = %v, want both kept", ts.leases())
			}
			if !ts.inviteCached(t, inviteToken) {
				t.Error("the invite left the cache")
			}
		})
	}
}

func TestLiveRejectsNonAdminTicket(t *testing.T) {
	const secret = "live-secret-for-tests"
	ts := newTestServer(t)
	ts.config.LiveSecret = secret

	ticket := authtest.SignTicket(secret, auth.Ticket{UID: "u1", Role: "user", Exp: time.Now().Add(5 * time.Minute).Unix()})
	assertStatus(t, ts.serve(adminRequest(http.MethodGet, "/admin/live", ticket)), http.StatusForbidden)
}

const testPanelOrigin = "https://panel.example.com"

func TestLivePreflight(t *testing.T) {
	t.Run("allows the panel origin", func(t *testing.T) {
		ts := newTestServer(t)
		ts.config.PanelOrigin = testPanelOrigin
		ts.config.LiveSecret = "live-secret-for-tests"

		rec := ts.serve(httptest.NewRequest(http.MethodOptions, "/admin/live", nil))
		assertStatus(t, rec, http.StatusNoContent)

		want := map[string]string{
			"Access-Control-Allow-Origin":  testPanelOrigin,
			"Access-Control-Allow-Headers": "Authorization",
			"Access-Control-Allow-Methods": "GET, OPTIONS",
			"Access-Control-Max-Age":       "600",
			"Vary":                         "Origin",
		}
		for header, value := range want {
			if got := rec.Header().Get(header); got != value {
				t.Errorf("%s = %q, want %q", header, got, value)
			}
		}
	})

	t.Run("sends no allowed origin when none is configured", func(t *testing.T) {
		ts := newTestServer(t)
		ts.config.LiveSecret = "live-secret-for-tests"

		rec := ts.serve(httptest.NewRequest(http.MethodOptions, "/admin/live", nil))
		assertStatus(t, rec, http.StatusNoContent)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Access-Control-Allow-Origin = %q, want none", got)
		}
	})

	t.Run("405 for POST", func(t *testing.T) {
		ts := newTestServer(t)
		ts.config.LiveSecret = "live-secret-for-tests"
		assertStatus(t, ts.serve(httptest.NewRequest(http.MethodPost, "/admin/live", nil)), http.StatusMethodNotAllowed)
	})
}

func TestLiveHiddenWithoutSecret(t *testing.T) {
	for _, method := range []string{http.MethodOptions, http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			ts := newTestServer(t)
			ts.config.PanelOrigin = testPanelOrigin

			rec := ts.serve(httptest.NewRequest(method, "/admin/live", nil))
			assertStatus(t, rec, http.StatusNotFound)
			assertNoCORS(t, rec)
		})
	}
}
