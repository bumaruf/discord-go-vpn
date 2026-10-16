package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/store"
)

const panelTestSecret = "panel-secret-for-tests"

// fakePanelData answers the panel's queries from its fields and records its writes.
type fakePanelData struct {
	invites    []store.InviteSummary
	devices    []store.DeviceSummary
	labels     map[string]string
	usage      []store.DayUsage
	usageCalls []string // "from|identity" of each UsageByDay call
	inserted   []store.NewInvite
	missing    bool // DeleteInvite and SetInviteMaxLeases find no invite
	maxLeases  map[string]*int
	audits     []store.AuditEntry
	err        error
}

func (f *fakePanelData) ListInvites(context.Context, int) ([]store.InviteSummary, error) {
	return f.invites, f.err
}

func (f *fakePanelData) InviteLabels(context.Context, int) (map[string]string, error) {
	return f.labels, f.err
}

func (f *fakePanelData) ListDevices(context.Context, int) ([]store.DeviceSummary, error) {
	return f.devices, f.err
}

func (f *fakePanelData) UsageByDay(_ context.Context, from, identity string) ([]store.DayUsage, error) {
	f.usageCalls = append(f.usageCalls, from+"|"+identity)
	return f.usage, f.err
}

func (f *fakePanelData) InsertInvite(_ context.Context, invite store.NewInvite) error {
	f.inserted = append(f.inserted, invite)
	return f.err
}

func (f *fakePanelData) DeleteInvite(context.Context, string) (bool, error) {
	return !f.missing, f.err
}

func (f *fakePanelData) SetInviteMaxLeases(_ context.Context, inviteID string, max *int) (bool, error) {
	if f.maxLeases == nil {
		f.maxLeases = make(map[string]*int)
	}
	f.maxLeases[inviteID] = max
	return !f.missing, f.err
}

func (f *fakePanelData) InsertAudit(_ context.Context, e store.AuditEntry) error {
	f.audits = append(f.audits, e)
	return f.err
}

func newPanelTestServer(t *testing.T) (*testServer, *fakePanelData) {
	t.Helper()
	ts := newTestServer(t)
	ts.config.PanelSecret = panelTestSecret
	data := &fakePanelData{}
	ts.data = data
	return ts, data
}

func panelRequest(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Authorization", "Bearer "+panelTestSecret)
	return req
}

func decodeBody[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decoding %q: %v", rec.Body.String(), err)
	}
	return v
}

var panelDataRoutes = []struct{ method, path string }{
	{http.MethodGet, "/admin/invites"},
	{http.MethodPost, "/admin/invites"},
	{http.MethodDelete, "/admin/invites/" + testInviteID},
	{http.MethodPut, "/admin/invites/" + testInviteID + "/max-leases"},
	{http.MethodGet, "/admin/devices"},
	{http.MethodGet, "/admin/usage?from=2026-09-01"},
	{http.MethodPost, "/admin/audit"},
}

func TestPanelDataRoutesRequireThePanelSecret(t *testing.T) {
	for _, route := range panelDataRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			t.Run("404 without the secret configured", func(t *testing.T) {
				ts := newTestServer(t)
				assertStatus(t, ts.serve(adminRequest(route.method, route.path, panelTestSecret)), http.StatusNotFound)
			})
			t.Run("401 with a wrong bearer", func(t *testing.T) {
				ts, _ := newPanelTestServer(t)
				assertStatus(t, ts.serve(adminRequest(route.method, route.path, "wrong-secret")), http.StatusUnauthorized)
			})
		})
	}
}

func TestPanelDataRoutesWithoutDatabase(t *testing.T) {
	bodies := map[string]any{
		"POST /admin/invites":                                createInviteRequest{Label: "x", Kind: store.InviteIndividual, TokenHash: validHash, Prefix: "vpn_ab", CreatedBy: "admin"},
		"PUT /admin/invites/" + testInviteID + "/max-leases": maxLeasesRequest{},
		"POST /admin/audit":                                  auditRequest{ActorID: "admin", Action: "disconnect"},
	}
	for _, route := range panelDataRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			ts, _ := newPanelTestServer(t)
			ts.data = nil
			body := bodies[route.method+" "+route.path]
			assertStatus(t, ts.serve(panelRequest(t, route.method, route.path, body)), http.StatusServiceUnavailable)
		})
	}
}

const validHash = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"

func TestListInvites(t *testing.T) {
	ts, data := newPanelTestServer(t)
	five := 5
	data.invites = []store.InviteSummary{{
		ID: testInviteID, Label: "Família", Kind: store.InviteGlobal, Prefix: "vpn_ab12",
		Status: store.InviteActive, CreatedAt: time.Date(2026, 9, 12, 9, 0, 0, 0, time.FixedZone("BRT", -3*3600)),
		MaxLeases: &five, DeviceCount: 3,
	}}

	rec := ts.serve(panelRequest(t, http.MethodGet, "/admin/invites", nil))
	assertStatus(t, rec, http.StatusOK)

	got := decodeBody[invitesResponse](t, rec)
	want := inviteJSON{
		ID: testInviteID, Label: "Família", Kind: store.InviteGlobal, Prefix: "vpn_ab12",
		Status: store.InviteActive, CreatedAt: "2026-09-12T12:00:00Z", MaxLeases: &five, DeviceCount: 3,
	}
	if len(got.Invites) != 1 || *got.Invites[0].MaxLeases != 5 || got.Invites[0].CreatedAt != want.CreatedAt ||
		got.Invites[0].ID != want.ID || got.Invites[0].DeviceCount != 3 {
		t.Errorf("invites = %+v, want [%+v]", got.Invites, want)
	}
}

func TestListDevicesIncludesInviteLabels(t *testing.T) {
	ts, data := newPanelTestServer(t)
	data.devices = []store.DeviceSummary{{ID: "dev1", Name: "PC", AccessCount: 2}}
	data.labels = map[string]string{testInviteID: "Família"}

	rec := ts.serve(panelRequest(t, http.MethodGet, "/admin/devices", nil))
	assertStatus(t, rec, http.StatusOK)

	got := decodeBody[devicesResponse](t, rec)
	if len(got.Devices) != 1 || got.Devices[0].ID != "dev1" || got.InviteLabels[testInviteID] != "Família" {
		t.Errorf("response = %+v", got)
	}
}

func TestDatabaseFailureAnswers503(t *testing.T) {
	ts, data := newPanelTestServer(t)
	captureLog(t)
	data.err = errors.New("server selection error")

	assertStatus(t, ts.serve(panelRequest(t, http.MethodGet, "/admin/devices", nil)), http.StatusServiceUnavailable)
}

func TestUsage(t *testing.T) {
	t.Run("rejects a malformed day", func(t *testing.T) {
		ts, _ := newPanelTestServer(t)
		assertStatus(t, ts.serve(panelRequest(t, http.MethodGet, "/admin/usage?from=ontem", nil)), http.StatusBadRequest)
	})

	t.Run("reads one device's usage", func(t *testing.T) {
		ts, data := newPanelTestServer(t)
		data.usage = []store.DayUsage{{Day: "2026-09-12", RxBytes: 10, TxBytes: 20}}

		rec := ts.serve(panelRequest(t, http.MethodGet, "/admin/usage?from=2026-09-01&device=dev1", nil))
		assertStatus(t, rec, http.StatusOK)

		got := decodeBody[usageResponse](t, rec)
		if len(got.Days) != 1 || got.Days[0] != (dayUsageJSON{Day: "2026-09-12", RxBytes: 10, TxBytes: 20}) {
			t.Errorf("days = %+v", got.Days)
		}
		if len(data.usageCalls) != 1 || data.usageCalls[0] != "2026-09-01|dev1" {
			t.Errorf("queries = %v", data.usageCalls)
		}
	})

	t.Run("answers from the cache until the collector could have written again", func(t *testing.T) {
		ts, data := newPanelTestServer(t)
		get := func() {
			assertStatus(t, ts.serve(panelRequest(t, http.MethodGet, "/admin/usage?from=2026-09-01", nil)), http.StatusOK)
		}

		get()
		ts.clock.advance(usageCacheTTL - time.Second)
		get()
		if len(data.usageCalls) != 1 {
			t.Fatalf("queries within the TTL = %d, want 1", len(data.usageCalls))
		}

		ts.clock.advance(time.Second)
		get()
		if len(data.usageCalls) != 2 {
			t.Errorf("queries after the TTL = %d, want 2", len(data.usageCalls))
		}
	})
}

func TestCreateInvite(t *testing.T) {
	valid := createInviteRequest{Label: "Família", Kind: store.InviteGlobal, TokenHash: validHash, Prefix: "vpn_ab12", CreatedBy: "admin"}

	t.Run("stores the invite as of now", func(t *testing.T) {
		ts, data := newPanelTestServer(t)
		assertStatus(t, ts.serve(panelRequest(t, http.MethodPost, "/admin/invites", valid)), http.StatusOK)

		if len(data.inserted) != 1 {
			t.Fatalf("inserted = %d invites, want 1", len(data.inserted))
		}
		got := data.inserted[0]
		if got.TokenHash != validHash || got.Kind != store.InviteGlobal || !got.CreatedAt.Equal(ts.now()) || got.MaxLeases != nil {
			t.Errorf("inserted = %+v", got)
		}
	})

	zero := 0
	invalid := map[string]func(*createInviteRequest){
		"empty label":     func(r *createInviteRequest) { r.Label = "" },
		"long label":      func(r *createInviteRequest) { r.Label = string(bytes.Repeat([]byte("a"), inviteLabelMax+1)) },
		"unknown kind":    func(r *createInviteRequest) { r.Kind = "family" },
		"plaintext token": func(r *createInviteRequest) { r.TokenHash = "vpn_secret" },
		"zero cap":        func(r *createInviteRequest) { r.MaxLeases = &zero },
	}
	for name, change := range invalid {
		t.Run("400 for "+name, func(t *testing.T) {
			ts, data := newPanelTestServer(t)
			req := valid
			change(&req)
			assertStatus(t, ts.serve(panelRequest(t, http.MethodPost, "/admin/invites", req)), http.StatusBadRequest)
			if len(data.inserted) != 0 {
				t.Error("an invalid invite was stored")
			}
		})
	}
}

func TestInviteWritesDropTheCachedInvite(t *testing.T) {
	const token = "vpn_cached-invite-token"
	cases := []struct {
		name string
		req  func(t *testing.T) *http.Request
	}{
		{"delete", func(t *testing.T) *http.Request {
			return panelRequest(t, http.MethodDelete, "/admin/invites/"+testInviteID, nil)
		}},
		{"max leases", func(t *testing.T) *http.Request {
			two := 2
			return panelRequest(t, http.MethodPut, "/admin/invites/"+testInviteID+"/max-leases", maxLeasesRequest{MaxLeases: &two})
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ts, _ := newPanelTestServer(t)
			ts.addInvite(token, testInviteID)
			if ts.inviteCached(t, token) {
				t.Fatal("the invite was cached before its first lookup")
			}

			assertStatus(t, ts.serve(c.req(t)), http.StatusOK)

			if ts.inviteCached(t, token) {
				t.Error("the invite stayed cached after the write")
			}
		})

		t.Run(c.name+" answers 404 for a missing invite", func(t *testing.T) {
			ts, data := newPanelTestServer(t)
			data.missing = true
			assertStatus(t, ts.serve(c.req(t)), http.StatusNotFound)
		})
	}

	t.Run("a null cap removes it", func(t *testing.T) {
		ts, data := newPanelTestServer(t)
		assertStatus(t, ts.serve(panelRequest(t, http.MethodPut, "/admin/invites/"+testInviteID+"/max-leases", maxLeasesRequest{})), http.StatusOK)
		if cap, ok := data.maxLeases[testInviteID]; !ok || cap != nil {
			t.Errorf("cap = %v (set %v), want nil", cap, ok)
		}
	})
}

func TestAudit(t *testing.T) {
	t.Run("stores the entry as of now", func(t *testing.T) {
		ts, data := newPanelTestServer(t)
		entry := auditRequest{ActorID: "admin", Action: "revoke_invite", Target: testInviteID, OK: true, Detail: "Convite revogado"}

		assertStatus(t, ts.serve(panelRequest(t, http.MethodPost, "/admin/audit", entry)), http.StatusNoContent)

		want := store.AuditEntry{ActorID: "admin", Action: "revoke_invite", Target: testInviteID, OK: true, Detail: "Convite revogado", At: ts.now()}
		if len(data.audits) != 1 || data.audits[0] != want {
			t.Errorf("audits = %+v, want [%+v]", data.audits, want)
		}
	})

	t.Run("400 for an unknown action", func(t *testing.T) {
		ts, data := newPanelTestServer(t)
		entry := auditRequest{ActorID: "admin", Action: "drop_database"}
		assertStatus(t, ts.serve(panelRequest(t, http.MethodPost, "/admin/audit", entry)), http.StatusBadRequest)
		if len(data.audits) != 0 {
			t.Error("an unknown action was stored")
		}
	})
}
